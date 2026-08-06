package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/health"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

type healthCfg interface {
	BindAddress() string
	TLSEnabled() bool
	TLSCertFile() string
	TLSKeyFile() string
	PingTimeout() time.Duration
}

func NewHealthServer(cfg healthCfg, sessionFactory db.SessionFactory) *HealthServer {
	mainRouter := http.NewServeMux()

	healthHandler := health.NewHandler(sessionFactory, cfg.PingTimeout())
	mainRouter.HandleFunc("GET /healthz", healthHandler.LivenessHandler)
	mainRouter.HandleFunc("GET /readyz", healthHandler.ReadinessHandler)

	mainHandler := WithNotFoundHandler(mainRouter)

	s := &HealthServer{
		cfg:       cfg,
		listening: make(chan struct{}),
	}
	s.httpServer = &http.Server{
		Addr:              cfg.BindAddress(),
		Handler:           mainHandler,
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

type HealthServer struct {
	cfg        healthCfg
	httpServer *http.Server
	listening  chan struct{}
}

var _ Server = &HealthServer{}

func (s *HealthServer) Listen() (listener net.Listener, err error) {
	return net.Listen("tcp", s.httpServer.Addr)
}

func (s *HealthServer) Serve(listener net.Listener) error {
	ctx := context.Background()
	var err error

	if s.cfg.TLSEnabled() {
		if s.cfg.TLSCertFile() == "" || s.cfg.TLSKeyFile() == "" {
			_ = listener.Close()
			return fmt.Errorf("unspecified required --https-cert-file, --https-key-file")
		}

		logger.With(ctx, logger.FieldBindAddress, s.httpServer.Addr).Info("Serving Health with TLS")
		err = s.httpServer.ServeTLS(listener, s.cfg.TLSCertFile(), s.cfg.TLSKeyFile())
	} else {
		logger.With(ctx, logger.FieldBindAddress, s.httpServer.Addr).Info("Serving Health without TLS")
		err = s.httpServer.Serve(listener)
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("health server terminated with errors: %w", err)
	}
	logger.Info(ctx, "Health server terminated")
	return nil
}

// Start is a convenience wrapper that calls Listen() and Serve()
func (s *HealthServer) Start() error {
	listener, err := s.Listen()
	if err != nil {
		return fmt.Errorf("failed to create health server listener: %w", err)
	}

	// Signal that we're listening
	close(s.listening)

	return s.Serve(listener)
}

// NotifyListening returns a channel that is closed when the server is listening
func (s *HealthServer) NotifyListening() <-chan struct{} {
	return s.listening
}

func (s *HealthServer) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *HealthServer) Close() error {
	return s.httpServer.Close()
}
