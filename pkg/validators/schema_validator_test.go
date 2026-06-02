package validators

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
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

    WifConfigSpec:
      type: object
      required:
        - version
        - project_id
      properties:
        version:
          type: string
        project_id:
          type: string
          minLength: 1

    ChannelSpec:
      type: object
      required:
        - display_name
      properties:
        display_name:
          type: string
          minLength: 1
`

func TestNewSchemaValidator(t *testing.T) {
	RegisterTestingT(t)

	registerRequiredSpecValidationEntities()

	// Create temporary schema file
	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "test-schema.yaml")
	err := os.WriteFile(schemaPath, []byte(testSchema), 0600)
	Expect(err).To(BeNil())

	// Test successful schema loading
	validator, err := NewSchemaValidator(schemaPath)
	Expect(err).To(BeNil())
	Expect(validator).ToNot(BeNil())
	Expect(validator.doc).ToNot(BeNil())
	Expect(validator.schemas).ToNot(BeNil())
	Expect(validator.schemas["clusters"]).ToNot(BeNil())
	Expect(validator.schemas["clusters"].Schema).ToNot(BeNil())
	Expect(validator.schemas["clusters"].TypeName).To(Equal("ClusterSpec"))
	Expect(validator.schemas["nodepools"]).ToNot(BeNil())
	Expect(validator.schemas["nodepools"].Schema).ToNot(BeNil())
	Expect(validator.schemas["nodepools"].TypeName).To(Equal("NodePoolSpec"))
	Expect(validator.HasSchema("wifconfigs")).To(BeFalse())
}

func TestNewSchemaValidator_InvalidPath(t *testing.T) {
	RegisterTestingT(t)

	// Test with non-existent file
	_, err := NewSchemaValidator("/nonexistent/path/schema.yaml")
	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("failed to load OpenAPI schema"))
}

func TestNewSchemaValidator_MalformedContent(t *testing.T) {
	RegisterTestingT(t)

	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "bad-schema.yaml")
	err := os.WriteFile(schemaPath, []byte("not: valid: openapi: {{{"), 0600)
	Expect(err).To(BeNil())

	_, err = NewSchemaValidator(schemaPath)
	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("failed to load OpenAPI schema"))
}

func TestNewSchemaValidator_MissingSchemas(t *testing.T) {
	RegisterTestingT(t)

	registerRequiredSpecValidationEntities()

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
	err := os.WriteFile(schemaPath, []byte(invalidSchema), 0600)
	Expect(err).To(BeNil())

	// Should fail because ClusterSpec is missing
	_, err = NewSchemaValidator(schemaPath)
	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("ClusterSpec schema not found"))
}

// TODO : HYPERFLEET-1159 - Uncomment this once Cluster and NodePool are registered
// func TestNewSchemaValidator_MissingRequiredEntityRegistration(t *testing.T) {
// 	RegisterTestingT(t)

// 	registry.Reset()
// 	registry.Register(registry.EntityDescriptor{
// 		Kind:           "Cluster",
// 		Plural:         "clusters",
// 		SpecSchemaName: "ClusterSpec",
// 	})

// 	tmpDir := t.TempDir()
// 	schemaPath := filepath.Join(tmpDir, "test-schema.yaml")
// 	err := os.WriteFile(schemaPath, []byte(testSchema), 0600)
// 	Expect(err).To(BeNil())

// 	_, err = NewSchemaValidator(schemaPath)
// 	Expect(err).ToNot(BeNil())
// 	Expect(err.Error()).To(ContainSubstring(`entity kind "NodePool" with SpecSchemaName must be registered`))
// }

func TestNewSchemaValidator_OptionalEntityMissingOpenAPISchema_SkipsWithWarning(t *testing.T) {
	RegisterTestingT(t)

	var logBuf bytes.Buffer
	logger.ReconfigureGlobalLogger(&logger.LogConfig{
		Level:     slog.LevelWarn,
		Format:    logger.FormatText,
		Output:    &logBuf,
		Component: "validators-test",
	})

	registerRequiredSpecValidationEntities()
	registry.Register(registry.EntityDescriptor{
		Kind:           "WifConfig",
		Plural:         "wifconfigs",
		SpecSchemaName: "WifConfigSpec",
	})

	schemaWithoutWifConfig := `
openapi: 3.0.0
info:
  title: Cluster NodePool Only
  version: 1.0.0
paths: {}
components:
  schemas:
    ClusterSpec:
      type: object
      properties:
        region:
          type: string
    NodePoolSpec:
      type: object
      properties:
        replicas:
          type: integer
`

	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "cluster-nodepool-only.yaml")
	err := os.WriteFile(schemaPath, []byte(schemaWithoutWifConfig), 0600)
	Expect(err).To(BeNil())

	validator, err := NewSchemaValidator(schemaPath)
	Expect(err).To(BeNil())
	Expect(validator.HasSchema("clusters")).To(BeTrue())
	Expect(validator.HasSchema("nodepools")).To(BeTrue())
	Expect(validator.HasSchema("wifconfigs")).To(BeFalse())

	logOutput := logBuf.String()
	Expect(logOutput).To(ContainSubstring("skipping validation for entity"))
	Expect(logOutput).To(ContainSubstring("WifConfigSpec"))
	Expect(logOutput).To(ContainSubstring("WifConfig"))
}

func TestNewSchemaValidator_RequiredEntityMissingOpenAPISchema_Fails(t *testing.T) {
	RegisterTestingT(t)

	registerRequiredSpecValidationEntities()

	schemaWithoutNodePool := `
