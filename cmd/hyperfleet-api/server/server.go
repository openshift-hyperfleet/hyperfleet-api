package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

type Server interface {
	Start() error
	Shutdown(context.Context) error
	Close() error
	NotifyListening() <-chan struct{}
}

type tlsCfg interface {
	TLSEnabled() bool
	TLSCertFile() string
	TLSKeyFile() string
}

type baseServer struct {
	cfg        tlsCfg
	httpServer *http.Server
	listening  chan struct{}
	name       string
}

func (s *baseServer) Listen() (net.Listener, error) {
	return net.Listen("tcp", s.httpServer.Addr)
}

func (s *baseServer) Serve(listener net.Listener) error {
	ctx := context.Background()
	var err error
	if s.cfg.TLSEnabled() {
		if s.cfg.TLSCertFile() == "" || s.cfg.TLSKeyFile() == "" {
			configErr := fmt.Errorf(
				"HTTPS certificate or key not configured; " +
					"set via tls.cert_file/key_file in config file, env vars, or flags",
			)
			return errors.Join(configErr, listener.Close())
		}

		logger.With(ctx, logger.FieldBindAddress, s.httpServer.Addr).Info("Serving " + s.name + " with TLS")
		err = s.httpServer.ServeTLS(listener, s.cfg.TLSCertFile(), s.cfg.TLSKeyFile())
	} else {
		logger.With(ctx, logger.FieldBindAddress, s.httpServer.Addr).Info("Serving " + s.name + " without TLS")
		err = s.httpServer.Serve(listener)
	}
	if err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("%s terminated with errors: %w", s.name, err)
	}
	logger.Info(ctx, s.name+" terminated")
	return nil
}

func (s *baseServer) Start() error {
	listener, err := s.Listen()
	if err != nil {
		return fmt.Errorf("unable to start %s on %s: %w", s.name, s.httpServer.Addr, err)
	}
	if s.cfg.TLSEnabled() {
		if _, err := tls.LoadX509KeyPair(s.cfg.TLSCertFile(), s.cfg.TLSKeyFile()); err != nil {
			return errors.Join(
				fmt.Errorf("%s: invalid TLS certificate/key: %w", s.name, err),
				listener.Close(),
			)
		}
	}
	close(s.listening)
	return s.Serve(listener)
}

func (s *baseServer) NotifyListening() <-chan struct{} {
	return s.listening
}

func (s *baseServer) Shutdown(ctx context.Context) error {
	if err := s.httpServer.Shutdown(ctx); err != nil {
		return fmt.Errorf("%s shutdown: %w", s.name, err)
	}
	return nil
}

func (s *baseServer) Close() error {
	if err := s.httpServer.Close(); err != nil {
		return fmt.Errorf("%s close: %w", s.name, err)
	}
	return nil
}

func removeTrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimSuffix(r.URL.Path, "/")
		next.ServeHTTP(w, r)
	})
}
