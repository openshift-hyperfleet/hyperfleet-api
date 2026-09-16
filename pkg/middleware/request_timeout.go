package middleware

import (
	"context"
	"net/http"
	"time"
)

// RequestTimeoutMiddleware applies the configured request timeout to every
// protected request. Services own database transaction boundaries.
func RequestTimeoutMiddleware(requestTimeout time.Duration) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if requestTimeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, requestTimeout)
				defer cancel()
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
