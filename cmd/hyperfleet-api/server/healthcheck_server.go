package server

import (
	"context"
	"fmt"
	"net"
	"net/http"

	health "github.com/docker/go-healthcheck"
	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

var (
	updater = health.NewStatusUpdater()
)

var _ Server = &healthCheckServer{}

type healthCheckServer struct {
	httpServer *http.Server
}

func NewHealthCheckServer() *healthCheckServer {
	router := mux.NewRouter()
	health.DefaultRegistry = health.NewRegistry()
	health.Register("maintenance_status", updater)
	router.HandleFunc("/healthcheck", health.StatusHandler).Methods(http.MethodGet)
	router.HandleFunc("/healthcheck/down", downHandler).Methods(http.MethodPost)
	router.HandleFunc("/healthcheck/up", upHandler).Methods(http.MethodPost)

	srv := &http.Server{
		Handler: router,
		Addr:    env().Config.HealthCheck.BindAddress,
	}

	return &healthCheckServer{
		httpServer: srv,
	}
}

func (s healthCheckServer) Start() {
	ctx := context.Background()
	var err error
	if env().Config.HealthCheck.EnableHTTPS {
		if env().Config.Server.HTTPSCertFile == "" || env().Config.Server.HTTPSKeyFile == "" {
			check(
				fmt.Errorf("unspecified required --https-cert-file, --https-key-file"),
				"Can't start https server",
			)
		}

		// Serve with TLS
		logger.With(ctx, logger.FieldBindAddress, env().Config.HealthCheck.BindAddress).Info("Serving HealthCheck with TLS")
		err = s.httpServer.ListenAndServeTLS(env().Config.Server.HTTPSCertFile, env().Config.Server.HTTPSKeyFile)
	} else {
		logger.With(ctx, logger.FieldBindAddress, env().Config.HealthCheck.BindAddress).Info("Serving HealthCheck without TLS")
		err = s.httpServer.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		check(err, "HealthCheck server terminated with errors")
	} else {
		logger.Info(ctx, "HealthCheck server terminated")
	}
}

func (s healthCheckServer) Stop() error {
	return s.httpServer.Shutdown(context.Background())
}

// Listen Unimplemented
func (s healthCheckServer) Listen() (listener net.Listener, err error) {
	return nil, nil
}

// Serve Unimplemented
func (s healthCheckServer) Serve(listener net.Listener) {
}

func upHandler(w http.ResponseWriter, r *http.Request) {
	updater.Update(nil)
}

func downHandler(w http.ResponseWriter, r *http.Request) {
	updater.Update(fmt.Errorf("maintenance mode"))
}
