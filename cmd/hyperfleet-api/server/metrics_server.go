package server

import (
	"context"
	"fmt"
	"net"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

func NewMetricsServer() Server {
	mainRouter := mux.NewRouter()
	mainRouter.NotFoundHandler = http.HandlerFunc(api.SendNotFound)

	// metrics endpoint only (health endpoints moved to health_server.go on port 8080)
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
	ctx := context.Background()
	var err error
	if env().Config.Metrics.EnableHTTPS {
		if env().Config.Server.HTTPSCertFile == "" || env().Config.Server.HTTPSKeyFile == "" {
			check(
				fmt.Errorf("unspecified required --https-cert-file, --https-key-file"),
				"Can't start https server",
			)
		}

		logger.With(ctx, logger.FieldBindAddress, env().Config.Metrics.BindAddress).Info("Serving Metrics with TLS")
		err = s.httpServer.ListenAndServeTLS(env().Config.Server.HTTPSCertFile, env().Config.Server.HTTPSKeyFile)
	} else {
		logger.With(ctx, logger.FieldBindAddress, env().Config.Metrics.BindAddress).Info("Serving Metrics without TLS")
		err = s.httpServer.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		check(err, "Metrics server terminated with errors")
	} else {
		logger.Info(ctx, "Metrics server terminated")
	}
}

func (s metricsServer) Stop() error {
	return s.httpServer.Shutdown(context.Background())
}
