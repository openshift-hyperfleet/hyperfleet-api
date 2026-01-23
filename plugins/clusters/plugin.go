package clusters

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
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/adapterStatus"
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/generic"
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/nodePools"
)

// ServiceLocator Service Locator
type ServiceLocator func() services.ClusterService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	// Initialize adapter requirements config from environment variables
	adapterConfig := config.NewAdapterRequirementsConfig()

	return func() services.ClusterService {
		return services.NewClusterService(
			dao.NewClusterDao(&env.Database.SessionFactory),
			dao.NewAdapterStatusDao(&env.Database.SessionFactory),
			adapterConfig,
		)
	}
}

// Service helper function to get the cluster service from the registry
func Service(s *environments.Services) services.ClusterService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("Clusters"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	// Service registration
	registry.RegisterService("Clusters", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})

	// Routes registration
	server.RegisterRoutes("clusters", func(apiV1Router *mux.Router, services server.ServicesInterface, authMiddleware auth.JWTMiddleware, authzMiddleware auth.AuthorizationMiddleware) {
		envServices := services.(*environments.Services)
		clusterHandler := handlers.NewClusterHandler(Service(envServices), generic.Service(envServices))

		clustersRouter := apiV1Router.PathPrefix("/clusters").Subrouter()
		clustersRouter.HandleFunc("", clusterHandler.List).Methods(http.MethodGet)
		clustersRouter.HandleFunc("/{id}", clusterHandler.Get).Methods(http.MethodGet)
		clustersRouter.HandleFunc("", clusterHandler.Create).Methods(http.MethodPost)
		clustersRouter.HandleFunc("/{id}", clusterHandler.Patch).Methods(http.MethodPatch)
		clustersRouter.HandleFunc("/{id}", clusterHandler.Delete).Methods(http.MethodDelete)

		// Nested resource: cluster statuses
		clusterStatusHandler := handlers.NewClusterStatusHandler(adapterStatus.Service(envServices), Service(envServices))
		clustersRouter.HandleFunc("/{id}/statuses", clusterStatusHandler.List).Methods(http.MethodGet)
		clustersRouter.HandleFunc("/{id}/statuses", clusterStatusHandler.Create).Methods(http.MethodPost)

		// Nested resource: cluster nodepools
		clusterNodePoolsHandler := handlers.NewClusterNodePoolsHandler(
			Service(envServices),
			nodePools.Service(envServices),
			generic.Service(envServices),
		)
		clustersRouter.HandleFunc("/{id}/nodepools", clusterNodePoolsHandler.List).Methods(http.MethodGet)
		clustersRouter.HandleFunc("/{id}/nodepools", clusterNodePoolsHandler.Create).Methods(http.MethodPost)
		clustersRouter.HandleFunc("/{id}/nodepools/{nodepool_id}", clusterNodePoolsHandler.Get).Methods(http.MethodGet)

		// Nested resource: nodepool statuses
		nodepoolStatusHandler := handlers.NewNodePoolStatusHandler(adapterStatus.Service(envServices), nodePools.Service(envServices))
		clustersRouter.HandleFunc("/{id}/nodepools/{nodepool_id}/statuses", nodepoolStatusHandler.List).Methods(http.MethodGet)
		clustersRouter.HandleFunc("/{id}/nodepools/{nodepool_id}/statuses", nodepoolStatusHandler.Create).Methods(http.MethodPost)

		clustersRouter.Use(authMiddleware.AuthenticateAccountJWT)
		clustersRouter.Use(authzMiddleware.AuthorizeApi)
	})

	// REMOVED: Controller registration - Sentinel handles orchestration
	// Controllers are no longer run inside the API service

	// Presenter registration
	presenters.RegisterPath(api.Cluster{}, "clusters")
	presenters.RegisterPath(&api.Cluster{}, "clusters")
	presenters.RegisterKind(api.Cluster{}, "Cluster")
	presenters.RegisterKind(&api.Cluster{}, "Cluster")
}
