package logging

import (
	"bufio"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/middleware"
)

func RequestLoggingMiddleware(masker *middleware.MaskingMiddleware) func(http.Handler) http.Handler {
	return func(handler http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			if strings.TrimSuffix(r.URL.Path, "/") == "/healthcheck" {
				handler.ServeHTTP(w, r)
				return
			}

			var maskedHeaders http.Header
			if masker != nil {
				maskedHeaders = masker.MaskHeaders(r.Header)
			} else {
				maskedHeaders = r.Header
			}

			logger.With(ctx,
				logger.HTTPMethod(r.Method),
				logger.HTTPPath(r.URL.Path),
				slog.String("remote_addr", r.RemoteAddr),
				logger.HTTPUserAgent(r.UserAgent()),
				slog.Any("headers", maskedHeaders),
			).Info("HTTP request received")

			rw := &responseWriter{ResponseWriter: w}

			start := time.Now()
			handler.ServeHTTP(rw, r)
			duration := time.Since(start)

			if rw.statusCode == 0 {
				rw.statusCode = http.StatusOK
			}

			logger.With(ctx,
				logger.HTTPMethod(r.Method),
				logger.HTTPPath(r.URL.Path),
				logger.HTTPStatusCode(rw.statusCode),
				logger.HTTPDuration(duration),
				slog.String("remote_addr", r.RemoteAddr),
				logger.HTTPUserAgent(r.UserAgent()),
			).Info("HTTP request completed")
		})
	}
}

type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code before delegating to the underlying ResponseWriter.
func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Write sets statusCode to 200 if not already set, then delegates to the underlying ResponseWriter.
func (rw *responseWriter) Write(b []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	return rw.ResponseWriter.Write(b)
}

// Flush implements http.Flusher for streaming responses.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack implements http.Hijacker for WebSocket upgrades.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := rw.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("hijacking not supported")
}

// Push implements http.Pusher for HTTP/2 server push.
func (rw *responseWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := rw.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return errors.New("push not supported")
}

// ReadFrom implements io.ReaderFrom for efficient file serving.
func (rw *responseWriter) ReadFrom(src io.Reader) (int64, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	if rf, ok := rw.ResponseWriter.(io.ReaderFrom); ok {
		return rf.ReadFrom(src)
	}
	return io.Copy(rw.ResponseWriter, src)
}
