package db

import (
	"encoding/json"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_context"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// TransactionMiddleware creates a new HTTP middleware that begins a database transaction
// and stores it in the request context.
func TransactionMiddleware(next http.Handler, connection SessionFactory) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, err := NewContext(r.Context(), connection)
		if err != nil {
			logger.WithError(r.Context(), err).Error("Could not create transaction")
			// use default error to avoid exposing internals to users
			serviceErr := errors.GeneralError("")
			traceID, _ := logger.GetRequestID(r.Context())
			writeProblemDetailsResponse(w, serviceErr.HttpCode, serviceErr.AsProblemDetails(r.URL.Path, traceID))
			return
		}

		// Bridge: Get real DB transaction ID and set to logger context
		if txID, ok := db_context.TxID(ctx); ok {
			ctx = logger.WithTransactionID(ctx, txID)
		}

		*r = *r.WithContext(ctx)
		defer func() { Resolve(r.Context()) }()

		next.ServeHTTP(w, r)
	})
}

func writeProblemDetailsResponse(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/problem+json")
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
