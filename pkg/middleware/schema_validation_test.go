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
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
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

    FooSpec:
      type: object
      required:
        - bar
      properties:
        bar:
          type: string
          minLength: 1
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
	var errorResponse openapi.ProblemDetails
	err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
	Expect(err).To(BeNil())
	Expect(errorResponse.Type).ToNot(BeEmpty())
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
	req := httptest.NewRequest(http.MethodDelete, "/api/hyperfleet/v1/clusters/550e8400-e29b-41d4-a716-446655440000", nil)
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

func TestSchemaValidationMiddleware_NestedNodePoolPathUsesNodePoolSchema(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	// Valid for ClusterSpec (region) but invalid for NodePoolSpec (missing replicas).
	clusterOnlySpec := map[string]interface{}{
		"name": "test-nodepool",
		"spec": map[string]interface{}{
			"region": "us-central1",
		},
	}

	body, _ := json.Marshal(clusterOnlySpec)
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

	Expect(nextHandlerCalled).To(BeFalse(), "cluster-shaped spec must not pass on nested nodepool path")
	Expect(rr.Code).To(Equal(http.StatusBadRequest))
}

func TestSchemaValidationMiddleware_NestedNodePoolPathRejectsClusterSpecOnPatch(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	clusterOnlySpec := map[string]interface{}{
		"spec": map[string]interface{}{
			"region": "us-east1",
		},
	}

	body, _ := json.Marshal(clusterOnlySpec)
	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/hyperfleet/v1/clusters/550e8400-e29b-41d4-a716-446655440000/nodepools/660e8400-e29b-41d4-a716-446655440001",
		bytes.NewBuffer(body),
	)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	nextHandlerCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextHandlerCalled = true
	})

	middleware(nextHandler).ServeHTTP(rr, req)

	Expect(nextHandlerCalled).To(BeFalse())
	Expect(rr.Code).To(Equal(http.StatusBadRequest))
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
	var errorResponse openapi.ProblemDetails
	err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
	Expect(err).To(BeNil())
	Expect(errorResponse.Detail).ToNot(BeNil())
	Expect(*errorResponse.Detail).To(ContainSubstring("spec field must be an object"))
}

// TestSchemaValidationMiddleware_RegisteredEntityUsesValidator ensures entities from
// registry.WithSpecSchema() with a matching OpenAPI component are validated end-to-end.
func TestSchemaValidationMiddleware_RegisteredEntityUsesValidator(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)
	Expect(validator.HasSchema("foos")).To(BeTrue(), "FooSpec must be loaded for registered plural foos")

	middleware := SchemaValidationMiddleware(validator)

	t.Run("valid spec passes validation and reaches next handler", func(t *testing.T) {
		validRequest := map[string]interface{}{
			"kind": "Foo",
			"name": "test-foo",
			"spec": map[string]interface{}{
				"bar": "ok",
			},
		}

		body, err := json.Marshal(validRequest)
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/foos", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		nextHandlerCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextHandlerCalled = true
			w.WriteHeader(http.StatusCreated)
		})

		middleware(nextHandler).ServeHTTP(rr, req)

		Expect(nextHandlerCalled).To(BeTrue(), "validator should accept valid FooSpec")
		Expect(rr.Code).To(Equal(http.StatusCreated))
	})

	t.Run("invalid spec is rejected by validator before next handler", func(t *testing.T) {
		invalidRequest := map[string]interface{}{
			"kind": "Foo",
			"name": "test-foo-invalid",
			"spec": map[string]interface{}{},
		}

		body, err := json.Marshal(invalidRequest)
		Expect(err).NotTo(HaveOccurred())

		req := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/foos", bytes.NewBuffer(body))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()

		nextHandlerCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextHandlerCalled = true
		})

		middleware(nextHandler).ServeHTTP(rr, req)

		Expect(nextHandlerCalled).To(BeFalse(), "validator should reject invalid FooSpec")
		Expect(rr.Code).To(Equal(http.StatusBadRequest))

		var errorResponse openapi.ProblemDetails
		err = json.Unmarshal(rr.Body.Bytes(), &errorResponse)
		Expect(err).NotTo(HaveOccurred())
		Expect(errorResponse.Code).ToNot(BeNil())
		Expect(*errorResponse.Code).To(Equal("HYPERFLEET-VAL-000"))
		Expect(errorResponse.Detail).ToNot(BeNil())
		Expect(*errorResponse.Detail).To(ContainSubstring("Invalid FooSpec"))
	})
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
	t.Helper()
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:           "Foo",
		Plural:         "foos",
		SpecSchemaName: "FooSpec",
	})

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
