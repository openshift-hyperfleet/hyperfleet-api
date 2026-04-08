package db

import (
	"context"
	stderrors "errors"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/response"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// isWriteMethod reports whether method requires a database transaction.
// Write methods (POST, PUT, PATCH, DELETE) return true, others return false.
// Method must be uppercase (e.g., "POST", not "post").
func isWriteMethod(method string) bool {
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
				var serviceErr *errors.ServiceError
				if IsDBConnectionError(err) {
					serviceErr = errors.ServiceUnavailable("Database connection unavailable")
				} else {
					// use default error to avoid exposing internals to users
					serviceErr = errors.GeneralError("")
				}
				traceID, _ := logger.GetRequestID(ctx)
				response.WriteProblemDetailsResponse(w, r, serviceErr.HTTPCode, serviceErr.AsProblemDetails(r.URL.Path, traceID))
				return
			}

			*r = *r.WithContext(ctx)
			defer func() { Resolve(ctx) }()

			next.ServeHTTP(w, r)
		} else {
			// Read operations: apply timeout context but skip transaction creation
			*r = *r.WithContext(ctx)
			next.ServeHTTP(w, r)
		}
	})
}

// IsDBConnectionError indicates whether err is an infrastructure failure
// (network unreachable, connection refused, connection dropped)
func IsDBConnectionError(err error) bool {
	if err == nil {
		return false
	}
	var netErr *net.OpError
	if stderrors.As(err, &netErr) {
		return true
	}
	if stderrors.Is(err, io.EOF) || stderrors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	return false
}
