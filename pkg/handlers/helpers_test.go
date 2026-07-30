package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

func TestWriteJSONResponse(t *testing.T) {
	RegisterTestingT(t)

	type testPayload struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	t.Run("writes JSON response with correct headers", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		payload := testPayload{ID: "999999", Name: "test"}

		writeJSONResponse(w, r, http.StatusOK, payload)

		Expect(w.Code).To(Equal(http.StatusOK))
		Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))
		Expect(w.Header().Get("Vary")).To(Equal("Authorization"))

		var response testPayload
		err := json.Unmarshal(w.Body.Bytes(), &response)
		Expect(err).To(BeNil())
		Expect(response.ID).To(Equal("999999"))
		Expect(response.Name).To(Equal("test"))
	})

	t.Run("handles nil payload correctly", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()

		writeJSONResponse(w, r, http.StatusNoContent, nil)

		Expect(w.Code).To(Equal(http.StatusNoContent))
		Expect(w.Header().Get("Content-Type")).To(Equal("application/json"))
		Expect(w.Body.Len()).To(Equal(0))
	})

	t.Run("sets correct status code for different responses", func(t *testing.T) {
		testCases := []struct {
			name       string
			statusCode int
		}{
			{"200 OK", http.StatusOK},
			{"201 Created", http.StatusCreated},
			{"202 Accepted", http.StatusAccepted},
			{"204 No Content", http.StatusNoContent},
			{"400 Bad Request", http.StatusBadRequest},
			{"404 Not Found", http.StatusNotFound},
			{"500 Internal Server Error", http.StatusInternalServerError},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				r := httptest.NewRequest(http.MethodGet, "/test", nil)
				w := httptest.NewRecorder()

				writeJSONResponse(w, r, tc.statusCode, testPayload{ID: "test"})

				Expect(w.Code).To(Equal(tc.statusCode))
			})
		}
	})

	t.Run("handles empty payload", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()

		writeJSONResponse(w, r, http.StatusOK, map[string]any{})

		Expect(w.Code).To(Equal(http.StatusOK))
		Expect(w.Body.String()).To(Equal("{}"))
	})

	t.Run("handles arrays", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		payload := []testPayload{
			{ID: "1", Name: "first"},
			{ID: "2", Name: "second"},
		}

		writeJSONResponse(w, r, http.StatusOK, payload)

		Expect(w.Code).To(Equal(http.StatusOK))
		var response []testPayload
		err := json.Unmarshal(w.Body.Bytes(), &response)
		Expect(err).To(BeNil())
		Expect(response).To(HaveLen(2))
		Expect(response[0].ID).To(Equal("1"))
		Expect(response[1].ID).To(Equal("2"))
	})
}
func TestDecodeAndValidate(t *testing.T) {
	RegisterTestingT(t)

	const validJSON = `{"name":"test","value":42}`
	const unknownFieldJSON = `{"name":"test","value":42,"unknown":"field"}`
	type testRequest struct {
		Name  string `json:"name"`
		Value int    `json:"value"`
	}

	t.Run("default decodeType - valid JSON", func(t *testing.T) {
		body := validJSON
		r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		var req testRequest
		var validateCalled bool
		validateFuncs := []validate{
			func() *errors.ServiceError {
				validateCalled = true
				return nil
			},
		}

		err := decodeAndValidate(r, &req, validateFuncs)

		Expect(err).To(BeNil())
		Expect(req.Name).To(Equal("test"))
		Expect(req.Value).To(Equal(42))
		Expect(validateCalled).To(BeTrue())
	})

	t.Run("default decodeType - allows unknown fields", func(t *testing.T) {
		body := unknownFieldJSON // includes "unknown":"field" which will be silently discarded
		r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		var req testRequest

		err := decodeAndValidate(r, &req, []validate{})

		Expect(err).To(BeNil())
		Expect(req.Name).To(Equal("test"))
		Expect(req.Value).To(Equal(42))
		// The "unknown" field is silently ignored by json.Unmarshal (no error, not stored anywhere)
	})

	t.Run("undefined decodeType - accepts unknown field JSON", func(t *testing.T) {
		body := unknownFieldJSON
		r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		var req testRequest

		err := decodeAndValidate(r, &req, []validate{}, "unknown")

		Expect(err).NotTo(BeNil())
		Expect(err.Reason).To(ContainSubstring("invalid decode mode: unknown (must be 'strict')"))
	})

	t.Run("strict mode - rejects unknown fields", func(t *testing.T) {
		body := unknownFieldJSON
		r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		var req testRequest

		err := decodeAndValidate(r, &req, []validate{}, "strict")

		Expect(err).NotTo(BeNil())
		Expect(err.HTTPCode).To(Equal(http.StatusBadRequest))
		Expect(err.Reason).To(ContainSubstring("Invalid request format"))
	})

	t.Run("strict mode - accepts valid JSON without extra fields", func(t *testing.T) {
		body := validJSON
		r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		var req testRequest

		err := decodeAndValidate(r, &req, []validate{}, "strict")

		Expect(err).To(BeNil())
		Expect(req.Name).To(Equal("test"))
		Expect(req.Value).To(Equal(42))
	})

	t.Run("malformed JSON returns error", func(t *testing.T) {
		body := `{"name":"test",invalid}`
		r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		var req testRequest

		err := decodeAndValidate(r, &req, []validate{})

		Expect(err).NotTo(BeNil())
		Expect(err.HTTPCode).To(Equal(http.StatusBadRequest))
		Expect(err.Reason).To(ContainSubstring("Invalid request format"))
	})

	t.Run("type mismatch returns validation error", func(t *testing.T) {
		body := `{"name":"test","value":"not-a-number"}`
		r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		var req testRequest

		err := decodeAndValidate(r, &req, []validate{})

		Expect(err).NotTo(BeNil())
		Expect(err.HTTPCode).To(Equal(http.StatusBadRequest))
		Expect(err.Reason).To(ContainSubstring("must be a number"))
	})

	t.Run("validation function fails", func(t *testing.T) {
		body := validJSON
		r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(body))
		var req testRequest
		validateFuncs := []validate{
			func() *errors.ServiceError {
				return errors.Validation("name is too short")
			},
		}

		err := decodeAndValidate(r, &req, validateFuncs)

		Expect(err).NotTo(BeNil())
		Expect(err.Reason).To(ContainSubstring("name is too short"))
	})

	t.Run("empty body returns error", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader(""))
		var req testRequest

		err := decodeAndValidate(r, &req, []validate{})

		Expect(err).NotTo(BeNil())
		Expect(err.HTTPCode).To(Equal(http.StatusBadRequest))
		Expect(err.Reason).To(ContainSubstring("Request body is required but was empty"))
	})
}

