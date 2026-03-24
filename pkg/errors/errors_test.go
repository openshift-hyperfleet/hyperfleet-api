package errors

import (
	stderrors "errors"
	"net/http"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
)

func TestConstructors(t *testing.T) {
	tests := []struct {
		name           string
		build          func() *ServiceError
		expectedCode   string
		expectedType   string
		expectedReason string
		expectedHTTP   int
	}{
		{
			name:           "NotFound",
			build:          func() *ServiceError { return NotFound("cluster %q not found", "my-cluster") },
			expectedCode:   CodeNotFoundGeneric,
			expectedHTTP:   http.StatusNotFound,
			expectedType:   ErrorTypeNotFound,
			expectedReason: `cluster "my-cluster" not found`,
		},
		{
			name:           "GeneralError",
			build:          func() *ServiceError { return GeneralError("error code %d", 42) },
			expectedCode:   CodeInternalGeneral,
			expectedHTTP:   http.StatusInternalServerError,
			expectedType:   ErrorTypeInternal,
			expectedReason: "error code 42",
		},
		{
			name:           "Unauthorized",
			build:          func() *ServiceError { return Unauthorized("not allowed") },
			expectedCode:   CodeAuthzInsufficient,
			expectedHTTP:   http.StatusForbidden,
			expectedType:   ErrorTypeAuthz,
			expectedReason: "not allowed",
		},
		{
			name:           "Unauthenticated",
			build:          func() *ServiceError { return Unauthenticated("no credentials") },
			expectedCode:   CodeAuthNoCredentials,
			expectedHTTP:   http.StatusUnauthorized,
			expectedType:   ErrorTypeAuth,
			expectedReason: "no credentials",
		},
		{
			name:           "Forbidden",
			build:          func() *ServiceError { return Forbidden("access denied") },
			expectedCode:   CodeAuthzForbidden,
			expectedHTTP:   http.StatusForbidden,
			expectedType:   ErrorTypeAuthz,
			expectedReason: "access denied",
		},
		{
			name:           "NotImplemented",
			build:          func() *ServiceError { return NotImplemented("coming soon") },
			expectedCode:   CodeNotImplemented,
			expectedHTTP:   http.StatusNotImplemented,
			expectedType:   ErrorTypeNotImpl,
			expectedReason: "coming soon",
		},
		{
			name:           "Conflict",
			build:          func() *ServiceError { return Conflict("already exists") },
			expectedCode:   CodeConflictExists,
			expectedHTTP:   http.StatusConflict,
			expectedType:   ErrorTypeConflict,
			expectedReason: "already exists",
		},
		{
			name:           "Validation",
			build:          func() *ServiceError { return Validation("field %s must be %s", "size", "positive") },
			expectedCode:   CodeValidationMultiple,
			expectedHTTP:   http.StatusBadRequest,
			expectedType:   ErrorTypeValidation,
			expectedReason: "field size must be positive",
		},
		{
			name: "ValidationWithDetails",
			build: func() *ServiceError {
				return ValidationWithDetails("validation failed", []ValidationDetail{
					{Field: "name", Message: "required"},
				})
			},
			expectedCode:   CodeValidationMultiple,
			expectedHTTP:   http.StatusBadRequest,
			expectedType:   ErrorTypeValidation,
			expectedReason: "validation failed",
		},
		{
			name:           "MalformedRequest",
			build:          func() *ServiceError { return MalformedRequest("cannot parse body") },
			expectedCode:   CodeMalformedBody,
			expectedHTTP:   http.StatusBadRequest,
			expectedType:   ErrorTypeMalformed,
			expectedReason: "cannot parse body",
		},
		{
			name:           "BadRequest",
			build:          func() *ServiceError { return BadRequest("bad input") },
			expectedCode:   CodeBadRequest,
			expectedHTTP:   http.StatusBadRequest,
			expectedType:   ErrorTypeBadRequest,
			expectedReason: "bad input",
		},
		{
			name:           "FailedToParseSearch wraps reason",
			build:          func() *ServiceError { return FailedToParseSearch("unexpected token") },
			expectedCode:   CodeSearchParseFail,
			expectedHTTP:   http.StatusBadRequest,
			expectedType:   ErrorTypeValidation,
			expectedReason: "Failed to parse search query: unexpected token",
		},
		{
			name:           "DatabaseAdvisoryLock",
			build:          func() *ServiceError { return DatabaseAdvisoryLock(stderrors.New("lock timeout")) },
			expectedCode:   CodeInternalDatabase,
			expectedHTTP:   http.StatusInternalServerError,
			expectedType:   ErrorTypeInternal,
			expectedReason: "internal database error",
		},
		{
			name:           "InvalidToken",
			build:          func() *ServiceError { return InvalidToken("token expired") },
			expectedCode:   CodeAuthExpiredToken,
			expectedHTTP:   http.StatusUnauthorized,
			expectedType:   ErrorTypeAuth,
			expectedReason: "token expired",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tt.build()
			Expect(err).NotTo(BeNil())
			Expect(err.RFC9457Code).To(Equal(tt.expectedCode))
			Expect(err.HTTPCode).To(Equal(tt.expectedHTTP))
			Expect(err.Type).To(Equal(tt.expectedType))
			Expect(err.Reason).To(Equal(tt.expectedReason))
		})
	}
}

