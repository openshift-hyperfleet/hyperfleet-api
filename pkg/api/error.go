package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// SendNotFound sends a 404 response in RFC 9457 Problem Details format.
func SendNotFound(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/problem+json")

	traceID, _ := logger.GetRequestID(r.Context())
	now := time.Now().UTC()
	detail := fmt.Sprintf("The requested endpoint '%s' does not exist", r.URL.Path)

	body := openapi.ProblemDetails{
		Type:      errors.ErrorTypeNotFound,
		Title:     "Endpoint Not Found",
		Status:    http.StatusNotFound,
		Detail:    &detail,
		Instance:  &r.URL.Path,
		Code:      ptrString(errors.CodeNotFoundEndpoint),
		Timestamp: &now,
		TraceId:   &traceID,
	}

	data, err := json.Marshal(body)
	if err != nil {
		logger.WithError(r.Context(), err).Error("Failed to marshal not found response")
		SendPanic(w, r)
		return
	}

	w.WriteHeader(http.StatusNotFound)
	_, err = w.Write(data)
	if err != nil {
		err = fmt.Errorf("can't send response body for request '%s'", r.URL.Path)
		logger.WithError(r.Context(), err).Error("Failed to send response")
	}
}

// SendPanic sends a panic error response in RFC 9457 Problem Details format.
// It attempts to include trace_id and timestamp dynamically, falling back to
// a pre-computed body if marshaling fails.
func SendPanic(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(http.StatusInternalServerError)

	// Try to generate a complete response with trace_id and timestamp
	traceID, _ := logger.GetRequestID(r.Context())
	now := time.Now().UTC()
	detail := "An unexpected error happened, please check the log of the service for details"
	instance := r.URL.Path

	panicError := openapi.ProblemDetails{
		Type:      errors.ErrorTypeInternal,
		Title:     "Internal Server Error",
		Status:    http.StatusInternalServerError,
		Detail:    &detail,
		Instance:  &instance,
		Code:      ptrString(errors.CodeInternalGeneral),
		Timestamp: &now,
		TraceId:   &traceID,
	}

	data, err := json.Marshal(panicError)
	if err != nil {
		// Fallback to pre-computed body without trace_id/timestamp
		data = panicBody
	}

	_, err = w.Write(data)
	if err != nil {
		err = fmt.Errorf(
			"can't send panic response for request '%s': %s",
			r.URL.Path,
			err.Error(),
		)
		logger.WithError(r.Context(), err).Error("Failed to send panic response")
	}
}

// panicBody is the error body that will be sent when something unexpected happens while trying to
// send another error response.
var panicBody []byte

func init() {
	ctx := context.Background()
	var err error

	detail := "An unexpected error happened, please check the log of the service for details"
	instance := "/api/hyperfleet/v1"

	panicError := openapi.ProblemDetails{
		Type:     errors.ErrorTypeInternal,
		Title:    "Internal Server Error",
		Status:   http.StatusInternalServerError,
		Detail:   &detail,
		Instance: &instance,
		Code:     ptrString(errors.CodeInternalGeneral),
	}

	panicBody, err = json.Marshal(panicError)
	if err != nil {
		err = fmt.Errorf("can't create the panic error body: %s", err.Error())
		logger.WithError(ctx, err).Error("Failed to create panic error body")
		os.Exit(1)
	}
}

func ptrString(s string) *string {
	return &s
}
