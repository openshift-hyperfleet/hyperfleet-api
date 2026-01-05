package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	_ "github.com/auth0/go-jwt-middleware"
	_ "github.com/golang-jwt/jwt/v4"
	"github.com/golang/glog"
	gorillahandlers "github.com/gorilla/handlers"
	sdk "github.com/openshift-online/ocm-sdk-go"
	"github.com/openshift-online/ocm-sdk-go/authentication"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
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

	if env().Config.Server.Auth.JWT.Enabled {
		// Create the logger for the authentication handler:
		authnLogger, err := sdk.NewGlogLoggerBuilder().
			InfoV(glog.Level(1)).
			DebugV(glog.Level(5)).
			Build()
		check(err, "Unable to create authentication logger")

		// Create the handler that verifies that tokens are valid:
		mainHandler, err = authentication.NewHandler().
			Logger(authnLogger).
			KeysFile(env().Config.Server.Auth.JWT.CertFile).
			KeysURL(env().Config.Server.Auth.JWT.CertURL).
			ACLFile(env().Config.Server.Auth.Authz.ACLFile).
			Public("^/api/hyperfleet/?$").
			Public("^/api/hyperfleet/config/?$").
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
		Addr:    env().Config.Server.GetBindAddress(),
		Handler: mainHandler,
	}

	return s
}

// Serve start the blocking call to Serve.
// Useful for breaking up ListenAndServer (Start) when you require the server to be listening before continuing
func (s apiServer) Serve(listener net.Listener) {
	var err error
	if env().Config.Server.HTTPS.Enabled {
		// Check https cert and key path path
		if env().Config.Server.HTTPS.CertFile == "" || env().Config.Server.HTTPS.KeyFile == "" {
			check(
				fmt.Errorf("unspecified required --server-https-cert-file, --server-https-key-file"),
				"Can't start https server",
			)
		}

		// Serve with TLS
		glog.Infof("Serving with TLS at %s", env().Config.Server.GetBindAddress())
		err = s.httpServer.ServeTLS(listener, env().Config.Server.HTTPS.CertFile, env().Config.Server.HTTPS.KeyFile)
	} else {
		glog.Infof("Serving without TLS at %s", env().Config.Server.GetBindAddress())
		err = s.httpServer.Serve(listener)
	}

	// Web server terminated.
	check(err, "Web server terminated with errors")
	glog.Info("Web server terminated")
}

// Listen only start the listener, not the server.
// Useful for breaking up ListenAndServer (Start) when you require the server to be listening before continuing
func (s apiServer) Listen() (listener net.Listener, err error) {
	return net.Listen("tcp", env().Config.Server.GetBindAddress())
}

// Start listening on the configured port and start the server. This is a convenience wrapper for Listen() and Serve(listener Listener)
func (s apiServer) Start() {
	listener, err := s.Listen()
	if err != nil {
		glog.Fatalf("Unable to start API server: %s", err)
	}
	s.Serve(listener)

	// after the server exits but before the application terminates
	// we need to explicitly close Go's sql connection pool.
	// this needs to be called *exactly* once during an app's lifetime.
	if err := env().Database.SessionFactory.Close(); err != nil {
		glog.Errorf("Error closing database connection: %v", err)
	}
}

func (s apiServer) Stop() error {
	return s.httpServer.Shutdown(context.Background())
}
