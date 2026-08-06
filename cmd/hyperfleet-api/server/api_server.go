package server

import (
	"net/http"
	"time"
)

type cfg interface {
	tlsCfg
	BindAddress() string
	ReadTimeout() time.Duration
	WriteTimeout() time.Duration
}

type APIServer struct {
	baseServer
}

func NewAPIServer(cfg cfg, handler http.Handler) *APIServer {
	return &APIServer{
		baseServer: baseServer{
			name:      "API server",
			cfg:       cfg,
			listening: make(chan struct{}),
			httpServer: &http.Server{
				Addr:              cfg.BindAddress(),
				Handler:           removeTrailingSlash(handler),
				ReadTimeout:       cfg.ReadTimeout(),
				WriteTimeout:      cfg.WriteTimeout(),
				ReadHeaderTimeout: 10 * time.Second,
			},
		},
	}
}
