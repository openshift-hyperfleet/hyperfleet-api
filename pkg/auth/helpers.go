package auth

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

func handleError(ctx context.Context, w http.ResponseWriter, code errors.ServiceErrorCode, reason string) {
	operationID := logger.GetOperationID(ctx)
	err := errors.New(code, "%s", reason)
	if err.HttpCode >= 400 && err.HttpCode <= 499 {
		logger.Info(ctx, err.Error())
	} else {
		logger.Error(ctx, err.Error())
	}

	writeJSONResponse(w, err.HttpCode, err.AsOpenapiError(operationID))
}

func writeJSONResponse(w http.ResponseWriter, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
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
