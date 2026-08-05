package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

type metricsCfg interface {
	BindAddress() string
	TLSEnabled() bool
	TLSCertFile() string
	TLSKeyFile() string
}

func NewMetricsServer(cfg metricsCfg) Server {
	mainRouter := http.NewServeMux()

	prometheusMetricsHandler := handlers.NewPrometheusMetricsHandler()
	mainRouter.Handle("GET /metrics", prometheusMetricsHandler.Handler())

	mainHandler := WithNotFoundHandler(mainRouter)

	s := &metricsServer{cfg: cfg}
	s.httpServer = &http.Server{
		Addr:              cfg.BindAddress(),
		Handler:           mainHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

type metricsServer struct {
	cfg        metricsCfg
	httpServer *http.Server
}

var _ Server = &metricsServer{}

func (s *metricsServer) Listen() (net.Listener, error) {
	return net.Listen("tcp", s.httpServer.Addr)
}

func (s *metricsServer) Serve(listener net.Listener) error {
	ctx := context.Background()
	var err error
	if s.cfg.TLSEnabled() {
		if s.cfg.TLSCertFile() == "" || s.cfg.TLSKeyFile() == "" {
			_ = listener.Close()
			return fmt.Errorf("unspecified required --https-cert-file, --https-key-file")
		}

		logger.With(ctx, logger.FieldBindAddress, s.httpServer.Addr).Info("Serving Metrics with TLS")
		err = s.httpServer.ServeTLS(listener, s.cfg.TLSCertFile(), s.cfg.TLSKeyFile())
	} else {
		logger.With(ctx, logger.FieldBindAddress, s.httpServer.Addr).Info("Serving Metrics without TLS")
		err = s.httpServer.Serve(listener)
	}
	if err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("metrics server terminated with errors: %w", err)
	}
	logger.Info(ctx, "Metrics server terminated")
	return nil
}

func (s *metricsServer) Start() error {
	listener, err := s.Listen()
	if err != nil {
		return fmt.Errorf("failed to create metrics server listener: %w", err)
	}
	return s.Serve(listener)
}

func (s *metricsServer) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *metricsServer) Close() error {
	return s.httpServer.Close()
}
