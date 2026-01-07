package logging

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/middleware"
)

// RequestLoggingMiddleware logs HTTP request and response information using slog
// Automatically masks sensitive headers and body fields using the provided masker
func RequestLoggingMiddleware(masker *middleware.MaskingMiddleware) func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Skip health check endpoint to reduce log spam
			if strings.TrimSuffix(r.URL.Path, "/") == "/api/hyperfleet" {
				handler.ServeHTTP(w, r)
				return
			}

			// Mask sensitive headers
			maskedHeaders := masker.MaskHeaders(r.Header)

			// Log incoming request
			logger.Info(ctx, "HTTP request received",
				logger.HTTPMethod(r.Method),
				logger.HTTPPath(r.URL.Path),
				slog.String("remote_addr", r.RemoteAddr),
				logger.HTTPUserAgent(r.UserAgent()),
				slog.Any("headers", maskedHeaders),
			)

			// Wrap response writer to capture status code
			rw := &responseWriter{ResponseWriter: w}

			// Execute handler and measure duration
			start := time.Now()
			handler.ServeHTTP(rw, r)
			duration := time.Since(start)

			// Log response
			logger.Info(ctx, "HTTP request completed",
				logger.HTTPMethod(r.Method),
				logger.HTTPPath(r.URL.Path),
				logger.HTTPStatusCode(rw.statusCode),
				logger.HTTPDuration(duration),
				slog.String("remote_addr", r.RemoteAddr),
				logger.HTTPUserAgent(r.UserAgent()),
			)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code before writing
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write captures the implicit 200 status code when WriteHeader wasn't called
func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	return rw.ResponseWriter.Write(b)
}
