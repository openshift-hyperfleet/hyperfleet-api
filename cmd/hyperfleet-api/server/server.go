package server

import (
	"context"
	"net/http"
	"strings"
)

type Server interface {
	Start() error
	Shutdown(context.Context) error
	Close() error
}

func removeTrailingSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.URL.Path = strings.TrimSuffix(r.URL.Path, "/")
		next.ServeHTTP(w, r)
	})
}
