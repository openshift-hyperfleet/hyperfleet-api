package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validators"
)

// handleValidationError writes validation error response in RFC 9457 Problem Details format
func handleValidationError(w http.ResponseWriter, r *http.Request, err *errors.ServiceError) {
	traceID, ok := logger.GetRequestID(r.Context())
	if !ok {
		traceID = "unknown"
	}

	// Log validation errors as warn (client error, not server error)
	logger.With(r.Context(),
		"trace_id", traceID,
	).WithError(err).Warn("Validation error")

	// Write RFC 9457 Problem Details error response
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(err.HttpCode)
	if encodeErr := json.NewEncoder(w).Encode(err.AsProblemDetails(r.URL.Path, traceID)); encodeErr != nil {
		logger.With(r.Context(),
			logger.HTTPPath(r.URL.Path),
			logger.HTTPMethod(r.Method),
			logger.HTTPStatusCode(err.HttpCode),
		).WithError(encodeErr).Error("Failed to encode validation error response")
	}
}

// SchemaValidationMiddleware validates cluster and nodepool spec fields against OpenAPI schemas
func SchemaValidationMiddleware(validator *validators.SchemaValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if the request requires spec validation
			shouldValidate, resourceType := shouldValidateRequest(r.Method, r.URL.Path)
			if !shouldValidate {
				next.ServeHTTP(w, r)
				return
			}

			// Read and buffer the request body
			bodyBytes, err := io.ReadAll(r.Body)
			if err != nil {
				serviceErr := errors.MalformedRequest("Failed to read request body")
				handleValidationError(w, r, serviceErr)
				return
			}
			if closeErr := r.Body.Close(); closeErr != nil {
				logger.WithError(r.Context(), closeErr).Warn("Failed to close request body")
			}

			// Restore the request body for the next handler
			r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

			// Parse JSON to extract spec field
			var requestData map[string]interface{}
			if err := json.Unmarshal(bodyBytes, &requestData); err != nil {
				serviceErr := errors.MalformedRequest("Invalid JSON in request body")
				handleValidationError(w, r, serviceErr)
				return
			}

			// Extract spec field
			spec, ok := requestData["spec"]
			if !ok {
				// If no spec field, skip validation (may be a request without spec)
				next.ServeHTTP(w, r)
				return
			}

			// Convert spec to map[string]interface{}
			specMap, ok := spec.(map[string]interface{})
			if !ok {
				serviceErr := errors.Validation("spec field must be an object")
				handleValidationError(w, r, serviceErr)
				return
			}

			// Validate spec using the resource type
			// validator.Validate returns nil for unknown resource types
			validationErr := validator.Validate(resourceType, specMap)

			// If validation failed, return 400 error
			if validationErr != nil {
				// Check if it's a ServiceError with details
				if serviceErr, ok := validationErr.(*errors.ServiceError); ok {
					handleValidationError(w, r, serviceErr)
					return
				}
				// Fallback to generic validation error
				serviceErr := errors.Validation("Spec validation failed: %v", validationErr)
				handleValidationError(w, r, serviceErr)
				return
			}

			// Validation passed, continue to next handler
			next.ServeHTTP(w, r)
		})
	}
}

// shouldValidateRequest determines if the request requires spec validation
// Returns (shouldValidate bool, resourceType string)
func shouldValidateRequest(method, path string) (bool, string) {
	// Only validate POST and PATCH requests
	if method != http.MethodPost && method != http.MethodPatch {
		return false, ""
	}

	// Check nodepools first (more specific path)
	// POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
	// PATCH /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
	if strings.Contains(path, "/nodepools") {
		return true, "nodepool"
	}

	// POST /api/hyperfleet/v1/clusters
	// PATCH /api/hyperfleet/v1/clusters/{cluster_id}
	if strings.HasSuffix(path, "/clusters") || strings.Contains(path, "/clusters/") {
		return true, "cluster"
	}

	return false, ""
}