func TestCleanTypeMismatchError(t *testing.T) {
	RegisterTestingT(t)

	type testStruct struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
		Flag  bool   `json:"flag"`
	}

	t.Run("string type mismatch", func(t *testing.T) {
		body := `{"name":123}`
		var req testStruct
		err := json.Unmarshal([]byte(body), &req)

		svcErr := cleanTypeMismatchError(err)

		Expect(svcErr).NotTo(BeNil())
		Expect(svcErr.Reason).To(ContainSubstring("'name' must be a string"))
	})

	t.Run("number type mismatch", func(t *testing.T) {
		body := `{"count":"not-a-number"}`
		var req testStruct
		err := json.Unmarshal([]byte(body), &req)

		svcErr := cleanTypeMismatchError(err)

		Expect(svcErr).NotTo(BeNil())
		Expect(svcErr.Reason).To(ContainSubstring("'count' must be a number"))
	})

	t.Run("boolean type mismatch", func(t *testing.T) {
		body := `{"flag":"true"}`
		var req testStruct
		err := json.Unmarshal([]byte(body), &req)

		svcErr := cleanTypeMismatchError(err)

		Expect(svcErr).NotTo(BeNil())
		Expect(svcErr.Reason).To(ContainSubstring("'flag' must be a boolean"))
	})

	t.Run("non-type-mismatch error returns nil", func(t *testing.T) {
		body := `{invalid json}`
		var req testStruct
		err := json.Unmarshal([]byte(body), &req)

		svcErr := cleanTypeMismatchError(err)

		Expect(svcErr).To(BeNil())
	})

	t.Run("nil error returns nil", func(t *testing.T) {
		svcErr := cleanTypeMismatchError(nil)

		Expect(svcErr).To(BeNil())
	})
}

func TestDescribeJSONKind(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		expected string
		kind     reflect.Kind
	}{
		{"an object", reflect.Map},
		{"an object", reflect.Struct},
		{"an array", reflect.Slice},
		{"an array", reflect.Array},
		{"a string", reflect.String},
		{"a boolean", reflect.Bool},
		{"a number", reflect.Int},
		{"a number", reflect.Int8},
		{"a number", reflect.Int16},
		{"a number", reflect.Int32},
		{"a number", reflect.Int64},
		{"a number", reflect.Uint},
		{"a number", reflect.Uint8},
		{"a number", reflect.Uint16},
		{"a number", reflect.Uint32},
		{"a number", reflect.Uint64},
		{"a number", reflect.Float32},
		{"a number", reflect.Float64},
		{"the correct type", reflect.Invalid},
	}

	for _, tt := range tests {
		t.Run(tt.kind.String(), func(t *testing.T) {
			result := describeJSONKind(tt.kind)
			Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestApplyFieldFilter(t *testing.T) {
	RegisterTestingT(t)

	type testResource struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		Metadata string `json:"metadata"`
	}

	presented := testResource{
		ID:       "123",
		Name:     "test",
		Metadata: "2026-07-28",
	}

	t.Run("no fields parameter returns original", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)

		result, err := applyFieldFilter(r, presented)

		Expect(err).To(BeNil())
		Expect(result).To(Equal(presented))
	})

	t.Run("fields parameter filters correctly", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test?fields=id,name", nil)

		result, err := applyFieldFilter(r, presented)

		Expect(err).To(BeNil())
		filtered, ok := result.(map[string]any)
		Expect(ok).To(BeTrue())
		Expect(filtered).To(HaveKey("id"))
		Expect(filtered).To(HaveKey("name"))
		Expect(filtered).NotTo(HaveKey("metadata"))
	})

	t.Run("invalid fields parameter returns error", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test?fields=nonexistent", nil)

		result, err := applyFieldFilter(r, presented)

		Expect(err).NotTo(BeNil())
		Expect(err.HTTPCode).To(Equal(http.StatusBadRequest))
		Expect(result).To(BeNil())
	})
}

func TestHandleError(t *testing.T) {
	RegisterTestingT(t)

	t.Run("writes problem details response", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		svcErr := errors.NotFound("resource not found")

		handleError(r, w, svcErr)

		Expect(w.Code).To(Equal(http.StatusNotFound))
		Expect(w.Header().Get("Content-Type")).To(Equal("application/problem+json"))

		var response map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &response)
		Expect(err).To(BeNil())
		Expect(response["detail"]).To(ContainSubstring("resource not found"))
	})

	t.Run("sets HTTP status code correctly", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		svcErr := errors.Validation("validation failed")

		handleError(r, w, svcErr)

		Expect(w.Code).To(Equal(http.StatusBadRequest))
	})
}
