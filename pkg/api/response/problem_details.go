package response

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// WriteProblemDetailsResponse writes an RFC 9457 Problem Details response
func WriteProblemDetailsResponse(w http.ResponseWriter, r *http.Request, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.Header().Set("Vary", "Authorization")

	w.WriteHeader(code)

	if payload != nil {
		response, err := json.Marshal(payload)
		if err != nil {
			logResponseError(r.Context(), r, code, "Failed to marshal Problem Details response payload", err)
			return
		}
		if _, err := w.Write(response); err != nil {
			logResponseError(r.Context(), r, code, "Failed to write Problem Details response body", err)
			return
		}
	}
}

func logResponseError(ctx context.Context, r *http.Request, code int, message string, err error) {
	logger.With(ctx,
		logger.HTTPPath(r.URL.Path),
		logger.HTTPMethod(r.Method),
		logger.HTTPStatusCode(code),
	).WithError(err).Error(message)
}
