package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
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
func (s *APIServer) Serve(listener net.Listener) {
	ctx := context.Background()
	var err error
	if s.cfg.TLSEnabled() {
		if s.cfg.TLSCertFile() == "" || s.cfg.TLSKeyFile() == "" {
			check(
				fmt.Errorf(
					"HTTPS certificate or key not configured; "+
						"set via server.tls.cert_file/key_file in config file, env vars, or flags",
				),
				"Can't start https server",
			)
		}

		logger.With(ctx, logger.FieldBindAddress, s.cfg.BindAddress()).Info("Serving with TLS")
		err = s.httpServer.ServeTLS(listener, s.cfg.TLSCertFile(), s.cfg.TLSKeyFile())
	} else {
		logger.With(ctx, logger.FieldBindAddress, s.cfg.BindAddress()).Info("Serving without TLS")
		err = s.httpServer.Serve(listener)
	}

	if err != nil && err != http.ErrServerClosed {
		check(err, "Web server terminated with errors")
	} else {
		logger.Info(ctx, "Web server terminated")
	}
}

// Listen only start the listener, not the server.
// Useful for breaking up ListenAndServer (Start) when you require the server to be listening before continuing
func (s *APIServer) Listen() (listener net.Listener, err error) {
	return net.Listen("tcp", s.cfg.BindAddress())
}

// Start listening on the configured port and start the server.
// This is a convenience wrapper for Listen() and Serve(listener Listener)
func (s *APIServer) Start() {
	ctx := context.Background()
	listener, err := s.Listen()
	if err != nil {
		logger.WithError(ctx, err).Error(fmt.Sprintf("Unable to start API server on %s", s.cfg.BindAddress()))
		os.Exit(1)
	}
	s.Serve(listener)
}

func (s *APIServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}
