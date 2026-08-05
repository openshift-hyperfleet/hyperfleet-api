package server

import (
	"fmt"

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

	// authMiddleware must be outermost so unauthenticated requests are rejected
	// before protectedAPIMiddleware opens a DB transaction or runs schema validation.
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
