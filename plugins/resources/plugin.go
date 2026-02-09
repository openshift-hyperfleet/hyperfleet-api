// Package resources provides a dynamic plugin that loads CRD definitions and registers routes.
// CRD definitions are loaded from the Kubernetes API server at startup.
package resources

import (
	"context"
	"net/http"
	"os"

	"github.com/gorilla/mux"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/presenters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/crd"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

// ServiceLocator creates a ResourceService instance
type ServiceLocator func() services.ResourceService

// NewServiceLocator creates a new service locator for resources
func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() services.ResourceService {
		return services.NewResourceService(
			dao.NewResourceDao(&env.Database.SessionFactory),
			dao.NewAdapterStatusDao(&env.Database.SessionFactory),
		)
	}
}

// Service retrieves the ResourceService from the services registry
func Service(s *environments.Services) services.ResourceService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("Resources"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	ctx := context.Background()
	crdLoaded := false

	// Try loading from local files first (for local development)
	if crdPath := os.Getenv("CRD_PATH"); crdPath != "" {
		if err := crd.LoadFromDirectory(crdPath); err != nil {
			logger.WithError(nil, err).Warn(
				"Failed to load CRDs from directory, trying Kubernetes API")
		} else {
			logger.With(nil, "crd_count", crd.DefaultRegistry().Count(), "path", crdPath).Info(
				"Loaded CRD definitions from local files")
			crdLoaded = true
		}
	}

	// Fall back to Kubernetes API
	if !crdLoaded {
		if err := crd.LoadFromKubernetes(ctx); err != nil {
			// Log warning but don't fail - CRDs might not be present in all environments
			logger.WithError(nil, err).Warn(
				"Failed to load CRDs from Kubernetes API, generic resource API disabled")
		} else {
			logger.With(nil, "crd_count", crd.DefaultRegistry().Count()).Info(
				"Loaded CRD definitions from Kubernetes API")
		}
	}

	// Service registration
	registry.RegisterService("Resources", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	// Dynamic route registration based on loaded CRDs
	server.RegisterRoutes("resources", func(
		apiV1Router *mux.Router,
		services server.ServicesInterface,
		authMiddleware auth.JWTMiddleware,
		authzMiddleware auth.AuthorizationMiddleware,
	) {
		envServices := services.(*environments.Services)
		resourceService := Service(envServices)

		if resourceService == nil {
			return
		}

		// Register routes for each enabled CRD
		for _, def := range crd.All() {
			registerResourceRoutes(apiV1Router, def, resourceService, authMiddleware, authzMiddleware)
		}
	})

	// Presenter registration for Resource type
	presenters.RegisterPath(api.Resource{}, "resources")
	presenters.RegisterPath(&api.Resource{}, "resources")
	presenters.RegisterKind(api.Resource{}, "Resource")
	presenters.RegisterKind(&api.Resource{}, "Resource")
}

// registerResourceRoutes registers HTTP routes for a single CRD definition
func registerResourceRoutes(
	apiV1Router *mux.Router,
	def *api.ResourceDefinition,
	resourceService services.ResourceService,
	authMiddleware auth.JWTMiddleware,
	authzMiddleware auth.AuthorizationMiddleware,
) {
	handlerCfg := handlers.ResourceHandlerConfig{
		Kind:             def.Kind,
		Plural:           def.Plural,
		IsOwned:          def.IsOwned(),
		OwnerKind:        def.GetOwnerKind(),
		OwnerPathParam:   def.GetOwnerPathParam(),
		RequiredAdapters: def.StatusConfig.RequiredAdapters,
	}

	handler := handlers.NewResourceHandler(resourceService, handlerCfg)

	var router *mux.Router

	if def.IsOwned() {
		// Owned resources are nested under their owner
		// e.g., /clusters/{cluster_id}/nodepools
		ownerPlural := getOwnerPlural(def.GetOwnerKind())
		pathPrefix := "/" + ownerPlural + "/{" + def.GetOwnerPathParam() + "}/" + def.Plural
		router = apiV1Router.PathPrefix(pathPrefix).Subrouter()
	} else {
		// Root resources at top level
		// e.g., /clusters
		router = apiV1Router.PathPrefix("/" + def.Plural).Subrouter()
	}

	// Register standard CRUD routes
	router.HandleFunc("", handler.List).Methods(http.MethodGet)
	router.HandleFunc("", handler.Create).Methods(http.MethodPost)
	router.HandleFunc("/{id}", handler.Get).Methods(http.MethodGet)
	router.HandleFunc("/{id}", handler.Patch).Methods(http.MethodPatch)
	router.HandleFunc("/{id}", handler.Delete).Methods(http.MethodDelete)

	// Apply authentication and authorization middleware
	router.Use(authMiddleware.AuthenticateAccountJWT)
	router.Use(authzMiddleware.AuthorizeApi)

	logger.With(nil, "kind", def.Kind).Info(
		"Registered routes for resource type")
}

// getOwnerPlural returns the plural form of an owner kind.
// This looks up the CRD definition for the owner to get its plural.
func getOwnerPlural(kind string) string {
	if def, found := crd.GetByKind(kind); found {
		return def.Plural
	}
	// Fallback: lowercase + "s"
	return kind + "s"
}
