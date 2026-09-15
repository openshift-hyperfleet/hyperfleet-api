package logger

import (
	"fmt"
	"log/slog"
	"net/http"
)

// RequestIDMiddleware Middleware wraps the given HTTP handler so that the details of the request are sent to the log.
func RequestIDMiddleware(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, err := WithRequestID(r.Context())
		if err != nil {
			wrappedErr := fmt.Errorf("request ID middleware: %w", err)
			slog.ErrorContext(r.Context(), "Failed to generate request ID; continuing without it", "error", wrappedErr)
			ctx = r.Context()
		}

		reqID, ok := GetRequestID(ctx)
		if ok && len(reqID) > 0 {
			w.Header().Set(ReqIDHeader, reqID)
		}

		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}
