package server

import (
	"context"
	"fmt"
	"net/http"
	"os"

	gorillahandlers "github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server/logging"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/crd"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/middleware"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validators"
)

type ServicesInterface interface {
	GetService(name string) interface{}
}

type RouteRegistrationFunc func(
	apiV1Router *mux.Router,
	services ServicesInterface,
	authMiddleware auth.JWTMiddleware,
	authzMiddleware auth.AuthorizationMiddleware,
)

var routeRegistry = make(map[string]RouteRegistrationFunc)

func RegisterRoutes(name string, registrationFunc RouteRegistrationFunc) {
	routeRegistry[name] = registrationFunc
}

func LoadDiscoveredRoutes(
	apiV1Router *mux.Router,
	services ServicesInterface,
	authMiddleware auth.JWTMiddleware,
	authzMiddleware auth.AuthorizationMiddleware,
) {
	for name, registrationFunc := range routeRegistry {
		registrationFunc(apiV1Router, services, authMiddleware, authzMiddleware)
		_ = name // prevent unused variable warning
	}
}

func (s *apiServer) routes() *mux.Router {
	services := &env().Services

	metadataHandler := handlers.NewMetadataHandler()

	var authMiddleware auth.JWTMiddleware
	authMiddleware = &auth.MiddlewareMock{}
	if env().Config.Server.EnableJWT {
		var err error
		authMiddleware, err = auth.NewAuthMiddleware()
		check(err, "Unable to create auth middleware")
	}
	if authMiddleware == nil {
		check(fmt.Errorf("auth middleware is nil"), "Unable to create auth middleware: missing middleware")
	}

	authzMiddleware := auth.NewAuthzMiddlewareMock()
	// TODO: Create issue to track enabling authorization middleware
	// if env().Config.Server.EnableAuthz {
	// 	var err error
	// 	authzMiddleware, err = auth.NewAuthzMiddleware()
	// 	check(err, "Unable to create authz middleware")
	// }
	// mainRouter is top level "/"
	mainRouter := mux.NewRouter()
	mainRouter.NotFoundHandler = http.HandlerFunc(api.SendNotFound)

	// Request ID middleware sets a unique request ID in the context of each request for tracing
	mainRouter.Use(logger.RequestIDMiddleware)

	// OpenTelemetry middleware (conditionally enabled)
	// Extracts trace_id/span_id from traceparent header and adds to logger context
	if env().Config.Logging.OTel.Enabled {
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

	registerApiMiddleware(apiV1Router)

	// Auto-discovered routes (no manual editing needed)
	LoadDiscoveredRoutes(apiV1Router, services, authMiddleware, authzMiddleware)

	return mainRouter
}

func registerApiMiddleware(router *mux.Router) {
	router.Use(MetricsMiddleware)

	// Schema validation middleware (validates spec fields for all resources)
	// Use background context for initialization logging
	ctx := context.Background()

	// Check if an external schema file is specified (for production with provider-specific schemas)
	schemaPath := os.Getenv("OPENAPI_SCHEMA_PATH")

	var schemaValidator *validators.SchemaValidator
	var err error

	if schemaPath != "" {
		// Production: Load schema from file (Helm sets OPENAPI_SCHEMA_PATH=/etc/hyperfleet/schemas/openapi.yaml)
		schemaValidator, err = validators.NewSchemaValidator(schemaPath)
		if err != nil {
			logger.With(ctx, logger.FieldSchemaPath, schemaPath).WithError(err).Warn("Failed to load schema validator from file")
		} else {
			logger.With(ctx, logger.FieldSchemaPath, schemaPath).Info("Schema validation enabled from file")
		}
	} else {
		// Default: Generate schema dynamically from CRD registry
		spec := openapi.GenerateSpec(crd.Default())
		schemaValidator, err = validators.NewSchemaValidatorFromSpec(spec)
		if err != nil {
			logger.WithError(ctx, err).Warn("Failed to create schema validator from generated spec")
		} else {
			logger.Info(ctx, "Schema validation enabled from dynamically generated spec")
		}
	}

	if schemaValidator != nil {
		router.Use(middleware.SchemaValidationMiddleware(schemaValidator))
	} else {
		logger.Warn(ctx, "Schema validation is disabled. Spec fields will not be validated.")
	}

	router.Use(
		func(next http.Handler) http.Handler {
			return db.TransactionMiddleware(next, env().Database.SessionFactory)
		},
	)

	router.Use(gorillahandlers.CompressHandler)
}
