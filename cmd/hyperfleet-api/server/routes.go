package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server/logging"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/middleware"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/tenant"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validators"
)

// APIBasePath is the root path of the HyperFleet API, below the host.
const APIBasePath = "/api/hyperfleet"

// APIV1BasePath is the root path of the v1 HyperFleet API.
const APIV1BasePath = APIBasePath + "/v1"

type ServicesInterface interface {
	GetService(name string) any
}

type RouteRegistrationFunc func(
	apiV1Router *Router,
	services ServicesInterface,
)

var routeRegistry = make(map[string]RouteRegistrationFunc)

func RegisterRoutes(name string, registrationFunc RouteRegistrationFunc) {
	routeRegistry[name] = registrationFunc
}

// LoadDiscoveredRoutes invokes all registered route registration functions.
//
// Note: All routes must use a method-prefixed pattern (e.g. "GET /path") to restrict HTTP methods.
func LoadDiscoveredRoutes(
	apiV1Router *Router,
	services ServicesInterface,
) {
	for name, registrationFunc := range routeRegistry {
		registrationFunc(apiV1Router, services)
		_ = name // prevent unused variable warning
	}
}

func (s *apiServer) routes(tracingEnabled bool) *Router {
	services := &env().Services

	metadataHandler := handlers.NewMetadataHandler()

	// mainRouter is top level "/"
	mainRouter := NewRouter()

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
	apiRouter := mainRouter.Group()
	apiRouter.HandleFunc("GET "+APIBasePath, metadataHandler.Get)

	//  /api/hyperfleet/v1
	apiV1Router := apiRouter.Group()

	err := registerAPIMiddleware(apiV1Router)
	check(err, "Failed to initialize API middleware")

	if env().Config.Server.JWT.Enabled {
		callerIdentityMW := auth.NewCallerIdentityMiddleware()
		apiV1Router.Use(callerIdentityMW.ResolveCallerIdentity)
	}

	// Tenant enforcement: resolves gateway-injected tenant identity into the
	// request context; the DAO layer scopes all queries to it.
	if env().Config.Tenant != nil && env().Config.Tenant.Enabled {
		tenantMW := tenant.NewMiddleware(*env().Config.Tenant)
		apiV1Router.Use(tenantMW.EnforceTenant)
	}

	//  /api/hyperfleet/v1/openapi
	openapiHandler, err := handlers.NewOpenAPIHandler()
	check(err, "Unable to create OpenAPI handler")
	apiV1Router.HandleFunc("GET "+APIV1BasePath+"/openapi.html", openapiHandler.GetOpenAPIUI)
	apiV1Router.HandleFunc("GET "+APIV1BasePath+"/openapi", openapiHandler.GetOpenAPI)

	// Auto-discovered routes (no manual editing needed)
	LoadDiscoveredRoutes(apiV1Router, services)

	return mainRouter
}

func registerAPIMiddleware(router *Router) error {
	router.Use(MetricsMiddleware)

	registry.Validate()

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

	router.Use(CompressMiddleware)

	return nil
}
