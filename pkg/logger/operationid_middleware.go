package logger

import (
	"net/http"
)

// OperationIDMiddleware Middleware wraps the given HTTP handler so that the details of the request are sent to the log.
func OperationIDMiddleware(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := WithOpID(r.Context())

		opID, ok := ctx.Value(OpIDKey).(string)
		if ok && len(opID) > 0 {
			w.Header().Set(OpIDHeader, opID)
		}

		handler.ServeHTTP(w, r.WithContext(ctx))
	})
}