func TestServiceError_HelperMethods(t *testing.T) {
	tests := []struct {
		name          string
		build         func() *ServiceError
		expectedError string
		is404         bool
		isConflict    bool
		isForbidden   bool
	}{
		{
			name:          "NotFound",
			build:         func() *ServiceError { return NotFound("cluster not found") },
			expectedError: CodeNotFoundGeneric + ": cluster not found",
			is404:         true,
		},
		{
			name:          "Conflict",
			build:         func() *ServiceError { return Conflict("already exists") },
			expectedError: CodeConflictExists + ": already exists",
			isConflict:    true,
		},
		{
			name:          "Unauthorized",
			build:         func() *ServiceError { return Unauthorized("not allowed") },
			expectedError: CodeAuthzInsufficient + ": not allowed",
			isForbidden:   true,
		},
		{
			name:          "Forbidden",
			build:         func() *ServiceError { return Forbidden("access denied") },
			expectedError: CodeAuthzForbidden + ": access denied",
			isForbidden:   true,
		},
		{
			name:          "Unauthenticated",
			build:         func() *ServiceError { return Unauthenticated("no credentials") },
			expectedError: CodeAuthNoCredentials + ": no credentials",
		},
		{
			name:          "GeneralError",
			build:         func() *ServiceError { return GeneralError("something failed") },
			expectedError: CodeInternalGeneral + ": something failed",
		},
		{
			name:          "Validation",
			build:         func() *ServiceError { return Validation("bad input") },
			expectedError: CodeValidationMultiple + ": bad input",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tt.build()
			Expect(err.Error()).To(Equal(tt.expectedError))
			Expect(err.AsError().Error()).To(Equal(tt.expectedError))
			Expect(err.Is404()).To(Equal(tt.is404))
			Expect(err.IsConflict()).To(Equal(tt.isConflict))
			Expect(err.IsForbidden()).To(Equal(tt.isForbidden))
		})
	}
}

func assertValidationErrors(
	t *testing.T,
	got *[]openapi.ValidationError,
	want *[]ValidationDetail,
	constraints map[int]openapi.ValidationErrorConstraint,
) {
	t.Helper()
	if want == nil {
		Expect(got).To(BeNil())
		return
	}
	Expect(got).NotTo(BeNil())
	Expect(*got).To(HaveLen(len(*want)))
	for i, detail := range *want {
		Expect((*got)[i].Field).To(Equal(detail.Field))
		Expect((*got)[i].Message).To(Equal(detail.Message))
		if detail.Value != nil {
			Expect((*got)[i].Value).To(Equal(detail.Value))
		} else {
			Expect((*got)[i].Value).To(BeNil())
		}
		if c, ok := constraints[i]; ok {
			Expect((*got)[i].Constraint).NotTo(BeNil())
			Expect(*(*got)[i].Constraint).To(Equal(c))
		} else {
			Expect((*got)[i].Constraint).To(BeNil())
		}
	}
}

