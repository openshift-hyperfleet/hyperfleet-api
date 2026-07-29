package handlers

import (
	"encoding/json"
	goerrors "errors"
	"io"
	"net/http"
	"reflect"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/presenters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/response"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
)

func writeJSONResponse(w http.ResponseWriter, r *http.Request, code int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	// By default, decide whether or not a cache is usable based on the matching of the JWT
	// For example, this will keep caches from being used in the same browser if two users were to log in back to back
	w.Header().Set("Vary", "Authorization")

	w.WriteHeader(code)

	if payload != nil {
		response, err := json.Marshal(payload)
		if err != nil {
			// Headers already sent, can't change status code
			logger.With(r.Context(),
				logger.HTTPPath(r.URL.Path),
				logger.HTTPMethod(r.Method),
				logger.HTTPStatusCode(code),
			).WithError(err).Error("Failed to marshal JSON response payload")
			return
		}
		if _, err := w.Write(response); err != nil {
			// Writing failed, nothing we can do at this point
			logger.With(r.Context(),
				logger.HTTPPath(r.URL.Path),
				logger.HTTPMethod(r.Method),
				logger.HTTPStatusCode(code),
			).WithError(err).Error("Failed to write JSON response body")
			return
		}
	}
}

// decodeAndValidate unmarshals the request body and runs validation functions.
// Pass "strict" to reject unknown fields (use for PATCH to catch immutable field changes).
func decodeAndValidate(r *http.Request, req any, validateFuncs []validate, decodeType ...string) *errors.ServiceError {
	dec := json.NewDecoder(r.Body)
	// Add "strict" decodeType to reject unknown fields in request body
	if len(decodeType) > 0 {
		if decodeType[0] == "strict" {
			dec.DisallowUnknownFields()
		} else {
			return errors.GeneralError("invalid decode mode: %s (must be 'strict')", decodeType[0])
		}
	}

	// Default behavior is to silently ignore unknown fields
	if err := dec.Decode(req); err != nil {
		if err == io.EOF {
			return errors.MalformedRequest("Request body is required but was empty")
		}
		if typeErr := cleanTypeMismatchError(err); typeErr != nil {
			return typeErr
		}
		return errors.MalformedRequest("Invalid request format: %s", err)
	}

	for _, validateFunc := range validateFuncs {
		if svcErr := validateFunc(); svcErr != nil {
			return svcErr
		}
	}
	return nil
}

func handleError(r *http.Request, w http.ResponseWriter, err *errors.ServiceError) {
	traceID, _ := logger.GetRequestID(r.Context())
	instance := r.URL.Path

	// Log with RFC 9457 code format
	if err.HTTPCode >= 400 && err.HTTPCode <= 499 {
		logger.With(r.Context(),
			"code", err.RFC9457Code,
			"http_code", err.HTTPCode,
			"reason", err.Reason).Info("Client error response")
	} else {
		logger.With(r.Context(),
			"code", err.RFC9457Code,
			"http_code", err.HTTPCode,
			"reason", err.Reason).Error("Server error response")
	}

	response.WriteProblemDetailsResponse(w, r, err.HTTPCode, err.AsProblemDetails(instance, traceID))
}

// applyFieldFilter applies field filtering to a presented resource based on the ?fields query parameter.
// If no fields are specified, it returns the original presented resource.
// If fields are specified, it filters the resource and returns only the requested fields.
// Note: This function only validates the fields parameter, not pagination parameters,
// to avoid rejecting irrelevant query params on single-resource GET endpoints.
func applyFieldFilter(r *http.Request, presented interface{}) (interface{}, *errors.ServiceError) {
	fields := ensureIDField(normalizeList(r.URL.Query()["fields"]))
	if fields != nil {
		filtered, filterErr := presenters.FilterSingle(fields, presented)
		if filterErr != nil {
			return nil, filterErr
		}
		return filtered, nil
	}
	return presented, nil
}

func cleanTypeMismatchError(err error) *errors.ServiceError {
	var typeErr *json.UnmarshalTypeError
	if !goerrors.As(err, &typeErr) {
		return nil
	}
	return errors.Validation("field '%s' must be %s", typeErr.Field, describeJSONKind(typeErr.Type.Kind()))
}

func describeJSONKind(k reflect.Kind) string {
	switch k {
	case reflect.Map, reflect.Struct:
		return "an object"
	case reflect.Slice, reflect.Array:
		return "an array"
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "a boolean"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return "a number"
	default:
		// Fallback for kinds json.UnmarshalTypeError.Type should never report
		// here (e.g. Ptr, Interface) — avoids mislabeling as a number.
		return "the correct type"
	}
}

func convertResourcePatch(req *openapi.ResourcePatchRequest) *api.ResourcePatch {
	patch := &api.ResourcePatch{}
	if req.Spec != nil {
		patch.Spec = *req.Spec
	}
	if req.Labels != nil {
		patch.Labels = *req.Labels
	}
	if req.References != nil {
		patch.References = *req.References
	}
	return patch
}

// extractReferences unwraps the optional references pointer from an API request.
// Returns nil when no references are supplied (nil pointer), or the map value.
func extractReferences(refs *api.ReferenceMap) api.ReferenceMap {
	if refs == nil {
		return nil
	}
	return *refs
}

// childCreateRejection returns a validation error indicating the resource must be
// created via its parent's nested route.
func childCreateRejection(descriptor registry.EntityDescriptor) *errors.ServiceError {
	parent := registry.MustGet(descriptor.ParentKind)
	svcErr := errors.Validation(
		"kind %q is a child kind; create it via /%s/{id}/%s",
		descriptor.Kind, parent.Plural, descriptor.Plural,
	)
	svcErr.HTTPCode = http.StatusUnprocessableEntity
	return svcErr
}
