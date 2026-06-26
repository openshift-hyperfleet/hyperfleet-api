package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validators"
)

// testPermissiveSchema defines an OpenAPI schema with no required fields or properties
// on ClusterSpec and NodePoolSpec. This isolates the empty-spec len==0 guard from any
// VisitJSON validation that would catch missing required fields first.
const testPermissiveSchema = `
openapi: 3.0.0
info:
  title: Test Permissive Schema
  version: 1.0.0
paths: {}
components:
  schemas:
    ClusterSpec:
      type: object
    NodePoolSpec:
      type: object
`

// setupPermissiveTestValidator creates a SchemaValidator using the permissive schema
// (no required fields, no properties). This ensures the only thing catching an empty
// spec {} is the len(specMap) == 0 guard, not OpenAPI field validation.
func setupPermissiveTestValidator(t *testing.T) *validators.SchemaValidator {
	t.Helper()

	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "permissive-schema.yaml")
	err := os.WriteFile(schemaPath, []byte(testPermissiveSchema), 0600)
	if err != nil {
		t.Fatalf("Failed to create permissive test schema: %v", err)
	}

	validator, err := validators.NewSchemaValidator(schemaPath)
	if err != nil {
		t.Fatalf("Failed to create permissive validator: %v", err)
	}

	return validator
}

// TestSchemaValidationMiddleware_EmptySpecRejected_Post verifies that POST /clusters
// with an empty spec {} is rejected with 400 and HYPERFLEET-VAL-000 even when the
// schema has no required fields.
func TestSchemaValidationMiddleware_EmptySpecRejected_Post(t *testing.T) {
	RegisterTestingT(t)

	validator := setupPermissiveTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	emptySpecRequest := map[string]interface{}{
		"name": "test-cluster",
		"spec": map[string]interface{}{},
	}

	body, _ := json.Marshal(emptySpecRequest)
	req := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
	})

	middleware(nextHandler).ServeHTTP(rr, req)

	Expect(nextHandlerCalled).To(BeFalse(), "next handler must not be called for empty spec")
	Expect(rr.Code).To(Equal(http.StatusBadRequest))

	var errorResponse openapi.ProblemDetails
	err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
	Expect(err).To(BeNil())
	Expect(errorResponse.Code).ToNot(BeNil())
	Expect(*errorResponse.Code).To(Equal("HYPERFLEET-VAL-000"))
	Expect(errorResponse.Detail).ToNot(BeNil())
	Expect(*errorResponse.Detail).To(ContainSubstring("spec must not be empty"))
}

// TestSchemaValidationMiddleware_EmptySpecRejected_Patch verifies that PATCH /clusters/<uuid>
// with an empty spec {} is rejected with 400 and HYPERFLEET-VAL-000.
func TestSchemaValidationMiddleware_EmptySpecRejected_Patch(t *testing.T) {
	RegisterTestingT(t)

	validator := setupPermissiveTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	emptySpecRequest := map[string]interface{}{
		"spec": map[string]interface{}{},
	}

	body, _ := json.Marshal(emptySpecRequest)
	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/hyperfleet/v1/clusters/550e8400-e29b-41d4-a716-446655440000",
		bytes.NewBuffer(body),
	)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
	})

	middleware(nextHandler).ServeHTTP(rr, req)

	Expect(nextHandlerCalled).To(BeFalse(), "next handler must not be called for empty spec on PATCH")
	Expect(rr.Code).To(Equal(http.StatusBadRequest))

	var errorResponse openapi.ProblemDetails
	err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
	Expect(err).To(BeNil())
	Expect(errorResponse.Code).ToNot(BeNil())
	Expect(*errorResponse.Code).To(Equal("HYPERFLEET-VAL-000"))
	Expect(errorResponse.Detail).ToNot(BeNil())
	Expect(*errorResponse.Detail).To(ContainSubstring("spec must not be empty"))
}

// TestSchemaValidationMiddleware_EmptySpecRejected_Nodepool verifies that POST
// /clusters/<uuid>/nodepools with an empty spec {} is rejected with 400 and
// HYPERFLEET-VAL-000.
func TestSchemaValidationMiddleware_EmptySpecRejected_Nodepool(t *testing.T) {
	RegisterTestingT(t)

	validator := setupPermissiveTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	emptySpecRequest := map[string]interface{}{
		"name": "test-nodepool",
		"spec": map[string]interface{}{},
	}

	body, _ := json.Marshal(emptySpecRequest)
	req := httptest.NewRequest(
		http.MethodPost,
		"/api/hyperfleet/v1/clusters/550e8400-e29b-41d4-a716-446655440000/nodepools",
		bytes.NewBuffer(body),
	)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
	})

	middleware(nextHandler).ServeHTTP(rr, req)

	Expect(nextHandlerCalled).To(BeFalse(), "next handler must not be called for empty nodepool spec")
	Expect(rr.Code).To(Equal(http.StatusBadRequest))

	var errorResponse openapi.ProblemDetails
	err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
	Expect(err).To(BeNil())
	Expect(errorResponse.Code).ToNot(BeNil())
	Expect(*errorResponse.Code).To(Equal("HYPERFLEET-VAL-000"))
	Expect(errorResponse.Detail).ToNot(BeNil())
	Expect(*errorResponse.Detail).To(ContainSubstring("spec must not be empty"))
}

// TestSchemaValidationMiddleware_NonEmptySpecPassesPermissive verifies that a non-empty
// spec with arbitrary fields passes validation when using the permissive schema (no
// required fields). This confirms the guard only blocks truly empty specs.
func TestSchemaValidationMiddleware_NonEmptySpecPassesPermissive(t *testing.T) {
	RegisterTestingT(t)

	validator := setupPermissiveTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	validRequest := map[string]interface{}{
		"name": "test-cluster",
		"spec": map[string]interface{}{
			"foo": "bar",
		},
	}

	body, _ := json.Marshal(validRequest)
	req := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
		w.WriteHeader(http.StatusCreated)
	})

	middleware(nextHandler).ServeHTTP(rr, req)

	Expect(nextHandlerCalled).To(BeTrue(), "non-empty spec should pass through with permissive schema")
	Expect(rr.Code).To(Equal(http.StatusCreated))
}
