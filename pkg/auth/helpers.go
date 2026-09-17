package auth

import (
	"context"
	"log/slog"
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
	if err.HTTPCode >= 400 && err.HTTPCode <= 499 {
		slog.WarnContext(ctx, "Client error occurred", "error", err)
	} else {
		slog.ErrorContext(ctx, "Server error occurred", "error", err)
	}

	response.WriteProblemDetailsResponse(w, r, err.HTTPCode, err.AsProblemDetails(instance, traceID))
}
