package nodePools

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/presenters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/generic"
)

// ServiceLocator Service Locator
type ServiceLocator func() services.NodePoolService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	// Initialize adapter requirements config from environment variables
	adapterConfig := config.NewAdapterRequirementsConfig()

	return func() services.NodePoolService {
		return services.NewNodePoolService(
			dao.NewNodePoolDao(&env.Database.SessionFactory),
			dao.NewAdapterStatusDao(&env.Database.SessionFactory),
			adapterConfig,
		)
	}
}

// Service helper function to get the nodePool service from the registry
func Service(s *environments.Services) services.NodePoolService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("NodePools"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	// Service registration
	registry.RegisterService("NodePools", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	// Routes registration
	server.RegisterRoutes("nodePools", func(apiV1Router *mux.Router, services server.ServicesInterface, authMiddleware auth.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		nodePoolHandler := handlers.NewNodePoolHandler(Service(envServices), generic.Service(envServices))

		// Only register routes that are in the OpenAPI spec
		// GET /api/hyperfleet/v1/nodepools - List all nodepools
		nodePoolsRouter := apiV1Router.PathPrefix("/nodepools").Subrouter()
		nodePoolsRouter.HandleFunc("", nodePoolHandler.List).Methods(http.MethodGet)

		nodePoolsRouter.Use(authMiddleware.AuthenticateAccountJWT)
		nodePoolsRouter.Use(authzMiddleware.AuthorizeApi)
	})

	// REMOVED: Controller registration - Sentinel handles orchestration
	// Controllers are no longer run inside the API service

	// Presenter registration
	presenters.RegisterPath(api.NodePool{}, "node_pools")
	presenters.RegisterPath(&api.NodePool{}, "node_pools")
	presenters.RegisterKind(api.NodePool{}, "NodePool")
	presenters.RegisterKind(&api.NodePool{}, "NodePool")
}
