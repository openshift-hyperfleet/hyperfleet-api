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
func BuildAPIServer(
	cfg *config.ApplicationConfig,
	resourceService services.ResourceService,
	adapterStatusService services.AdapterStatusService,
	schemaValidator *validators.SchemaValidator,
	jwtHandler *auth.JWTHandler,
	sessionFactory db.SessionFactory,
) (*server.APIServer, error) {
	mainMiddleware := []server.Middleware{logger.RequestIDMiddleware}
	if cfg.Tracing.Enabled {
		mainMiddleware = append(mainMiddleware, middleware.OTelMiddleware)
	}
	masker := middleware.NewMaskingMiddleware(cfg.Logging)
	mainMiddleware = append(mainMiddleware, requestlogging.RequestLoggingMiddleware(masker))

	apiMiddleware := []server.Middleware{
		server.MetricsMiddleware,
		server.CompressMiddleware,
	}

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
