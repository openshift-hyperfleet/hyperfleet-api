package server

import (
	"context"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

type Server interface {
	Start()
	Stop() error
	Listen() (net.Listener, error)
	Serve(net.Listener)
}

func removeTrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimSuffix(r.URL.Path, "/")
		next.ServeHTTP(w, r)
	})
}

// Exit on error
func check(err error, msg string) {
	ctx := context.Background()
	if err != nil && err != http.ErrServerClosed {
		logger.Error(ctx, msg, "error", err)
		os.Exit(1)
	}
}
