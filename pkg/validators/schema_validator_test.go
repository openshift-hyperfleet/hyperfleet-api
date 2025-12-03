package validators

import (
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
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
        - provider
      properties:
        region:
          type: string
          enum: [us-central1, us-east1, europe-west1]
        provider:
          type: string
          enum: [gcp, aws]
        network:
          type: object
          properties:
            vpc_id:
              type: string
              minLength: 1

    NodePoolSpec:
      type: object
      required:
        - machine_type
        - replicas
      properties:
        machine_type:
          type: string
          minLength: 1
        replicas:
          type: integer
          minimum: 1
          maximum: 100
        autoscaling:
          type: boolean
`

func TestNewSchemaValidator(t *testing.T) {
	RegisterTestingT(t)

	// Create temporary schema file
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "test-schema.yaml")
	err := os.WriteFile(schemaPath, []byte(testSchema), 0644)
	Expect(err).To(BeNil())

	// Test successful schema loading
	validator, err := NewSchemaValidator(schemaPath)
	Expect(err).To(BeNil())
	Expect(validator).ToNot(BeNil())
	Expect(validator.doc).ToNot(BeNil())
	Expect(validator.schemas).ToNot(BeNil())
	Expect(validator.schemas["cluster"]).ToNot(BeNil())
	Expect(validator.schemas["cluster"].Schema).ToNot(BeNil())
	Expect(validator.schemas["cluster"].TypeName).To(Equal("ClusterSpec"))
	Expect(validator.schemas["nodepool"]).ToNot(BeNil())
	Expect(validator.schemas["nodepool"].Schema).ToNot(BeNil())
	Expect(validator.schemas["nodepool"].TypeName).To(Equal("NodePoolSpec"))
}

func TestNewSchemaValidator_InvalidPath(t *testing.T) {
	RegisterTestingT(t)

	// Test with non-existent file
	_, err := NewSchemaValidator("/nonexistent/path/schema.yaml")
	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("failed to load OpenAPI schema"))
}

func TestNewSchemaValidator_MissingSchemas(t *testing.T) {
	RegisterTestingT(t)

	// Schema without required components
	invalidSchema := `
openapi: 3.0.0
info:
  title: Invalid Schema
  version: 1.0.0
paths: {}
components:
  schemas:
    SomeOtherSchema:
      type: object
`

	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "invalid-schema.yaml")
	err := os.WriteFile(schemaPath, []byte(invalidSchema), 0644)
	Expect(err).To(BeNil())

	// Should fail because ClusterSpec is missing
	_, err = NewSchemaValidator(schemaPath)
	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("ClusterSpec schema not found"))
}

func TestValidateClusterSpec_Valid(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)

	// Test valid cluster spec
	validSpec := map[string]interface{}{
		"region":   "us-central1",
		"provider": "gcp",
	}

	err := validator.ValidateClusterSpec(validSpec)
	Expect(err).To(BeNil())
}

func TestValidateClusterSpec_ValidWithOptionalFields(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)

	// Test valid cluster spec with optional network field
	validSpec := map[string]interface{}{
		"region":   "europe-west1",
		"provider": "aws",
		"network": map[string]interface{}{
			"vpc_id": "vpc-12345",
		},
	}

	err := validator.ValidateClusterSpec(validSpec)
	Expect(err).To(BeNil())
}

func TestValidateClusterSpec_MissingRequiredField(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)

	// Test spec missing required field
	invalidSpec := map[string]interface{}{
		"region": "us-central1",
		// missing "provider"
	}

	err := validator.ValidateClusterSpec(invalidSpec)
	Expect(err).ToNot(BeNil())

	// Check that error is a ServiceError with details
	serviceErr := getServiceError(err)
	Expect(serviceErr).ToNot(BeNil())
	Expect(serviceErr.Details).ToNot(BeEmpty())
}

func TestValidateClusterSpec_InvalidEnum(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)

	// Test spec with invalid enum value
	invalidSpec := map[string]interface{}{
		"region":   "asia-southeast1", // not in enum
		"provider": "gcp",
	}

	err := validator.ValidateClusterSpec(invalidSpec)
	Expect(err).ToNot(BeNil())

	serviceErr := getServiceError(err)
	Expect(serviceErr).ToNot(BeNil())
	Expect(serviceErr.Details).ToNot(BeEmpty())

	// Verify we get validation details (field path extraction is tested separately)
	Expect(serviceErr.Details[0].Error).ToNot(BeEmpty())
}

func TestValidateClusterSpec_InvalidNestedField(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)

	// Test spec with invalid nested field (empty vpc_id)
	invalidSpec := map[string]interface{}{
		"region":   "us-central1",
		"provider": "gcp",
		"network": map[string]interface{}{
			"vpc_id": "", // violates minLength: 1
		},
	}

	err := validator.ValidateClusterSpec(invalidSpec)
	Expect(err).ToNot(BeNil())

	serviceErr := getServiceError(err)
	Expect(serviceErr).ToNot(BeNil())
}

func TestValidateNodePoolSpec_Valid(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)

	// Test valid nodepool spec
	validSpec := map[string]interface{}{
		"machine_type": "n1-standard-4",
		"replicas":     3,
	}

	err := validator.ValidateNodePoolSpec(validSpec)
	Expect(err).To(BeNil())
}

func TestValidateNodePoolSpec_ValidWithOptional(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)

	// Test valid nodepool spec with optional autoscaling
	validSpec := map[string]interface{}{
		"machine_type": "n1-standard-4",
		"replicas":     5,
		"autoscaling":  true,
	}

	err := validator.ValidateNodePoolSpec(validSpec)
	Expect(err).To(BeNil())
}

func TestValidateNodePoolSpec_MissingRequiredField(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)

	// Test spec missing required field
	invalidSpec := map[string]interface{}{
		"machine_type": "n1-standard-4",
		// missing "replicas"
	}

	err := validator.ValidateNodePoolSpec(invalidSpec)
	Expect(err).ToNot(BeNil())

	serviceErr := getServiceError(err)
	Expect(serviceErr).ToNot(BeNil())
	Expect(serviceErr.Details).ToNot(BeEmpty())
}

func TestValidateNodePoolSpec_InvalidType(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)

	// Test spec with wrong type (replicas should be integer)
	invalidSpec := map[string]interface{}{
		"machine_type": "n1-standard-4",
		"replicas":     "three", // should be integer
	}

	err := validator.ValidateNodePoolSpec(invalidSpec)
	Expect(err).ToNot(BeNil())

	serviceErr := getServiceError(err)
	Expect(serviceErr).ToNot(BeNil())
}

func TestValidateNodePoolSpec_OutOfRange(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)

	// Test spec with value out of range
	invalidSpec := map[string]interface{}{
		"machine_type": "n1-standard-4",
		"replicas":     150, // exceeds maximum: 100
	}

	err := validator.ValidateNodePoolSpec(invalidSpec)
	Expect(err).ToNot(BeNil())

	serviceErr := getServiceError(err)
	Expect(serviceErr).ToNot(BeNil())
}

func TestValidateNodePoolSpec_BelowMinimum(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)

	// Test spec with value below minimum
	invalidSpec := map[string]interface{}{
		"machine_type": "n1-standard-4",
		"replicas":     0, // below minimum: 1
	}

	err := validator.ValidateNodePoolSpec(invalidSpec)
	Expect(err).ToNot(BeNil())
}

func TestValidateNodePoolSpec_EmptyMachineType(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidator(t)

	// Test spec with empty machine_type (violates minLength)
	invalidSpec := map[string]interface{}{
		"machine_type": "",
		"replicas":     3,
	}

	err := validator.ValidateNodePoolSpec(invalidSpec)
	Expect(err).ToNot(BeNil())
}

// Helper functions

func setupTestValidator(t *testing.T) *SchemaValidator {
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "test-schema.yaml")
	err := os.WriteFile(schemaPath, []byte(testSchema), 0644)
	if err != nil {
		t.Fatalf("Failed to create test schema: %v", err)
	}

	validator, err := NewSchemaValidator(schemaPath)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	return validator
}

func getServiceError(err error) *errors.ServiceError {
	if err == nil {
		return nil
	}

	// Import errors package at the top
	// This is a type assertion to check if err is a *errors.ServiceError
	if serviceErr, ok := err.(*errors.ServiceError); ok {
		return serviceErr
	}

	return nil
}
