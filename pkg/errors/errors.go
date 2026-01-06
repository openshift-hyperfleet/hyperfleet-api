package errors

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
)

const (
	// Prefix used for error code strings
	// Example:
	//   ErrorCodePrefix = "rh-text"
	//   results in: hyperfleet-1
	ErrorCodePrefix = "hyperfleet"

	// HREF for API errors
	ErrorHref = "/api/hyperfleet/v1/errors/"

	// InvalidToken occurs when a token is invalid (generally, not found in the database)
	ErrorInvalidToken ServiceErrorCode = 1

	// Forbidden occurs when a user has been blacklisted
	ErrorForbidden ServiceErrorCode = 4

	// Conflict occurs when a database constraint is violated
	ErrorConflict ServiceErrorCode = 6

	// NotFound occurs when a record is not found in the database
	ErrorNotFound ServiceErrorCode = 7

	// Validation occurs when an object fails validation
	ErrorValidation ServiceErrorCode = 8

	// General occurs when an error fails to match any other error code
	ErrorGeneral ServiceErrorCode = 9

	// NotImplemented occurs when an API REST method is not implemented in a handler
	ErrorNotImplemented ServiceErrorCode = 10

	// Unauthorized occurs when the requester is not authorized to perform the specified action
	ErrorUnauthorized ServiceErrorCode = 11

	// Unauthenticated occurs when the provided credentials cannot be validated
	ErrorUnauthenticated ServiceErrorCode = 15

	// MalformedRequest occurs when the request body cannot be read
	ErrorMalformedRequest ServiceErrorCode = 17

	// Bad Request
	ErrorBadRequest ServiceErrorCode = 21

	// Invalid Search Query
	ErrorFailedToParseSearch ServiceErrorCode = 23

	// DatabaseAdvisoryLock occurs whe the advisory lock is failed to get
	ErrorDatabaseAdvisoryLock ServiceErrorCode = 26
)

type ServiceErrorCode int

type ServiceErrors []ServiceError

// ValidationDetail represents a single field validation error
type ValidationDetail struct {
	Field string `json:"field"`
	Error string `json:"error"`
}

func Find(code ServiceErrorCode) (bool, *ServiceError) {
	for _, err := range Errors() {
		if err.Code == code {
			return true, &err
		}
	}
	return false, nil
}

func Errors() ServiceErrors {
	return ServiceErrors{
		{Code: ErrorInvalidToken, Reason: "Invalid token provided", HttpCode: http.StatusForbidden},
		{Code: ErrorForbidden, Reason: "Forbidden to perform this action", HttpCode: http.StatusForbidden},
		{Code: ErrorConflict, Reason: "An entity with the specified unique values already exists", HttpCode: http.StatusConflict},
		{Code: ErrorNotFound, Reason: "Resource not found", HttpCode: http.StatusNotFound},
		{Code: ErrorValidation, Reason: "General validation failure", HttpCode: http.StatusBadRequest},
		{Code: ErrorGeneral, Reason: "Unspecified error", HttpCode: http.StatusInternalServerError},
		{Code: ErrorNotImplemented, Reason: "HTTP Method not implemented for this endpoint", HttpCode: http.StatusMethodNotAllowed},
		{Code: ErrorUnauthorized, Reason: "Account is unauthorized to perform this action", HttpCode: http.StatusForbidden},
		{Code: ErrorUnauthenticated, Reason: "Account authentication could not be verified", HttpCode: http.StatusUnauthorized},
		{Code: ErrorMalformedRequest, Reason: "Unable to read request body", HttpCode: http.StatusBadRequest},
		{Code: ErrorBadRequest, Reason: "Bad request", HttpCode: http.StatusBadRequest},
		{Code: ErrorFailedToParseSearch, Reason: "Failed to parse search query", HttpCode: http.StatusBadRequest},
		{Code: ErrorDatabaseAdvisoryLock, Reason: "Database advisory lock error", HttpCode: http.StatusInternalServerError},
	}
}

type ServiceError struct {
	// Code is the numeric and distinct ID for the error
	Code ServiceErrorCode
	// Reason is the context-specific reason the error was generated
	Reason string
	// HttopCode is the HttpCode associated with the error when the error is returned as an API response
	HttpCode int
	// Details contains field-level validation errors (optional)
	Details []ValidationDetail
}

