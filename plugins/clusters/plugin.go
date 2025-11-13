package clusters

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/openshift-hyperfleet/hyperfleet/cmd/hyperfleet/environments"
	"github.com/openshift-hyperfleet/hyperfleet/cmd/hyperfleet/environments/registry"
	"github.com/openshift-hyperfleet/hyperfleet/cmd/hyperfleet/server"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/api/presenters"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/controllers"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/handlers"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet/plugins/adapterStatus"
	"github.com/openshift-hyperfleet/hyperfleet/plugins/events"
	"github.com/openshift-hyperfleet/hyperfleet/plugins/generic"
	"github.com/openshift-hyperfleet/hyperfleet/plugins/nodePools"
)

// ServiceLocator Service Locator
type ServiceLocator func() services.ClusterService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() services.ClusterService {
		return services.NewClusterService(
			db.NewAdvisoryLockFactory(env.Database.SessionFactory),
			dao.NewClusterDao(&env.Database.SessionFactory),
			dao.NewAdapterStatusDao(&env.Database.SessionFactory),
			events.Service(&env.Services),
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

	// Controller registration
	server.RegisterController("Clusters", func(manager *controllers.KindControllerManager, services *environments.Services) {
		clusterServices := Service(services)

		manager.Add(&controllers.ControllerConfig{
			Source: "Clusters",
			Handlers: map[api.EventType][]controllers.ControllerHandlerFunc{
				api.CreateEventType: {clusterServices.OnUpsert},
				api.UpdateEventType: {clusterServices.OnUpsert},
				api.DeleteEventType: {clusterServices.OnDelete},
			},
		})
	})

	// Presenter registration
	presenters.RegisterPath(api.Cluster{}, "clusters")
	presenters.RegisterPath(&api.Cluster{}, "clusters")
	presenters.RegisterKind(api.Cluster{}, "Cluster")
	presenters.RegisterKind(&api.Cluster{}, "Cluster")
}
