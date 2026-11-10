package server

import (
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/fleet"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
)

const apiBasePath = "/api/hyperfleet"

const apiV1BasePath = apiBasePath + "/v1"

type RouteRegistrar struct {
	Register func(*Router) error
	Name     string
}

func NewRouterFromConfig(
	mainMiddleware []Middleware,
	apiMiddleware []Middleware,
	protectedAPIMiddleware []Middleware,
	authMiddleware []Middleware,
	registrars []RouteRegistrar,
	fleetLister func() ([]fleet.MC, error),
	serviceClusterName string,
) (*Router, error) {
	metadataHandler := handlers.NewMetadataHandler()

	// mainRouter is top level "/"
	mainRouter := NewRouter()
	for _, mw := range mainMiddleware {
		mainRouter.Use(mw)
	}

	//  /api/hyperfleet
	apiRouter := mainRouter.Group()
	apiRouter.HandleFunc("GET "+apiBasePath, metadataHandler.Get)

	//  /api/hyperfleet/v1 - public API routes get metrics and compression
	apiV1Router := apiRouter.Group(apiV1BasePath)
	for _, mw := range apiMiddleware {
		apiV1Router.Use(mw)
	}

	//  /api/hyperfleet/v1/openapi
	openapiHandler, err := handlers.NewOpenAPIHandler()
	if err != nil {
		return nil, fmt.Errorf("unable to create OpenAPI handler: %w", err)
	}
	apiV1Router.HandleFunc("GET /openapi.html", openapiHandler.GetOpenAPIUI)
	apiV1Router.HandleFunc("GET /openapi", openapiHandler.GetOpenAPI)

	//  /api/hyperfleet/v1/poc-dashboard
	pocDashboardHandler, err := handlers.NewPocDashboardHandler()
	if err != nil {
		return nil, fmt.Errorf("unable to create PoC dashboard handler: %w", err)
	}
	apiV1Router.HandleFunc("GET /poc-dashboard", pocDashboardHandler.GetPocDashboard)

	//  /api/hyperfleet/v1/poc-fleet
	pocFleetHandler := handlers.NewPocFleetHandler(fleetLister, serviceClusterName)
	apiV1Router.HandleFunc("GET /poc-fleet", pocFleetHandler.GetPocFleet)

	// authMiddleware must be outermost so unauthenticated requests are rejected
	// before protectedAPIMiddleware runs schema validation or reaches mutation services.
	protectedRouter := apiV1Router.Group()
	for _, mw := range authMiddleware {
		protectedRouter.Use(mw)
	}
	for _, mw := range protectedAPIMiddleware {
		protectedRouter.Use(mw)
	}

	if err := registerRoutes(protectedRouter, registrars); err != nil {
		return nil, err
	}

	return mainRouter, nil
}

func registerRoutes(router *Router, registrars []RouteRegistrar) error {
	for _, registrar := range registrars {
		if registrar.Register == nil {
			return fmt.Errorf("register %s routes: registrar is nil", registrar.Name)
		}
		if err := registrar.Register(router); err != nil {
			return fmt.Errorf("register %s routes: %w", registrar.Name, err)
		}
	}
	return nil
}
