package logger

import (
	"log/slog"
	"net/http"
	"time"
)

// HTTPMethod returns a slog attribute for HTTP method
func HTTPMethod(method string) slog.Attr {
	return slog.String("method", method)
}

// HTTPPath returns a slog attribute for HTTP path
func HTTPPath(path string) slog.Attr {
	return slog.String("path", path)
}

// HTTPStatusCode returns a slog attribute for HTTP status code
func HTTPStatusCode(code int) slog.Attr {
	return slog.Int("status_code", code)
}

// HTTPDuration returns a slog attribute for HTTP request duration in milliseconds
func HTTPDuration(d time.Duration) slog.Attr {
	return slog.Int64("duration_ms", d.Milliseconds())
}

// HTTPUserAgent returns a slog attribute for HTTP user agent
func HTTPUserAgent(ua string) slog.Attr {
	return slog.String("user_agent", ua)
}

// HTTPRequestAttrs returns a slice of slog attributes for HTTP request
func HTTPRequestAttrs(r *http.Request) []slog.Attr {
	return []slog.Attr{
		HTTPMethod(r.Method),
		HTTPPath(r.URL.Path),
		HTTPUserAgent(r.UserAgent()),
	}
}

// HTTPResponseAttrs returns a slice of slog attributes for HTTP response
func HTTPResponseAttrs(statusCode int, duration time.Duration) []slog.Attr {
	return []slog.Attr{
		HTTPStatusCode(statusCode),
		HTTPDuration(duration),
	}
}
