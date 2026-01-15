package auth

import (
	"context"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/response"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

func handleError(ctx context.Context, w http.ResponseWriter, r *http.Request, code string, reason string) {
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

	response.WriteProblemDetailsResponse(w, r, err.HttpCode, err.AsProblemDetails(instance, traceID))
}
