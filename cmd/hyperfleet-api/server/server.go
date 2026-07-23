package server

import (
	"context"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

type Server interface {
	Start()
	Stop() error
	Listen() (net.Listener, error)
	Serve(net.Listener)
}

// ListenNotifier is an optional interface that servers can implement
// to signal when they are ready to accept connections
type ListenNotifier interface {
	NotifyListening() <-chan struct{}
}

// TODO(HYPERFLEET-1371): env() is the last caller of the global environments
// singleton in this package (used by health_server.go and metrics_server.go).
// APIServer already takes its config via constructor injection (see cfg in
// api_server.go); HealthServer/MetricsServer should follow the same pattern
// once the environments/ package is removed.
func env() *environments.Env {
	return environments.Environment()
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
		logger.WithError(ctx, err).Error(msg)
		os.Exit(1)
	}
}
