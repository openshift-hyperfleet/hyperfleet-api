package server

import (
	"context"
	"fmt"
	"net/http"

	gorillahandlers "github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server/logging"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/middleware"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validators"
)

type ServicesInterface interface {
	GetService(name string) interface{}
}

type RouteRegistrationFunc func(
	apiV1Router *mux.Router,
	services ServicesInterface,
	authMiddleware auth.JWTMiddleware,
)

var routeRegistry = make(map[string]RouteRegistrationFunc)

func RegisterRoutes(name string, registrationFunc RouteRegistrationFunc) {
	routeRegistry[name] = registrationFunc
}

// LoadDiscoveredRoutes invokes all registered route registration functions.
//
// Note: All routes must use .Methods() to restrict HTTP methods.
func LoadDiscoveredRoutes(
	apiV1Router *mux.Router,
	services ServicesInterface,
	authMiddleware auth.JWTMiddleware,
) {
	for name, registrationFunc := range routeRegistry {
		registrationFunc(apiV1Router, services, authMiddleware)
		_ = name // prevent unused variable warning
	}
}

func (s *apiServer) routes(tracingEnabled bool) *mux.Router {
	services := &env().Services

	metadataHandler := handlers.NewMetadataHandler()

	var authMiddleware auth.JWTMiddleware
	authMiddleware = &auth.MiddlewareMock{}
	if env().Config.Server.JWT.Enabled {
		var err error
		authMiddleware, err = auth.NewAuthMiddleware()
		check(err, "Unable to create auth middleware")
	}
	if authMiddleware == nil {
		check(fmt.Errorf("auth middleware is nil"), "Unable to create auth middleware: missing middleware")
	}

	// mainRouter is top level "/"
	mainRouter := mux.NewRouter()
	mainRouter.NotFoundHandler = http.HandlerFunc(api.SendNotFound)

	// Request ID middleware sets a unique request ID in the context of each request for tracing
	mainRouter.Use(logger.RequestIDMiddleware)

	// OpenTelemetry middleware (conditionally enabled)
	// Extracts trace_id/span_id from traceparent header and adds to logger context
	if tracingEnabled {
		mainRouter.Use(middleware.OTelMiddleware)
	}

	// Initialize masking middleware once (reused across all requests)
	masker := middleware.NewMaskingMiddleware(env().Config.Logging)

	// Request logging middleware logs pertinent information about the request and response
	mainRouter.Use(logging.RequestLoggingMiddleware(masker))

	//  /api/hyperfleet
	apiRouter := mainRouter.PathPrefix("/api/hyperfleet").Subrouter()
	apiRouter.HandleFunc("", metadataHandler.Get).Methods(http.MethodGet)

	//  /api/hyperfleet/v1
	apiV1Router := apiRouter.PathPrefix("/v1").Subrouter()

	//  /api/hyperfleet/v1/openapi
	openapiHandler, err := handlers.NewOpenAPIHandler()
	check(err, "Unable to create OpenAPI handler")
	apiV1Router.HandleFunc("/openapi.html", openapiHandler.GetOpenAPIUI).Methods(http.MethodGet)
	apiV1Router.HandleFunc("/openapi", openapiHandler.GetOpenAPI).Methods(http.MethodGet)

	err = registerAPIMiddleware(apiV1Router)
	check(err, "Failed to initialize API middleware")

	// Auto-discovered routes (no manual editing needed)
	LoadDiscoveredRoutes(apiV1Router, services, authMiddleware)

	return mainRouter
}

func registerAPIMiddleware(router *mux.Router) error {
	router.Use(MetricsMiddleware)

	schemaPath := env().Config.Server.OpenAPISchemaPath
	ctx := context.Background()

	schemaValidator, err := validators.NewSchemaValidator(schemaPath)
	if err != nil {
		return fmt.Errorf("schema validation required but failed to load from %s: %w", schemaPath, err)
	}

	logger.With(ctx, logger.FieldSchemaPath, schemaPath).Info("Schema validation enabled")
	router.Use(middleware.SchemaValidationMiddleware(schemaValidator))

	router.Use(
		func(next http.Handler) http.Handler {
			return db.TransactionMiddleware(next, env().Database.SessionFactory, env().Config.Database.Pool.RequestTimeout)
		},
	)

	router.Use(gorillahandlers.CompressHandler)

	return nil
}
