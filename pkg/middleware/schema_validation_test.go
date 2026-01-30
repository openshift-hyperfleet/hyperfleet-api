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

const testSchema = `
openapi: 3.0.0
info:
  title: Test Schema
  version: 1.0.0
paths: {}
components:
  schemas:
    ClusterSpec:
      type: object
      required:
        - region
      properties:
        region:
          type: string
          enum: [us-central1, us-east1]

    NodePoolSpec:
      type: object
      required:
        - replicas
      properties:
        replicas:
          type: integer
          minimum: 1
          maximum: 10
`

func TestSchemaValidationMiddleware_PostRequestValidation(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	// Create a valid cluster creation request
	validRequest := map[string]interface{}{
		"name": "test-cluster",
		"spec": map[string]interface{}{
			"region": "us-central1",
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

	// Verify next handler was called (validation passed)
	Expect(nextHandlerCalled).To(BeTrue())
	Expect(rr.Code).To(Equal(http.StatusCreated))
}

func TestSchemaValidationMiddleware_PostRequestInvalidSpec(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	// Create an invalid cluster creation request (missing required field)
	invalidRequest := map[string]interface{}{
		"name": "test-cluster",
		"spec": map[string]interface{}{
			// missing "region"
		},
	}

	body, _ := json.Marshal(invalidRequest)
	req := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
	})

	middleware(nextHandler).ServeHTTP(rr, req)

	// Verify next handler was NOT called (validation failed)
	Expect(nextHandlerCalled).To(BeFalse())
	Expect(rr.Code).To(Equal(http.StatusBadRequest))

	// Verify error response format
	var errorResponse openapi.Error
	err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
	Expect(err).To(BeNil())
	Expect(errorResponse.Code).ToNot(BeNil())
	Expect(errorResponse.Detail).ToNot(BeNil())
}

func TestSchemaValidationMiddleware_PatchRequestValidation(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	// Create a valid cluster patch request
	validRequest := map[string]interface{}{
		"spec": map[string]interface{}{
			"region": "us-east1",
		},
	}

	body, _ := json.Marshal(validRequest)
	req := httptest.NewRequest(http.MethodPatch, "/api/hyperfleet/v1/clusters/123", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	middleware(nextHandler).ServeHTTP(rr, req)

	// Verify next handler was called
	Expect(nextHandlerCalled).To(BeTrue())
	Expect(rr.Code).To(Equal(http.StatusOK))
}

func TestSchemaValidationMiddleware_GetRequestSkipped(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	// GET request should skip validation
	req := httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/clusters", nil)
	rr := httptest.NewRecorder()

	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	middleware(nextHandler).ServeHTTP(rr, req)

	// Verify next handler was called (no validation)
	Expect(nextHandlerCalled).To(BeTrue())
	Expect(rr.Code).To(Equal(http.StatusOK))
}

func TestSchemaValidationMiddleware_DeleteRequestSkipped(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	// DELETE request should skip validation
	req := httptest.NewRequest(http.MethodDelete, "/api/hyperfleet/v1/clusters/123", nil)
	rr := httptest.NewRecorder()

	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
		w.WriteHeader(http.StatusNoContent)
	})

	middleware(nextHandler).ServeHTTP(rr, req)

	// Verify next handler was called
	Expect(nextHandlerCalled).To(BeTrue())
	Expect(rr.Code).To(Equal(http.StatusNoContent))
}

func TestSchemaValidationMiddleware_NonClusterPath(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	// POST to non-cluster/nodepool path should skip validation
	validRequest := map[string]interface{}{
		"name": "test",
		"spec": map[string]interface{}{},
	}

	body, _ := json.Marshal(validRequest)
	req := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/other", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	middleware(nextHandler).ServeHTTP(rr, req)

	// Verify next handler was called (path not validated)
	Expect(nextHandlerCalled).To(BeTrue())
	Expect(rr.Code).To(Equal(http.StatusOK))
}

func TestSchemaValidationMiddleware_NodePoolValidation(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	// Create a valid nodepool request
	validRequest := map[string]interface{}{
		"name": "test-nodepool",
		"spec": map[string]interface{}{
			"replicas": 3,
		},
	}

	body, _ := json.Marshal(validRequest)
	req := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters/123/nodepools", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
		w.WriteHeader(http.StatusCreated)
	})

	middleware(nextHandler).ServeHTTP(rr, req)

	// If failed, print response for debugging
	if !nextHandlerCalled {
		t.Logf("Response code: %d", rr.Code)
		t.Logf("Response body: %s", rr.Body.String())
	}

	// Verify next handler was called
	Expect(nextHandlerCalled).To(BeTrue())
	Expect(rr.Code).To(Equal(http.StatusCreated))
}

func TestSchemaValidationMiddleware_NodePoolInvalidSpec(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	// Create an invalid nodepool request (replicas out of range)
	invalidRequest := map[string]interface{}{
		"name": "test-nodepool",
		"spec": map[string]interface{}{
			"replicas": 100, // exceeds maximum: 10
		},
	}

	body, _ := json.Marshal(invalidRequest)
	req := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters/123/nodepools", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
	})

	middleware(nextHandler).ServeHTTP(rr, req)

	// Verify next handler was NOT called
	Expect(nextHandlerCalled).To(BeFalse())
	Expect(rr.Code).To(Equal(http.StatusBadRequest))
}

func TestSchemaValidationMiddleware_MissingSpecField(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	// Request without spec field should pass through (spec is optional in request)
	requestWithoutSpec := map[string]interface{}{
		"name": "test-cluster",
	}

	body, _ := json.Marshal(requestWithoutSpec)
	req := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
		w.WriteHeader(http.StatusCreated)
	})

	middleware(nextHandler).ServeHTTP(rr, req)

	// Verify next handler was called (missing spec is ok)
	Expect(nextHandlerCalled).To(BeTrue())
}

func TestSchemaValidationMiddleware_InvalidSpecType(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	// Spec field is not an object
	invalidRequest := map[string]interface{}{
		"name": "test-cluster",
		"spec": "invalid-string",
	}

	body, _ := json.Marshal(invalidRequest)
	req := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
	})

	middleware(nextHandler).ServeHTTP(rr, req)

	// Verify next handler was NOT called
	Expect(nextHandlerCalled).To(BeFalse())
	Expect(rr.Code).To(Equal(http.StatusBadRequest))

	// Verify error message
	var errorResponse openapi.Error
	err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
	Expect(err).To(BeNil())
	Expect(*errorResponse.Detail).To(ContainSubstring("spec field must be an object"))
}

func TestSchemaValidationMiddleware_MalformedJSON(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	// Invalid JSON
	invalidJSON := []byte(`{"name": "test", "spec": {invalid}`)
	req := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", bytes.NewBuffer(invalidJSON))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
	})

	middleware(nextHandler).ServeHTTP(rr, req)

	// Verify next handler was NOT called
	Expect(nextHandlerCalled).To(BeFalse())
	Expect(rr.Code).To(Equal(http.StatusBadRequest))
}

// Helper function to setup test validator
func setupTestValidator(t *testing.T) *validators.SchemaValidator {
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "test-schema.yaml")
	err := os.WriteFile(schemaPath, []byte(testSchema), 0600)
	if err != nil {
		t.Fatalf("Failed to create test schema: %v", err)
	}

	validator, err := validators.NewSchemaValidator(schemaPath)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	return validator
}
