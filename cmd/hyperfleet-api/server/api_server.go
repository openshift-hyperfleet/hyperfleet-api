package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	_ "github.com/auth0/go-jwt-middleware"
	_ "github.com/golang-jwt/jwt/v4"
	gorillahandlers "github.com/gorilla/handlers"
	"github.com/openshift-online/ocm-sdk-go/authentication"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

type apiServer struct {
	httpServer *http.Server
}

var _ Server = &apiServer{}

func env() *environments.Env {
	return environments.Environment()
}

func NewAPIServer() Server {
	s := &apiServer{}

	mainRouter := s.routes()

	// referring to the router as type http.Handler allows us to add middleware via more handlers
	var mainHandler http.Handler = mainRouter

	if env().Config.Server.EnableJWT {
		// Create the logger for the authentication handler using slog bridge
		authnLogger := logger.NewOCMLoggerBridge()

		// Create the handler that verifies that tokens are valid:
		var err error
		mainHandler, err = authentication.NewHandler().
			Logger(authnLogger).
			KeysFile(env().Config.Server.JwkCertFile).
			KeysURL(env().Config.Server.JwkCertURL).
			ACLFile(env().Config.Server.ACLFile).
			Public("^/api/hyperfleet/?$").
			Public("^/api/hyperfleet/v1/?$").
			Public("^/api/hyperfleet/v1/openapi/?$").
			Public("^/api/hyperfleet/v1/openapi.html/?$").
			Public("^/api/hyperfleet/v1/errors(/.*)?$").
			Next(mainHandler).
			Build()
		check(err, "Unable to create authentication handler")
	}

	// Configure CORS for Red Hat console and API access
	mainHandler = gorillahandlers.CORS(
		gorillahandlers.AllowedOrigins([]string{
			// OCM UI local development URLs
			"https://qa.foo.redhat.com:1337",
			"https://prod.foo.redhat.com:1337",
			"https://ci.foo.redhat.com:1337",
			// Production and staging console URLs
			"https://console.redhat.com",
			"https://qaprodauth.console.redhat.com",
			"https://qa.console.redhat.com",
			"https://ci.console.redhat.com",
			"https://console.stage.redhat.com",
			// API docs UI
			"https://api.stage.openshift.com",
			"https://api.openshift.com",
			// Customer portal
			"https://access.qa.redhat.com",
			"https://access.stage.redhat.com",
			"https://access.redhat.com",
		}),
		gorillahandlers.AllowedMethods([]string{
			http.MethodDelete,
			http.MethodGet,
			http.MethodPatch,
			http.MethodPost,
		}),
		gorillahandlers.AllowedHeaders([]string{
			"Authorization",
			"Content-Type",
		}),
		gorillahandlers.MaxAge(int((10 * time.Minute).Seconds())),
	)(mainHandler)

	mainHandler = removeTrailingSlash(mainHandler)

	s.httpServer = &http.Server{
		Addr:    env().Config.Server.BindAddress,
		Handler: mainHandler,
	}

	return s
}

// Serve start the blocking call to Serve.
// Useful for breaking up ListenAndServer (Start) when you require the server to be listening before continuing
func (s apiServer) Serve(listener net.Listener) {
	ctx := context.Background()
	var err error
	if env().Config.Server.EnableHTTPS {
		// Check https cert and key path path
		if env().Config.Server.HTTPSCertFile == "" || env().Config.Server.HTTPSKeyFile == "" {
			check(
				fmt.Errorf("unspecified required --https-cert-file, --https-key-file"),
				"Can't start https server",
			)
		}

		// Serve with TLS
		logger.Info(ctx, "Serving with TLS", "bind_address", env().Config.Server.BindAddress)
		err = s.httpServer.ServeTLS(listener, env().Config.Server.HTTPSCertFile, env().Config.Server.HTTPSKeyFile)
	} else {
		logger.Info(ctx, "Serving without TLS", "bind_address", env().Config.Server.BindAddress)
		err = s.httpServer.Serve(listener)
	}

	// Web server terminated.
	check(err, "Web server terminated with errors")
	logger.Info(ctx, "Web server terminated")
}

// Listen only start the listener, not the server.
// Useful for breaking up ListenAndServer (Start) when you require the server to be listening before continuing
func (s apiServer) Listen() (listener net.Listener, err error) {
	return net.Listen("tcp", env().Config.Server.BindAddress)
}

// Start listening on the configured port and start the server. This is a convenience wrapper for Listen() and Serve(listener Listener)
func (s apiServer) Start() {
	ctx := context.Background()
	listener, err := s.Listen()
	if err != nil {
		logger.Error(ctx, "Unable to start API server", "error", err)
		os.Exit(1)
	}
	s.Serve(listener)

	// after the server exits but before the application terminates
	// we need to explicitly close Go's sql connection pool.
	// this needs to be called *exactly* once during an app's lifetime.
	if err := env().Database.SessionFactory.Close(); err != nil {
		logger.Error(ctx, "Error closing database connection", "error", err)
	}
}

func (s apiServer) Stop() error {
	return s.httpServer.Shutdown(context.Background())
}
