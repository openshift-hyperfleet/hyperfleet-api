package validators

import (
	"context"
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

// ResourceSchema represents a validation schema for a specific resource type
type ResourceSchema struct {
	TypeName string
	Schema   *openapi3.SchemaRef
}

// SchemaValidator validates JSON objects against OpenAPI schemas
type SchemaValidator struct {
	doc     *openapi3.T
	schemas map[string]*ResourceSchema
}

// NewSchemaValidator creates a new schema validator by loading an OpenAPI spec from the given path
func NewSchemaValidator(schemaPath string) (*SchemaValidator, error) {
	// Load OpenAPI spec
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI schema from %s: %w", schemaPath, err)
	}

	// Validate the loaded document
	if err := doc.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("invalid OpenAPI schema: %w", err)
	}

	// Extract ClusterSpec schema
	clusterSpecSchema := doc.Components.Schemas["ClusterSpec"]
	if clusterSpecSchema == nil {
		return nil, fmt.Errorf("ClusterSpec schema not found in OpenAPI spec")
	}

	// Extract NodePoolSpec schema
	nodePoolSpecSchema := doc.Components.Schemas["NodePoolSpec"]
	if nodePoolSpecSchema == nil {
		return nil, fmt.Errorf("NodePoolSpec schema not found in OpenAPI spec")
	}

	// Build schemas map
	schemas := map[string]*ResourceSchema{
		"cluster": {
			TypeName: "ClusterSpec",
			Schema:   clusterSpecSchema,
		},
		"nodepool": {
			TypeName: "NodePoolSpec",
			Schema:   nodePoolSpecSchema,
		},
	}

	return &SchemaValidator{
		doc:     doc,
		schemas: schemas,
	}, nil
}

// Validate validates a spec for the given resource type
// Returns nil if resourceType is not found in schemas (allows graceful handling)
func (v *SchemaValidator) Validate(resourceType string, spec map[string]interface{}) error {
	resourceSchema := v.schemas[resourceType]
	if resourceSchema == nil {
		// Unknown resource type, skip validation
		return nil
	}

	return v.validateSpec(spec, resourceSchema.Schema, resourceSchema.TypeName)
}

// ValidateClusterSpec validates a cluster spec against the ClusterSpec schema
// Deprecated: Use Validate("cluster", spec) instead
func (v *SchemaValidator) ValidateClusterSpec(spec map[string]interface{}) error {
	return v.Validate("cluster", spec)
}

// ValidateNodePoolSpec validates a nodepool spec against the NodePoolSpec schema
// Deprecated: Use Validate("nodepool", spec) instead
func (v *SchemaValidator) ValidateNodePoolSpec(spec map[string]interface{}) error {
	return v.Validate("nodepool", spec)
}

// validateSpec performs the actual validation and converts errors to our error format
func (v *SchemaValidator) validateSpec(spec map[string]interface{}, schemaRef *openapi3.SchemaRef, specTypeName string) error {
	// Cast spec to interface{} for VisitJSON
	var specData interface{} = spec

	// Validate against schema
	if err := schemaRef.Value.VisitJSON(specData); err != nil {
		// Convert validation error to our error format with details
		validationDetails := convertValidationError(err, "spec")
		return errors.ValidationWithDetails(
			fmt.Sprintf("Invalid %s", specTypeName),
			validationDetails,
		)
	}

	return nil
}

// convertValidationError converts OpenAPI validation errors to our ValidationDetail format
func convertValidationError(err error, prefix string) []errors.ValidationDetail {
	var details []errors.ValidationDetail

	switch e := err.(type) {
	case openapi3.MultiError:
		// Recursively process each sub-error
		for _, subErr := range e {
			subDetails := convertValidationError(subErr, prefix)
			details = append(details, subDetails...)
		}
	case *openapi3.SchemaError:
		// Extract field path from SchemaError
		field := prefix

		// Use JSONPointer which contains the actual data path
		// JSONPointer returns the path like ["platform", "gcp", "diskSize"]
		if len(e.JSONPointer()) > 0 {
			jsonPath := strings.Join(e.JSONPointer(), ".")
			if jsonPath != "" {
				field = prefix + "." + jsonPath
			}
		}

		// Use the error message (Reason) which already contains field information
		// Examples:
		//   - "property 'region' is missing"
		//   - "property 'unknownField' is unsupported"
		//   - "number must be at least 10"
		details = append(details, errors.ValidationDetail{
			Field: field,
			Error: e.Reason,
		})
	default:
		// Fallback for unknown error types
		// Error message already contains the full description
		details = append(details, errors.ValidationDetail{
			Field: prefix,
			Error: err.Error(),
		})
	}

	return details
}
