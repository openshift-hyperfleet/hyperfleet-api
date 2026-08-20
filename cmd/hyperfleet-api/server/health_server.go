package server

import (
	"net/http"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/health"
)

type healthCfg interface {
	tlsCfg
	BindAddress() string
	PingTimeout() time.Duration
}

func NewHealthServer(cfg healthCfg, sessionFactory db.SessionFactory) *HealthServer {
	mainRouter := http.NewServeMux()

	healthHandler := health.NewHandler(sessionFactory, cfg.PingTimeout())
	mainRouter.HandleFunc("GET /healthz", healthHandler.LivenessHandler)
	mainRouter.HandleFunc("GET /readyz", healthHandler.ReadinessHandler)

	mainHandler := WithNotFoundHandler(mainRouter)

	s := &HealthServer{
		baseServer: baseServer{
			name:      "health server",
			cfg:       cfg,
			listening: make(chan struct{}),
			httpServer: &http.Server{
				Addr:              cfg.BindAddress(),
				Handler:           mainHandler,
				ReadTimeout:       5 * time.Second,
				WriteTimeout:      10 * time.Second,
				IdleTimeout:       60 * time.Second,
				ReadHeaderTimeout: 10 * time.Second,
			},
		},
	}
	return s
}

type HealthServer struct {
	baseServer
}
