package db

import (
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/response"
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
			response.WriteProblemDetailsResponse(w, r, serviceErr.HttpCode, serviceErr.AsProblemDetails(r.URL.Path, traceID))
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
