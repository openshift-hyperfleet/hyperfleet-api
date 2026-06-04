package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
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
	w.WriteHeader(err.HTTPCode)
	if encodeErr := json.NewEncoder(w).Encode(err.AsProblemDetails(r.URL.Path, traceID)); encodeErr != nil {
		logger.With(r.Context(),
			logger.HTTPPath(r.URL.Path),
			logger.HTTPMethod(r.Method),
			logger.HTTPStatusCode(err.HTTPCode),
		).WithError(encodeErr).Error("Failed to encode validation error response")
	}
}

// specEntityMatcher pairs a resource plural with a pre-compiled regex for path matching.
type specEntityMatcher struct {
	re     *regexp.Regexp
	plural string
}

// buildEndsWithEntityRegex compiles a regex that matches a URL path ending in:
//   - /<plural>
//   - /<plural>/
//   - /<plural>/<uuid>
func buildEndsWithEntityRegex(plural string) *regexp.Regexp {
	pattern := fmt.Sprintf(
		`/%s(?:/?|/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})$`,
		regexp.QuoteMeta(plural),
	)
	return regexp.MustCompile(pattern)
}

func buildMatchers(specEntities []registry.EntityDescriptor) []specEntityMatcher {
	matchers := make([]specEntityMatcher, len(specEntities))
	for i, d := range specEntities {
		matchers[i] = specEntityMatcher{
			plural: d.Plural,
			re:     buildEndsWithEntityRegex(d.Plural),
		}
	}
	return matchers
}

// SchemaValidationMiddleware validates resource spec fields against OpenAPI schemas for every
// registered entity that declares SpecSchemaName.
func SchemaValidationMiddleware(validator *validators.SchemaValidator) func(http.Handler) http.Handler {

	// TODO : HYPERFLEET-1159 - Remove this once Cluster and NodePool are registered
	specEntities := []registry.EntityDescriptor{
		{
			Kind:           "Cluster",
			Plural:         "clusters",
			SpecSchemaName: "ClusterSpec",
		},
		{
			Kind:           "NodePool",
			Plural:         "nodepools",
			ParentKind:     "Cluster",
			SpecSchemaName: "NodePoolSpec",
		},
	}

	specEntities = append(specEntities, registry.WithSpecSchema()...)
	matchers := buildMatchers(specEntities)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			shouldValidate, resourcePlural := shouldValidateRequest(r.Method, r.URL.Path, matchers)
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
			specMap, ok := spec.(map[string]any)
			if !ok {
				serviceErr := errors.Validation("spec field must be an object")
				handleValidationError(w, r, serviceErr)
				return
			}

			validationErr := validator.Validate(resourcePlural, specMap)

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

// shouldValidateRequest determines if the request requires spec validation.
//
// Each matcher's regex anchors at the end of the path, so when a path contains
// multiple registered plurals (e.g. /clusters/{id}/nodepools), only the
// rightmost (deepest) segment can match, and that match wins automatically.
func shouldValidateRequest(
	method, path string, matchers []specEntityMatcher,
) (shouldValidate bool, resourcePlural string) {
	if method != http.MethodPost && method != http.MethodPatch {
		return false, ""
	}

	shouldValidate = false
	for _, m := range matchers {
		shouldValidate = m.re.MatchString(path)
		if shouldValidate {
			resourcePlural = m.plural
			break
		}
	}

	return shouldValidate, resourcePlural
}
