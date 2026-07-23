package servecmd

import (
	"fmt"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	requestlogging "github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server/logging"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/middleware"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validators"
)

// BuildAPIServer assembles the middleware chains and route registrars, then
// wires them into a router and HTTP server.
//
// Middleware slices that depend on runtime decisions (tracing on/off, auth
// on/off) are built here rather than inside the server package, so that package
// stays free of both pkg/config and those decisions.
//
// jwtHandler may be nil when cfg.Server.JWT.Enabled is false. When JWT is
// enabled, jwtHandler must be non-nil or BuildAPIServer returns an error.
func BuildAPIServer(
	cfg *config.ApplicationConfig,
	resourceService services.ResourceService,
	adapterStatusService services.AdapterStatusService,
	schemaValidator *validators.SchemaValidator,
	jwtHandler *auth.JWTHandler,
	sessionFactory db.SessionFactory,
	tracingEnabled bool,
) (*server.APIServer, error) {
	mainMiddleware := []server.Middleware{logger.RequestIDMiddleware}
	if tracingEnabled {
		mainMiddleware = append(mainMiddleware, middleware.OTelMiddleware)
	}
	masker := middleware.NewMaskingMiddleware(cfg.Logging)
	mainMiddleware = append(mainMiddleware, requestlogging.RequestLoggingMiddleware(masker))

	// Applied to every API route (public and protected) - observability and compression.
	apiMiddleware := []server.Middleware{
		server.MetricsMiddleware,
		server.CompressMiddleware,
	}

	// Applied only behind auth - schema validation and DB transaction.
	protectedAPIMiddleware := []server.Middleware{
		middleware.SchemaValidationMiddleware(schemaValidator),
		func(next http.Handler) http.Handler {
			return db.TransactionMiddleware(next, sessionFactory, cfg.Database.Pool.RequestTimeout)
		},
	}

	var authMiddleware []server.Middleware
	if cfg.Server.JWT.Enabled {
		if jwtHandler == nil {
			return nil, fmt.Errorf("JWT authentication is enabled but no JWT handler was provided")
		}
		callerIdentityMiddleware := auth.NewCallerIdentityMiddleware()
		authMiddleware = append(
			authMiddleware,
			jwtHandler.Middleware,
			callerIdentityMiddleware.ResolveCallerIdentity,
		)
	}

	registrars := []server.RouteRegistrar{
		server.NewEntityRouteRegistrar(resourceService, adapterStatusService, schemaValidator),
	}

	router, err := server.NewRouterFromConfig(
		mainMiddleware, apiMiddleware, protectedAPIMiddleware, authMiddleware, registrars,
	)
	if err != nil {
		return nil, err
	}

	return server.NewAPIServer(cfg.Server, router), nil
}
