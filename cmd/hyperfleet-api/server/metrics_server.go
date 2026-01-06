package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
)

func NewMetricsServer() Server {
	mainRouter := mux.NewRouter()
	mainRouter.NotFoundHandler = http.HandlerFunc(api.SendNotFound)

	// metrics endpoint
	prometheusMetricsHandler := handlers.NewPrometheusMetricsHandler()
	mainRouter.Handle("/metrics", prometheusMetricsHandler.Handler())

	var mainHandler http.Handler = mainRouter

	s := &metricsServer{}
	s.httpServer = &http.Server{
		Addr:    env().Config.Metrics.BindAddress,
		Handler: mainHandler,
	}
	return s
}

type metricsServer struct {
	httpServer *http.Server
}

var _ Server = &metricsServer{}

func (s metricsServer) Listen() (listener net.Listener, err error) {
	return nil, nil
}

func (s metricsServer) Serve(listener net.Listener) {
}

func (s metricsServer) Start() {
	var err error
	if env().Config.Metrics.EnableHTTPS {
		if env().Config.Server.HTTPSCertFile == "" || env().Config.Server.HTTPSKeyFile == "" {
			check(
				fmt.Errorf("unspecified required --https-cert-file, --https-key-file"),
				"Can't start https server",
			)
		}

		// Serve with TLS
		slog.Info("Serving Metrics with TLS", "address", env().Config.Metrics.BindAddress)
		err = s.httpServer.ListenAndServeTLS(env().Config.Server.HTTPSCertFile, env().Config.Server.HTTPSKeyFile)
	} else {
		slog.Info("Serving Metrics without TLS", "address", env().Config.Metrics.BindAddress)
		err = s.httpServer.ListenAndServe()
	}
	check(err, "Metrics server terminated with errors")
	slog.Info("Metrics server terminated")
}

func (s metricsServer) Stop() error {
	return s.httpServer.Shutdown(context.Background())
}
