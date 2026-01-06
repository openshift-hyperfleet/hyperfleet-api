package server

import (
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
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
	if err != nil && err != http.ErrServerClosed {
		slog.Error(msg, "error", err)
		os.Exit(1)
	}
}
