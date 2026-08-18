package response

import (
	"context"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// WriteServiceErrorResponse writes err as an RFC 9457 Problem Details response,
// resolving the trace ID from ctx (via logger.GetRequestID) and using the
// request path as the problem instance. ctx is taken explicitly rather than
// derived from r.Context() so callers can still resolve a trace ID when r is
// nil. Callers are responsible for logging the error themselves beforehand,
// since the appropriate log level (e.g. Warn vs Info vs Error) varies by caller.
func WriteServiceErrorResponse(ctx context.Context, w http.ResponseWriter, r *http.Request, err *errors.ServiceError) {
	traceID, ok := logger.GetRequestID(ctx)
	if !ok {
		traceID = "unknown"
	}
	instance := ""
	if r != nil {
		instance = r.URL.Path
	}
	WriteProblemDetailsResponse(w, r, err.HTTPCode, err.AsProblemDetails(instance, traceID))
}