func TestAsProblemDetails(t *testing.T) {
	tests := []struct {
		name             string
		build            func() *ServiceError
		expectedErrors   *[]ValidationDetail
		checkConstraints map[int]openapi.ValidationErrorConstraint
		instance         string
		traceID          string
		expectedType     string
		expectedTitle    string
		expectedDetail   string
		expectedCode     string
		expectedStatus   int
	}{
		{
			name:           "basic fields populated",
			build:          func() *ServiceError { return NotFound("cluster abc not found") },
			instance:       "/api/v1/clusters/abc",
			traceID:        "trace-123",
			expectedType:   ErrorTypeNotFound,
			expectedTitle:  "Resource Not Found",
			expectedStatus: http.StatusNotFound,
			expectedDetail: "cluster abc not found",
			expectedCode:   CodeNotFoundGeneric,
		},
		{
			name:           "empty instance and traceID are nil",
			build:          func() *ServiceError { return GeneralError("internal error") },
			expectedType:   ErrorTypeInternal,
			expectedTitle:  "Internal Server Error",
			expectedStatus: http.StatusInternalServerError,
			expectedDetail: "internal error",
			expectedCode:   CodeInternalGeneral,
		},
		{
			name:           "validation without details has nil errors field",
			build:          func() *ServiceError { return Validation("bad input") },
			expectedType:   ErrorTypeValidation,
			expectedTitle:  "Validation Failed",
			expectedStatus: http.StatusBadRequest,
			expectedDetail: "bad input",
			expectedCode:   CodeValidationMultiple,
		},
		{
			name: "validation details with known constraints",
			build: func() *ServiceError {
				return ValidationWithDetails("validation failed", []ValidationDetail{
					{Field: "name", Message: "required", Constraint: "required"},
					{Field: "size", Value: -1, Message: "must be positive", Constraint: "min"},
					{Field: "label", Message: "invalid format", Constraint: "pattern"},
				})
			},
			expectedType:   ErrorTypeValidation,
			expectedTitle:  "Validation Failed",
			expectedStatus: http.StatusBadRequest,
			expectedDetail: "validation failed",
			expectedCode:   CodeValidationMultiple,
			expectedErrors: &[]ValidationDetail{
				{Field: "name", Message: "required"},
				{Field: "size", Value: -1, Message: "must be positive"},
				{Field: "label", Message: "invalid format"},
			},
			checkConstraints: map[int]openapi.ValidationErrorConstraint{
				0: openapi.Required,
				1: openapi.Min,
				2: openapi.Pattern,
			},
		},
		{
			name: "unknown constraint is omitted",
			build: func() *ServiceError {
				return ValidationWithDetails("validation failed", []ValidationDetail{
					{Field: "foo", Message: "bad value", Constraint: "not_a_real_constraint"},
				})
			},
			expectedType:   ErrorTypeValidation,
			expectedTitle:  "Validation Failed",
			expectedStatus: http.StatusBadRequest,
			expectedDetail: "validation failed",
			expectedCode:   CodeValidationMultiple,
			expectedErrors: &[]ValidationDetail{
				{Field: "foo", Message: "bad value"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tt.build()
			pd := err.AsProblemDetails(tt.instance, tt.traceID)

			Expect(pd.Type).To(Equal(tt.expectedType))
			Expect(pd.Title).To(Equal(tt.expectedTitle))
			Expect(pd.Status).To(Equal(tt.expectedStatus))
			Expect(pd.Detail).NotTo(BeNil())
			Expect(*pd.Detail).To(Equal(tt.expectedDetail))
			Expect(pd.Code).NotTo(BeNil())
			Expect(*pd.Code).To(Equal(tt.expectedCode))
			Expect(pd.Timestamp).NotTo(BeNil())

			if tt.instance != "" {
				Expect(pd.Instance).NotTo(BeNil())
				Expect(*pd.Instance).To(Equal(tt.instance))
			} else {
				Expect(pd.Instance).To(BeNil())
			}
			if tt.traceID != "" {
				Expect(pd.TraceId).NotTo(BeNil())
				Expect(*pd.TraceId).To(Equal(tt.traceID))
			} else {
				Expect(pd.TraceId).To(BeNil())
			}

			assertValidationErrors(t, pd.Errors, tt.expectedErrors, tt.checkConstraints)
		})
	}
}

func TestErrorFind(t *testing.T) {
	tests := []struct {
		name        string
		code        string
		expectType  string
		expectFound bool
	}{
		{"known code", CodeNotFoundGeneric, ErrorTypeNotFound, true},
		{"unknown code", "INVALID-CODE-999", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			found, err := Find(tt.code)
			Expect(found).To(Equal(tt.expectFound))
			if tt.expectFound {
				Expect(err).NotTo(BeNil())
				Expect(err.RFC9457Code).To(Equal(tt.code))
				Expect(err.Type).To(Equal(tt.expectType))
			} else {
				Expect(err).To(BeNil())
			}
		})
	}
}