openapi: 3.0.0
info:
  title: Cluster Only
  version: 1.0.0
paths: {}
components:
  schemas:
    ClusterSpec:
      type: object
      properties:
        region:
          type: string
`

	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "cluster-only.yaml")
	err := os.WriteFile(schemaPath, []byte(schemaWithoutNodePool), 0600)
	Expect(err).To(BeNil())

	_, err = NewSchemaValidator(schemaPath)
	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("NodePoolSpec schema not found"))
}

func TestValidate_SkipsWhenOptionalEntitySchemaNotLoaded(t *testing.T) {
	RegisterTestingT(t)

	var logBuf bytes.Buffer
	logger.ReconfigureGlobalLogger(&logger.LogConfig{
		Level:     slog.LevelWarn,
		Format:    logger.FormatText,
		Output:    &logBuf,
		Component: "validators-test",
	})

	registerRequiredSpecValidationEntities()
	registry.Register(registry.EntityDescriptor{
		Kind:           "WifConfig",
		Plural:         "wifconfigs",
		SpecSchemaName: "WifConfigSpec",
	})

	schemaWithoutWifConfig := `
openapi: 3.0.0
info:
  title: Cluster NodePool Only
  version: 1.0.0
paths: {}
components:
  schemas:
    ClusterSpec:
      type: object
      properties:
        region:
          type: string
    NodePoolSpec:
      type: object
      properties:
        replicas:
          type: integer
`

	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "cluster-nodepool-only.yaml")
	err := os.WriteFile(schemaPath, []byte(schemaWithoutWifConfig), 0600)
	Expect(err).To(BeNil())

	validator, err := NewSchemaValidator(schemaPath)
	Expect(err).To(BeNil())

	// Invalid spec would fail validation if WifConfigSpec were loaded.
	err = validator.Validate("wifconfigs", map[string]interface{}{})
	Expect(err).To(BeNil())
}

func TestValidate_WifConfigSpec_Valid(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidatorWithOptionalEntities(t)

	err := validator.Validate("wifconfigs", map[string]interface{}{
		"version":    "4.17",
		"project_id": "my-gcp-project",
	})
	Expect(err).To(BeNil())
}

func TestValidate_WifConfigSpec_MissingRequiredField(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidatorWithOptionalEntities(t)

	err := validator.Validate("wifconfigs", map[string]interface{}{
		"version": "4.17",
	})
	Expect(err).ToNot(BeNil())

	serviceErr := getServiceError(err)
	Expect(serviceErr).ToNot(BeNil())
	Expect(serviceErr.Details).ToNot(BeEmpty())
}

func TestValidate_ChannelSpec_Valid(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidatorWithOptionalEntities(t)

	err := validator.Validate("channels", map[string]interface{}{
		"display_name": "stable",
	})
	Expect(err).To(BeNil())
}

func TestValidate_ChannelSpec_MissingRequiredField(t *testing.T) {
	RegisterTestingT(t)

	validator := setupTestValidatorWithOptionalEntities(t)

	err := validator.Validate("channels", map[string]interface{}{})
	Expect(err).ToNot(BeNil())

	serviceErr := getServiceError(err)
	Expect(serviceErr).ToNot(BeNil())
	Expect(serviceErr.Details).ToNot(BeEmpty())
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
	Expect(serviceErr.Details[0].Message).ToNot(BeEmpty())
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
	t.Helper()
	registerRequiredSpecValidationEntities()

	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "test-schema.yaml")
	err := os.WriteFile(schemaPath, []byte(testSchema), 0600)
	if err != nil {
		t.Fatalf("Failed to create test schema: %v", err)
	}

	validator, err := NewSchemaValidator(schemaPath)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	return validator
}

func registerRequiredSpecValidationEntities() {
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:           "Cluster",
		Plural:         "clusters",
		SpecSchemaName: "ClusterSpec",
	})
	registry.Register(registry.EntityDescriptor{
		Kind:           "NodePool",
		Plural:         "nodepools",
		ParentKind:     "Cluster",
		SpecSchemaName: "NodePoolSpec",
	})
}

func registerOptionalSpecValidationEntities() {
	registry.Register(registry.EntityDescriptor{
		Kind:           "WifConfig",
		Plural:         "wifconfigs",
		SpecSchemaName: "WifConfigSpec",
	})
	registry.Register(registry.EntityDescriptor{
		Kind:           "Channel",
		Plural:         "channels",
		SpecSchemaName: "ChannelSpec",
	})
}

func setupTestValidatorWithOptionalEntities(t *testing.T) *SchemaValidator {
	t.Helper()
	registerRequiredSpecValidationEntities()
	registerOptionalSpecValidationEntities()

	tmpDir := t.TempDir()
	schemaPath := filepath.Join(tmpDir, "test-schema.yaml")
	err := os.WriteFile(schemaPath, []byte(testSchema), 0600)
	if err != nil {
		t.Fatalf("Failed to create test schema: %v", err)
	}

	validator, err := NewSchemaValidator(schemaPath)
	if err != nil {
		t.Fatalf("Failed to create validator: %v", err)
	}

	Expect(validator.HasSchema("wifconfigs")).To(BeTrue())
	Expect(validator.HasSchema("channels")).To(BeTrue())

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
