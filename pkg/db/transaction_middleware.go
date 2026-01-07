package db

import (
	"encoding/json"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// TransactionMiddleware creates a new HTTP middleware that begins a database transaction
// and stores it in the request context.
func TransactionMiddleware(next http.Handler, connection SessionFactory) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, err := NewContext(r.Context(), connection)
		if err != nil {
			logger.Error(r.Context(), "Could not create transaction", "error", err.Error())
			// use default error to avoid exposing internals to users
			err := errors.GeneralError("")
			operationID := logger.GetOperationID(r.Context())
			writeJSONResponse(w, err.HttpCode, err.AsOpenapiError(operationID))
			return
		}

		*r = *r.WithContext(ctx)
		defer func() { Resolve(r.Context()) }()

		next.ServeHTTP(w, r)
	})
}

func writeJSONResponse(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	if payload != nil {
		response, err := json.Marshal(payload)
		if err != nil {
			// Log error but don't expose to client since headers already sent
			return
		}
		if _, err := w.Write(response); err != nil {
			// Response writing failed, nothing we can do at this point
			return
		}
	}
}
