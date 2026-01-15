package errors

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// Error type URIs for RFC 9457
const (
	ErrorTypeBase        = "https://api.hyperfleet.io/errors/"
	ErrorTypeValidation  = ErrorTypeBase + "validation-error"
	ErrorTypeAuth        = ErrorTypeBase + "authentication-error"
	ErrorTypeAuthz       = ErrorTypeBase + "authorization-error"
	ErrorTypeNotFound    = ErrorTypeBase + "not-found"
	ErrorTypeConflict    = ErrorTypeBase + "conflict"
	ErrorTypeRateLimit   = ErrorTypeBase + "rate-limit"
	ErrorTypeInternal    = ErrorTypeBase + "internal-error"
	ErrorTypeService     = ErrorTypeBase + "service-unavailable"
	ErrorTypeBadRequest  = ErrorTypeBase + "bad-request"
	ErrorTypeMalformed   = ErrorTypeBase + "malformed-request"
	ErrorTypeNotImpl     = ErrorTypeBase + "not-implemented"
)

// Error codes in HYPERFLEET-CAT-NUM format
const (
	// Validation errors (VAL) - 400/422
	CodeValidationMultiple = "HYPERFLEET-VAL-000"
	CodeValidationRequired = "HYPERFLEET-VAL-001"
	CodeValidationInvalid  = "HYPERFLEET-VAL-002"
	CodeValidationFormat   = "HYPERFLEET-VAL-003"
	CodeValidationRange    = "HYPERFLEET-VAL-004"

	// Authentication errors (AUT) - 401
	CodeAuthNoCredentials      = "HYPERFLEET-AUT-001"
	CodeAuthInvalidCredentials = "HYPERFLEET-AUT-002"
	CodeAuthExpiredToken       = "HYPERFLEET-AUT-003"

	// Authorization errors (AUZ) - 403
	CodeAuthzInsufficient = "HYPERFLEET-AUZ-001"
	CodeAuthzForbidden    = "HYPERFLEET-AUZ-002"

	// Not Found errors (NTF) - 404
	CodeNotFoundGeneric  = "HYPERFLEET-NTF-001"
	CodeNotFoundCluster  = "HYPERFLEET-NTF-002"
	CodeNotFoundNodePool = "HYPERFLEET-NTF-003"

	// Conflict errors (CNF) - 409
	CodeConflictExists  = "HYPERFLEET-CNF-001"
	CodeConflictVersion = "HYPERFLEET-CNF-002"

	// Rate Limit errors (LMT) - 429
	CodeRateLimitExceeded = "HYPERFLEET-LMT-001"

	// Internal errors (INT) - 500
	CodeInternalGeneral  = "HYPERFLEET-INT-001"
	CodeInternalDatabase = "HYPERFLEET-INT-002"

	// Service errors (SVC) - 502/503/504
	CodeServiceUnavailable = "HYPERFLEET-SVC-001"
	CodeServiceTimeout     = "HYPERFLEET-SVC-002"

	// Bad Request errors
	CodeBadRequest      = "HYPERFLEET-VAL-005"
	CodeMalformedBody   = "HYPERFLEET-VAL-006"
	CodeSearchParseFail = "HYPERFLEET-VAL-007"
	CodeNotImplemented  = "HYPERFLEET-INT-003"
)

type ServiceErrors []ServiceError

// ValidationDetail represents a single field validation error (RFC 9457 format)
type ValidationDetail struct {
	Field      string      `json:"field"`
	Value      interface{} `json:"value,omitempty"`
	Constraint string      `json:"constraint,omitempty"`
	Message    string      `json:"message"`
}

// ServiceError represents an API error with RFC 9457 Problem Details support
type ServiceError struct {
	// RFC9457Code is the HYPERFLEET-CAT-NUM format code
	RFC9457Code string
	// Type is the RFC 9457 type URI
	Type string
	// Title is a short human-readable summary
	Title string
	// Reason is the context-specific reason (maps to detail in RFC 9457)
	Reason string
	// HttpCode is the HTTP status code
	HttpCode int
	// Details contains field-level validation errors
	Details []ValidationDetail
}

