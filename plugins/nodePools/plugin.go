package nodePools

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
	"github.com/openshift-hyperfleet/hyperfleet/plugins/events"
	"github.com/openshift-hyperfleet/hyperfleet/plugins/generic"
)

// ServiceLocator Service Locator
type ServiceLocator func() services.NodePoolService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() services.NodePoolService {
		return services.NewNodePoolService(
			db.NewAdvisoryLockFactory(env.Database.SessionFactory),
			dao.NewNodePoolDao(&env.Database.SessionFactory),
			dao.NewAdapterStatusDao(&env.Database.SessionFactory),
			events.Service(&env.Services),
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

	// Controller registration
	server.RegisterController("NodePools", func(manager *controllers.KindControllerManager, services *environments.Services) {
		nodePoolServices := Service(services)

		manager.Add(&controllers.ControllerConfig{
			Source: "NodePools",
			Handlers: map[api.EventType][]controllers.ControllerHandlerFunc{
				api.CreateEventType: {nodePoolServices.OnUpsert},
				api.UpdateEventType: {nodePoolServices.OnUpsert},
				api.DeleteEventType: {nodePoolServices.OnDelete},
			},
		})
	})

	// Presenter registration
	presenters.RegisterPath(api.NodePool{}, "node_pools")
	presenters.RegisterPath(&api.NodePool{}, "node_pools")
	presenters.RegisterKind(api.NodePool{}, "NodePool")
	presenters.RegisterKind(&api.NodePool{}, "NodePool")
}