func TestErrorFormatting(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		reason         string
		expectedCode   string
		expectedReason string
		values         []any
		expectedHTTP   int
	}{
		{
			name:           "unknown code falls back to internal",
			code:           "HYPERFLEET-UNKNOWN-999",
			reason:         "custom reason",
			expectedCode:   CodeInternalGeneral,
			expectedHTTP:   http.StatusInternalServerError,
			expectedReason: "custom reason",
		},
		{
			name:           "empty reason keeps default",
			code:           CodeNotFoundGeneric,
			reason:         "",
			expectedCode:   CodeNotFoundGeneric,
			expectedHTTP:   http.StatusNotFound,
			expectedReason: "Resource not found",
		},
		{
			name:           "plain reason without format args",
			code:           CodeInternalGeneral,
			reason:         "plain reason",
			expectedCode:   CodeInternalGeneral,
			expectedHTTP:   http.StatusInternalServerError,
			expectedReason: "plain reason",
		},
		{
			name:           "reason with format args",
			code:           CodeInternalGeneral,
			reason:         "test %s, %d",
			values:         []any{"errors", 1},
			expectedCode:   CodeInternalGeneral,
			expectedHTTP:   http.StatusInternalServerError,
			expectedReason: "test errors, 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := New(tt.code, tt.reason, tt.values...)
			Expect(err).NotTo(BeNil())
			Expect(err.RFC9457Code).To(Equal(tt.expectedCode))
			Expect(err.HTTPCode).To(Equal(tt.expectedHTTP))
			Expect(err.Reason).To(Equal(tt.expectedReason))
		})
	}
}

func TestErrorCodes_FormatConvention(t *testing.T) {
	for code := range errorDefinitions {
		t.Run(code, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(strings.HasPrefix(code, "HYPERFLEET-")).To(BeTrue())
			parts := strings.Split(code, "-")
			Expect(parts).To(HaveLen(3))
			Expect(parts[1]).To(MatchRegexp(`^[A-Z]{3}$`))
			Expect(parts[2]).To(MatchRegexp(`^\d{3}$`))
		})
	}
}

func TestErrorTypeURIs(t *testing.T) {
	types := map[string]struct{}{}
	for _, def := range errorDefinitions {
		types[def.Type] = struct{}{}
	}
	for uri := range types {
		t.Run(uri, func(t *testing.T) {
			RegisterTestingT(t)
			Expect(strings.HasPrefix(uri, ErrorTypeBase)).To(BeTrue())
		})
	}
}

func TestEdgeCases(t *testing.T) {
	tests := []struct {
		name              string
		build             func() *ServiceError
		expectedReason    string
		notContainStrings []string
	}{
		{
			name: "special characters in reason are preserved verbatim",
			build: func() *ServiceError {
				return BadRequest("%s", "error: <script>alert('xss')</script> & \"quotes\"")
			},
			expectedReason: "error: <script>alert('xss')</script> & \"quotes\"",
		},
		{
			name: "DatabaseAdvisoryLock hides sensitive details",
			build: func() *ServiceError {
				return DatabaseAdvisoryLock(stderrors.New("pq: deadlock detected on table pg_advisory_lock"))
			},
			expectedReason:    "internal database error",
			notContainStrings: []string{"pg_advisory_lock", "deadlock", "pq"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tt.build()
			Expect(err.Reason).To(Equal(tt.expectedReason))
			for _, s := range tt.notContainStrings {
				Expect(err.Reason).NotTo(ContainSubstring(s))
				Expect(err.Error()).NotTo(ContainSubstring(s))
				Expect(err.AsError().Error()).NotTo(ContainSubstring(s))
			}
		})
	}
}
