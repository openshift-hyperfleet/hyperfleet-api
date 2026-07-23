package server

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
)

type RouteRegistrar struct {
	Register func(*mux.Router) error
	Name     string
}

func NewRouter(
	mainMiddleware []mux.MiddlewareFunc,
	apiMiddleware []mux.MiddlewareFunc,
	protectedMiddleware []mux.MiddlewareFunc,
	registrars []RouteRegistrar,
) (*mux.Router, error) {
	metadataHandler := handlers.NewMetadataHandler()

	// mainRouter is top level "/"
	mainRouter := mux.NewRouter()
	mainRouter.NotFoundHandler = http.HandlerFunc(api.SendNotFound)
	for _, middleware := range mainMiddleware {
		mainRouter.Use(middleware)
	}

	//  /api/hyperfleet
	apiRouter := mainRouter.PathPrefix("/api/hyperfleet").Subrouter()
	apiRouter.HandleFunc("", metadataHandler.Get).Methods(http.MethodGet)

	//  /api/hyperfleet/v1
	apiV1Router := apiRouter.PathPrefix("/v1").Subrouter()

	//  /api/hyperfleet/v1/openapi
	openapiHandler, err := handlers.NewOpenAPIHandler()
	if err != nil {
		return nil, fmt.Errorf("unable to create OpenAPI handler: %w", err)
	}
	apiV1Router.HandleFunc("/openapi.html", openapiHandler.GetOpenAPIUI).Methods(http.MethodGet)
	apiV1Router.HandleFunc("/openapi", openapiHandler.GetOpenAPI).Methods(http.MethodGet)

	// protectedMiddleware (auth) must be outermost so unauthenticated requests are
	// rejected before apiMiddleware opens a DB transaction or runs schema validation.
	protectedRouter := apiV1Router.PathPrefix("").Subrouter()
	for _, middleware := range protectedMiddleware {
		protectedRouter.Use(middleware)
	}
	for _, middleware := range apiMiddleware {
		protectedRouter.Use(middleware)
	}

	if err := registerRoutes(protectedRouter, registrars); err != nil {
		return nil, err
	}

	return mainRouter, nil
}

func registerRoutes(router *mux.Router, registrars []RouteRegistrar) error {
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