// New Reason can be a string with format verbs, which will be replace by the specified values
func New(code ServiceErrorCode, reason string, values ...interface{}) *ServiceError {
	// If the code isn't defined, use the general error code
	var err *ServiceError
	exists, err := Find(code)
	if !exists {
		slog.Error("Undefined error code used", "code", code)
		err = &ServiceError{
			Code:     ErrorGeneral,
			Reason:   "Unspecified error",
			HttpCode: 500,
		}
	}

	// If the reason is unspecified, use the default
	if reason != "" {
		err.Reason = fmt.Sprintf(reason, values...)
	}

	return err
}

func (e *ServiceError) Error() string {
	return fmt.Sprintf("%s: %s", *CodeStr(e.Code), e.Reason)
}

func (e *ServiceError) AsError() error {
	return fmt.Errorf("%s", e.Error())
}

func (e *ServiceError) Is404() bool {
	return e.Code == NotFound("").Code
}

func (e *ServiceError) IsConflict() bool {
	return e.Code == Conflict("").Code
}

func (e *ServiceError) IsForbidden() bool {
	return e.Code == Forbidden("").Code
}

func (e *ServiceError) AsOpenapiError(operationID string) openapi.Error {
	openapiErr := openapi.Error{
		Kind:        openapi.PtrString("Error"),
		Id:          openapi.PtrString(strconv.Itoa(int(e.Code))),
		Href:        Href(e.Code),
		Code:        CodeStr(e.Code),
		Reason:      openapi.PtrString(e.Reason),
		OperationId: openapi.PtrString(operationID),
	}

	// Add validation details if present
	if len(e.Details) > 0 {
		details := make([]openapi.ErrorDetailsInner, len(e.Details))
		for i, detail := range e.Details {
			details[i] = openapi.ErrorDetailsInner{
				Field: openapi.PtrString(detail.Field),
				Error: openapi.PtrString(detail.Error),
			}
		}
		openapiErr.Details = details
	}

	return openapiErr
}

func CodeStr(code ServiceErrorCode) *string {
	return openapi.PtrString(fmt.Sprintf("%s-%d", ErrorCodePrefix, code))
}

func Href(code ServiceErrorCode) *string {
	return openapi.PtrString(fmt.Sprintf("%s%d", ErrorHref, code))
}

func NotFound(reason string, values ...interface{}) *ServiceError {
	return New(ErrorNotFound, reason, values...)
}

func GeneralError(reason string, values ...interface{}) *ServiceError {
	return New(ErrorGeneral, reason, values...)
}

func Unauthorized(reason string, values ...interface{}) *ServiceError {
	return New(ErrorUnauthorized, reason, values...)
}

func Unauthenticated(reason string, values ...interface{}) *ServiceError {
	return New(ErrorUnauthenticated, reason, values...)
}

func Forbidden(reason string, values ...interface{}) *ServiceError {
	return New(ErrorForbidden, reason, values...)
}

func NotImplemented(reason string, values ...interface{}) *ServiceError {
	return New(ErrorNotImplemented, reason, values...)
}

func Conflict(reason string, values ...interface{}) *ServiceError {
	return New(ErrorConflict, reason, values...)
}

func Validation(reason string, values ...interface{}) *ServiceError {
	return New(ErrorValidation, reason, values...)
}

// ValidationWithDetails creates a validation error with field-level details
func ValidationWithDetails(reason string, details []ValidationDetail) *ServiceError {
	err := New(ErrorValidation, "%s", reason)
	err.Details = details
	return err
}

func MalformedRequest(reason string, values ...interface{}) *ServiceError {
	return New(ErrorMalformedRequest, reason, values...)
}

func BadRequest(reason string, values ...interface{}) *ServiceError {
	return New(ErrorBadRequest, reason, values...)
}

func FailedToParseSearch(reason string, values ...interface{}) *ServiceError {
	message := fmt.Sprintf("Failed to parse search query: %s", reason)
	return New(ErrorFailedToParseSearch, message, values...)
}

func DatabaseAdvisoryLock(err error) *ServiceError {
	return New(ErrorDatabaseAdvisoryLock, err.Error(), []string{})
}
