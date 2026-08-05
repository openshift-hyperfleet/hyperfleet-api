package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

type cfg interface {
	BindAddress() string
	ReadTimeout() time.Duration
	WriteTimeout() time.Duration
	TLSEnabled() bool
	TLSCertFile() string
	TLSKeyFile() string
}

type APIServer struct {
	cfg        cfg
	httpServer *http.Server
}

func NewAPIServer(cfg cfg, handler http.Handler) *APIServer {
	return &APIServer{
		cfg: cfg,
		httpServer: &http.Server{
			Addr:              cfg.BindAddress(),
			Handler:           removeTrailingSlash(handler),
			ReadTimeout:       cfg.ReadTimeout(),
			WriteTimeout:      cfg.WriteTimeout(),
			ReadHeaderTimeout: 10 * time.Second, // Hardcoded to prevent Slowloris attacks (not user-configurable)
		},
	}
}

// Serve start the blocking call to Serve.
// Useful for breaking up ListenAndServer (Start) when you require the server to be listening before continuing
func (s *APIServer) Serve(listener net.Listener) error {
	ctx := context.Background()
	var err error
	if s.cfg.TLSEnabled() {
		if s.cfg.TLSCertFile() == "" || s.cfg.TLSKeyFile() == "" {
			_ = listener.Close()
			return fmt.Errorf(
				"HTTPS certificate or key not configured; " +
					"set via server.tls.cert_file/key_file in config file, env vars, or flags",
			)
		}

		logger.With(ctx, logger.FieldBindAddress, s.cfg.BindAddress()).Info("Serving with TLS")
		err = s.httpServer.ServeTLS(listener, s.cfg.TLSCertFile(), s.cfg.TLSKeyFile())
	} else {
		logger.With(ctx, logger.FieldBindAddress, s.cfg.BindAddress()).Info("Serving without TLS")
		err = s.httpServer.Serve(listener)
	}

	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("web server terminated with errors: %w", err)
	}
	logger.Info(ctx, "Web server terminated")
	return nil
}

// Listen only start the listener, not the server.
// Useful for breaking up ListenAndServer (Start) when you require the server to be listening before continuing
func (s *APIServer) Listen() (listener net.Listener, err error) {
	return net.Listen("tcp", s.cfg.BindAddress())
}

// Start listening on the configured port and start the server.
// This is a convenience wrapper for Listen() and Serve(listener Listener)
func (s *APIServer) Start() error {
	listener, err := s.Listen()
	if err != nil {
		return fmt.Errorf("unable to start API server on %s: %w", s.cfg.BindAddress(), err)
	}
	return s.Serve(listener)
}

func (s *APIServer) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *APIServer) Close() error {
	return s.httpServer.Close()
}
