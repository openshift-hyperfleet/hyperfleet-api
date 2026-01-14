package auth

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

func handleError(ctx context.Context, w http.ResponseWriter, r *http.Request, code errors.ServiceErrorCode, reason string) {
	traceID, ok := logger.GetRequestID(ctx)
	if !ok {
		traceID = "unknown"
	}
	err := errors.New(code, "%s", reason)
	instance := ""
	if r != nil {
		instance = r.URL.Path
	}
	if err.HttpCode >= 400 && err.HttpCode <= 499 {
		logger.WithError(ctx, err).Warn("Client error occurred")
	} else {
		logger.WithError(ctx, err).Error("Server error occurred")
	}

	writeProblemDetailsResponse(w, err.HttpCode, err.AsProblemDetails(instance, traceID))
}

func writeProblemDetailsResponse(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(code)

	if payload != nil {
		response, err := json.Marshal(payload)
		if err != nil {
			// Headers already sent, can't change status code
			return
		}
		if _, err := w.Write(response); err != nil {
			// Writing failed, nothing we can do at this point
			return
		}
	}
}
