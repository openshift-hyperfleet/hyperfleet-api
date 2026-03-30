package db

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/response"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_context"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// isWriteMethod returns true if the HTTP method requires a database transaction.
// Write methods (POST, PUT, PATCH, DELETE) return true and will get transactional
// database connections. All other methods (primarily GET) return false and bypass
// transaction creation for improved performance.
//
// The method is normalized to uppercase to handle case variations, though RFC 7231
// specifies that HTTP methods are case-sensitive and should be uppercase.
func isWriteMethod(method string) bool {
	// Normalize to uppercase to handle case variations (defensive)
	method = strings.ToUpper(method)

	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

// TransactionMiddleware creates a database transaction for write operations only.
//
// Write methods (POST/PUT/PATCH/DELETE) get GORM transactions for ACID guarantees.
// Read methods (GET) skip transaction creation for performance, reducing connection
// pool pressure and latency under high adapter polling load.
//
// Trade-off: List operations (COUNT + SELECT) may show inconsistent pagination
// totals under concurrent deletes, but this is an acceptable cosmetic issue.
//
// The requestTimeout is applied to all requests (read and write) to prevent
// queries from blocking indefinitely when the database is under pressure.
func TransactionMiddleware(next http.Handler, connection SessionFactory, requestTimeout time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		if requestTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, requestTimeout)
			defer cancel()
		}

		if isWriteMethod(r.Method) {
			var err error
			ctx, err = NewContext(ctx, connection)
			if err != nil {
				logger.WithError(ctx, err).Error("Could not create transaction")
				// use default error to avoid exposing internals to users
				serviceErr := errors.GeneralError("")
				traceID, _ := logger.GetRequestID(ctx)
				response.WriteProblemDetailsResponse(w, r, serviceErr.HTTPCode, serviceErr.AsProblemDetails(r.URL.Path, traceID))
				return
			}

			// Bridge: Get real DB transaction ID and set to logger context
			if txID, ok := db_context.TxID(ctx); ok {
				ctx = logger.WithTransactionID(ctx, txID)
			}

			*r = *r.WithContext(ctx)
			// Capture context for defer to ensure we resolve the correct transaction context
			txCtx := ctx
			defer func() { Resolve(txCtx) }()

			next.ServeHTTP(w, r)
		} else {
			// Read operations: apply timeout context but skip transaction creation
			*r = *r.WithContext(ctx)
			next.ServeHTTP(w, r)
		}
	})
}
