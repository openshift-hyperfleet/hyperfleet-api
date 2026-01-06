package logger

import (
	"log/slog"
	"net/http"
	"time"
)

// HTTP request logging helpers per HyperFleet Logging Specification.
// These functions create slog attributes for API-specific fields:
//   - method: HTTP method (GET, POST, etc.)
//   - path: Request path
//   - status_code: HTTP response status code
//   - duration_ms: Request duration in milliseconds
//   - user_agent: Client user agent string

// HTTPMethod returns an slog attribute for the HTTP method
func HTTPMethod(method string) slog.Attr {
	return slog.String("method", method)
}

// HTTPPath returns an slog attribute for the request path
func HTTPPath(path string) slog.Attr {
	return slog.String("path", path)
}

// HTTPStatusCode returns an slog attribute for the response status code
func HTTPStatusCode(code int) slog.Attr {
	return slog.Int("status_code", code)
}

// HTTPDuration returns an slog attribute for request duration in milliseconds
func HTTPDuration(d time.Duration) slog.Attr {
	return slog.Int64("duration_ms", d.Milliseconds())
}

// HTTPUserAgent returns an slog attribute for the user agent
func HTTPUserAgent(ua string) slog.Attr {
	return slog.String("user_agent", ua)
}

// HTTPRequestAttrs returns common HTTP request attributes for logging
func HTTPRequestAttrs(r *http.Request) []slog.Attr {
	return []slog.Attr{
		HTTPMethod(r.Method),
		HTTPPath(r.URL.Path),
		HTTPUserAgent(r.UserAgent()),
	}
}

// HTTPResponseAttrs returns HTTP response attributes for logging
func HTTPResponseAttrs(statusCode int, duration time.Duration) []slog.Attr {
	return []slog.Attr{
		HTTPStatusCode(statusCode),
		HTTPDuration(duration),
	}
}
