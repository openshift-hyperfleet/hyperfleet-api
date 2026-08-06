package server

import (
	"net/http"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
)

type metricsCfg interface {
	tlsCfg
	BindAddress() string
}

func NewMetricsServer(cfg metricsCfg) *MetricsServer {
	mainRouter := http.NewServeMux()

	prometheusMetricsHandler := handlers.NewPrometheusMetricsHandler()
	mainRouter.Handle("GET /metrics", prometheusMetricsHandler.Handler())

	mainHandler := WithNotFoundHandler(mainRouter)

	return &MetricsServer{
		baseServer: baseServer{
			name:      "metrics server",
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
}

type MetricsServer struct {
	baseServer
}
