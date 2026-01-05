package server

import (
	"context"
	"fmt"
	"net"
	"net/http"

	health "github.com/docker/go-healthcheck"
	"github.com/golang/glog"
	"github.com/gorilla/mux"
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
		Addr:    env().Config.HealthCheck.GetBindAddress(),
	}

	return &healthCheckServer{
		httpServer: srv,
	}
}

func (s healthCheckServer) Start() {
	var err error
	if env().Config.HealthCheck.EnableHTTPS {
		if env().Config.Server.HTTPS.CertFile == "" || env().Config.Server.HTTPS.KeyFile == "" {
			check(
				fmt.Errorf("unspecified required --server-https-cert-file, --server-https-key-file"),
				"Can't start https server",
			)
		}

		// Serve with TLS
		glog.Infof("Serving HealthCheck with TLS at %s", env().Config.HealthCheck.GetBindAddress())
		err = s.httpServer.ListenAndServeTLS(env().Config.Server.HTTPS.CertFile, env().Config.Server.HTTPS.KeyFile)
	} else {
		glog.Infof("Serving HealthCheck without TLS at %s", env().Config.HealthCheck.GetBindAddress())
		err = s.httpServer.ListenAndServe()
	}
	check(err, "HealthCheck server terminated with errors")
	glog.Infof("HealthCheck server terminated")
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
