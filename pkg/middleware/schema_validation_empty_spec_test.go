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

// Permissive schema isolates the len==0 guard from VisitJSON — production
// ClusterSpec/NodePoolSpec have no required fields either.
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

func TestSchemaValidationMiddleware_EmptySpecRejection(t *testing.T) {
	RegisterTestingT(t)

	validator := setupPermissiveTestValidator(t)
	middleware := SchemaValidationMiddleware(validator)

	tests := []struct {
		requestBody map[string]any
		name        string
		method      string
		path        string
		expectBlock bool
	}{
		{
			name:   "POST cluster with empty spec rejected",
			method: http.MethodPost,
			path:   "/api/hyperfleet/v1/clusters",
			requestBody: map[string]any{
				"name": "test-cluster",
				"spec": map[string]any{},
			},
			expectBlock: true,
		},
		{
			name:   "PATCH cluster with empty spec rejected",
			method: http.MethodPatch,
			path:   "/api/hyperfleet/v1/clusters/550e8400-e29b-41d4-a716-446655440000",
			requestBody: map[string]any{
				"spec": map[string]any{},
			},
			expectBlock: true,
		},
		{
			name:   "POST nodepool with empty spec rejected",
			method: http.MethodPost,
			path:   "/api/hyperfleet/v1/clusters/550e8400-e29b-41d4-a716-446655440000/nodepools",
			requestBody: map[string]any{
				"name": "test-nodepool",
				"spec": map[string]any{},
			},
			expectBlock: true,
		},
		{
			name:   "POST cluster with non-empty spec passes permissive schema",
			method: http.MethodPost,
			path:   "/api/hyperfleet/v1/clusters",
			requestBody: map[string]any{
				"name": "test-cluster",
				"spec": map[string]any{"foo": "bar"},
			},
			expectBlock: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			body, _ := json.Marshal(tt.requestBody)
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBuffer(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			nextHandlerCalled := false
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				nextHandlerCalled = true
				w.WriteHeader(http.StatusCreated)
			})

			middleware(nextHandler).ServeHTTP(rr, req)

			if tt.expectBlock {
				Expect(nextHandlerCalled).To(BeFalse())
				Expect(rr.Code).To(Equal(http.StatusBadRequest))

				var errorResponse openapi.ProblemDetails
				err := json.Unmarshal(rr.Body.Bytes(), &errorResponse)
				Expect(err).To(BeNil())
				Expect(errorResponse.Code).ToNot(BeNil())
				Expect(*errorResponse.Code).To(Equal("HYPERFLEET-VAL-000"))
				Expect(errorResponse.Detail).ToNot(BeNil())
				Expect(*errorResponse.Detail).To(ContainSubstring("spec must not be empty"))
			} else {
				Expect(nextHandlerCalled).To(BeTrue())
				Expect(rr.Code).To(Equal(http.StatusCreated))
			}
		})
	}
}