// errorDefinition holds the default values for each error type
type errorDefinition struct {
	Type     string
	Title    string
	Reason   string
	HttpCode int
}

var errorDefinitions = map[string]errorDefinition{
	CodeAuthExpiredToken:      {ErrorTypeAuth, "Invalid Token", "Invalid token provided", http.StatusUnauthorized},
	CodeAuthzForbidden:        {ErrorTypeAuthz, "Forbidden", "Forbidden to perform this action", http.StatusForbidden},
	CodeConflictExists:        {ErrorTypeConflict, "Resource Conflict", "An entity with the specified unique values already exists", http.StatusConflict},
	CodeNotFoundGeneric:       {ErrorTypeNotFound, "Resource Not Found", "Resource not found", http.StatusNotFound},
	CodeValidationMultiple:    {ErrorTypeValidation, "Validation Failed", "General validation failure", http.StatusBadRequest},
	CodeInternalGeneral:       {ErrorTypeInternal, "Internal Server Error", "Unspecified error", http.StatusInternalServerError},
	CodeNotImplemented:        {ErrorTypeNotImpl, "Not Implemented", "Functionality not implemented", http.StatusNotImplemented},
	CodeAuthzInsufficient:     {ErrorTypeAuthz, "Unauthorized", "Account is unauthorized to perform this action", http.StatusForbidden},
	CodeAuthNoCredentials:     {ErrorTypeAuth, "Authentication Required", "Account authentication could not be verified", http.StatusUnauthorized},
	CodeMalformedBody:         {ErrorTypeMalformed, "Malformed Request", "Unable to read request body", http.StatusBadRequest},
	CodeBadRequest:            {ErrorTypeBadRequest, "Bad Request", "Bad request", http.StatusBadRequest},
	CodeSearchParseFail:       {ErrorTypeValidation, "Invalid Search Query", "Failed to parse search query", http.StatusBadRequest},
	CodeInternalDatabase:      {ErrorTypeInternal, "Database Error", "Database advisory lock error", http.StatusInternalServerError},
	CodeAuthInvalidCredentials: {ErrorTypeAuth, "Invalid Credentials", "The provided credentials are invalid", http.StatusUnauthorized},
}

// Find looks up an error definition by its RFC 9457 code
func Find(code string) (bool, *ServiceError) {
	def, exists := errorDefinitions[code]
	if !exists {
		return false, nil
	}
	return true, &ServiceError{
		RFC9457Code: code,
		Type:        def.Type,
		Title:       def.Title,
		Reason:      def.Reason,
		HttpCode:    def.HttpCode,
	}
}

// Errors returns all defined errors
func Errors() ServiceErrors {
	errors := make(ServiceErrors, 0, len(errorDefinitions))
	for code, def := range errorDefinitions {
		errors = append(errors, ServiceError{
			RFC9457Code: code,
			Type:        def.Type,
			Title:       def.Title,
			Reason:      def.Reason,
			HttpCode:    def.HttpCode,
		})
	}
	return errors
}

// New creates a new ServiceError with optional custom reason
func New(code string, reason string, values ...interface{}) *ServiceError {
	exists, err := Find(code)
	if !exists {
		ctx := context.Background()
		logger.With(ctx, logger.FieldErrorCode, code).Error("Undefined error code used")
		err = &ServiceError{
			RFC9457Code: CodeInternalGeneral,
			Type:        ErrorTypeInternal,
			Title:       "Internal Server Error",
			Reason:      "Unspecified error",
			HttpCode:    http.StatusInternalServerError,
		}
	}

	if reason != "" {
		err.Reason = fmt.Sprintf(reason, values...)
	}

	return err
}

func (e *ServiceError) Error() string {
	return fmt.Sprintf("%s: %s", e.RFC9457Code, e.Reason)
}

func (e *ServiceError) AsError() error {
	return fmt.Errorf("%s", e.Error())
}

func (e *ServiceError) Is404() bool {
	return e.Type == ErrorTypeNotFound
}

