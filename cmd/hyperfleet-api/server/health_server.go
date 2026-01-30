package server

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/health"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

func NewHealthServer() Server {
	mainRouter := mux.NewRouter()
	mainRouter.NotFoundHandler = http.HandlerFunc(api.SendNotFound)

	// health endpoints (HyperFleet standard)
	healthHandler := health.NewHandler(env().Database.SessionFactory)
	mainRouter.HandleFunc("/healthz", healthHandler.LivenessHandler).Methods(http.MethodGet)
	mainRouter.HandleFunc("/readyz", healthHandler.ReadinessHandler).Methods(http.MethodGet)

	var mainHandler http.Handler = mainRouter

	s := &healthServer{
		shutdownTimeout: env().Config.Health.ShutdownTimeout,
		listening:       make(chan struct{}),
	}
	s.httpServer = &http.Server{
		Addr:              env().Config.Health.BindAddress,
		Handler:           mainHandler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	return s
}

type healthServer struct {
	httpServer      *http.Server
	shutdownTimeout time.Duration
	listening       chan struct{}
}

var _ Server = &healthServer{}

func (s *healthServer) Listen() (listener net.Listener, err error) {
	return net.Listen("tcp", s.httpServer.Addr)
}

func (s *healthServer) Serve(listener net.Listener) {
	ctx := context.Background()
	var err error

	if env().Config.Health.EnableHTTPS {
		if env().Config.Server.HTTPSCertFile == "" || env().Config.Server.HTTPSKeyFile == "" {
			check(
				fmt.Errorf("unspecified required --https-cert-file, --https-key-file"),
				"Can't start https server",
			)
		}

		logger.With(ctx, logger.FieldBindAddress, env().Config.Health.BindAddress).Info("Serving Health with TLS")
		err = s.httpServer.ServeTLS(listener, env().Config.Server.HTTPSCertFile, env().Config.Server.HTTPSKeyFile)
	} else {
		logger.With(ctx, logger.FieldBindAddress, env().Config.Health.BindAddress).Info("Serving Health without TLS")
		err = s.httpServer.Serve(listener)
	}
	if err != nil && err != http.ErrServerClosed {
		check(err, "Health server terminated with errors")
	} else {
		logger.Info(ctx, "Health server terminated")
	}
}

// Start is a convenience wrapper that calls Listen() and Serve()
func (s *healthServer) Start() {
	listener, err := s.Listen()
	if err != nil {
		check(err, "Failed to create health server listener")
		return
	}

	// Signal that we're listening
	close(s.listening)

	s.Serve(listener)
}

// NotifyListening returns a channel that is closed when the server is listening
func (s *healthServer) NotifyListening() <-chan struct{} {
	return s.listening
}

func (s healthServer) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), s.shutdownTimeout)
	defer cancel()
	return s.httpServer.Shutdown(ctx)
}