func (e *ServiceError) IsConflict() bool {
	return e.Type == ErrorTypeConflict
}

func (e *ServiceError) IsForbidden() bool {
	return e.Type == ErrorTypeAuthz
}

// validConstraints maps string values to their corresponding ValidationErrorConstraint enum values
var validConstraints = map[string]openapi.ValidationErrorConstraint{
	"required":   openapi.Required,
	"min":        openapi.Min,
	"max":        openapi.Max,
	"min_length": openapi.MinLength,
	"max_length": openapi.MaxLength,
	"pattern":    openapi.Pattern,
	"enum":       openapi.Enum,
	"format":     openapi.Format,
	"unique":     openapi.Unique,
}

// isValidConstraint checks if the given constraint string is a valid ValidationErrorConstraint
// and returns the corresponding enum value if valid
func isValidConstraint(constraint string) (openapi.ValidationErrorConstraint, bool) {
	if c, ok := validConstraints[constraint]; ok {
		return c, true
	}
	return "", false
}

// AsProblemDetails converts the ServiceError to RFC 9457 Problem Details format
func (e *ServiceError) AsProblemDetails(instance string, traceID string) openapi.Error {
	now := time.Now().UTC()
	problemDetails := openapi.Error{
		Type:      e.Type,
		Title:     e.Title,
		Status:    e.HttpCode,
		Detail:    &e.Reason,
		Instance:  &instance,
		Code:      &e.RFC9457Code,
		Timestamp: &now,
		TraceId:   &traceID,
	}

	// Add validation errors if present
	if len(e.Details) > 0 {
		validationErrors := make([]openapi.ValidationError, len(e.Details))
		for i, detail := range e.Details {
			validationErrors[i] = openapi.ValidationError{
				Field:   detail.Field,
				Message: detail.Message,
				Value:   detail.Value,
			}
			if detail.Constraint != "" {
				if constraint, ok := isValidConstraint(detail.Constraint); ok {
					validationErrors[i].Constraint = &constraint
				}
			}
		}
		problemDetails.Errors = &validationErrors
	}

	return problemDetails
}

// Constructor functions

func NotFound(reason string, values ...interface{}) *ServiceError {
	return New(CodeNotFoundGeneric, reason, values...)
}

func GeneralError(reason string, values ...interface{}) *ServiceError {
	return New(CodeInternalGeneral, reason, values...)
}

func Unauthorized(reason string, values ...interface{}) *ServiceError {
	return New(CodeAuthzInsufficient, reason, values...)
}

func Unauthenticated(reason string, values ...interface{}) *ServiceError {
	return New(CodeAuthNoCredentials, reason, values...)
}

func Forbidden(reason string, values ...interface{}) *ServiceError {
	return New(CodeAuthzForbidden, reason, values...)
}

func NotImplemented(reason string, values ...interface{}) *ServiceError {
	return New(CodeNotImplemented, reason, values...)
}

func Conflict(reason string, values ...interface{}) *ServiceError {
	return New(CodeConflictExists, reason, values...)
}

func Validation(reason string, values ...interface{}) *ServiceError {
	return New(CodeValidationMultiple, reason, values...)
}

// ValidationWithDetails creates a validation error with field-level details
func ValidationWithDetails(reason string, details []ValidationDetail) *ServiceError {
	err := New(CodeValidationMultiple, "%s", reason)
	err.Details = details
	return err
}

func MalformedRequest(reason string, values ...interface{}) *ServiceError {
	return New(CodeMalformedBody, reason, values...)
}

func BadRequest(reason string, values ...interface{}) *ServiceError {
	return New(CodeBadRequest, reason, values...)
}

func FailedToParseSearch(reason string, values ...interface{}) *ServiceError {
	message := fmt.Sprintf("Failed to parse search query: %s", reason)
	return New(CodeSearchParseFail, message, values...)
}

func DatabaseAdvisoryLock(err error) *ServiceError {
	return New(CodeInternalDatabase, "%s", err.Error())
}

func InvalidToken(reason string, values ...interface{}) *ServiceError {
	return New(CodeAuthExpiredToken, reason, values...)
}
