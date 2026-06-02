package validators

import (
	"context"
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
)

// TODO : HYPERFLEET-1159 - Uncomment this once Cluster and NodePool are registered
// var requiredSpecValidationKinds = []string{"Cluster", "NodePool"}

// ResourceSchema represents a validation schema for a specific resource type
type ResourceSchema struct {
	Schema   *openapi3.SchemaRef
	TypeName string
}

// SchemaValidator validates JSON objects against OpenAPI schemas
type SchemaValidator struct {
	doc     *openapi3.T
	schemas map[string]*ResourceSchema
}

// NewSchemaValidator creates a new schema validator by loading an OpenAPI spec from the given path.
// Cluster and NodePool must be registered with SpecSchemaName and have matching OpenAPI components.
// Other registered entities with SpecSchemaName are validated only when their component exists;
// missing components are skipped with a warning at startup.
func NewSchemaValidator(schemaPath string) (*SchemaValidator, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI schema from %s: %w", schemaPath, err)
	}

	if validateErr := doc.Validate(context.Background()); validateErr != nil {
		return nil, fmt.Errorf("invalid OpenAPI schema: %w", validateErr)
	}

	schemas, err := buildSchemasMap(doc)
	if err != nil {
		return nil, err
	}

	return &SchemaValidator{
		doc:     doc,
		schemas: schemas,
	}, nil
}

func buildSchemasMap(doc *openapi3.T) (map[string]*ResourceSchema, error) {
	ctx := context.Background()
	schemas := make(map[string]*ResourceSchema)
	// registeredKinds := make(map[string]bool, len(requiredSpecValidationKinds))

	for _, d := range registry.WithSpecSchema() {
		schemaRef := doc.Components.Schemas[d.SpecSchemaName]
		if schemaRef == nil {
			// TODO : HYPERFLEET-1159 - Uncomment this once Cluster and NodePool are registered
			// if isRequiredSpecValidationKind(d.Kind) {
			// 	return nil, fmt.Errorf(
			// 		"%s schema not found in OpenAPI spec (required for entity kind %q)",
			// 		d.SpecSchemaName, d.Kind,
			// 	)
			// }

			logger.With(ctx,
				"schema_name", d.SpecSchemaName,
				"kind", d.Kind,
				"plural", d.Plural,
			).Warn("OpenAPI spec schema not found, skipping validation for entity")
			continue
		}

		schemas[d.Plural] = &ResourceSchema{
			TypeName: d.SpecSchemaName,
			Schema:   schemaRef,
		}
		// registeredKinds[d.Kind] = true
	}

	// TODO : HYPERFLEET-1159 - Remove this once Cluster and NodePool are registered
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
	schemas["clusters"] = &ResourceSchema{
		TypeName: "ClusterSpec",
		Schema:   clusterSpecSchema,
	}
	schemas["nodepools"] = &ResourceSchema{
		TypeName: "NodePoolSpec",
		Schema:   nodePoolSpecSchema,
	}
	// for _, kind := range requiredSpecValidationKinds {
	// 	if !registeredKinds[kind] {
	// 		return nil, fmt.Errorf(
	// 			"entity kind %q with SpecSchemaName must be registered for schema validation",
	// 			kind,
	// 		)
	// 	}
	// }

	return schemas, nil
}

// TODO : HYPERFLEET-1159 - Uncomment this once Cluster and NodePool are registered
// func isRequiredSpecValidationKind(kind string) bool {
// 	for _, required := range requiredSpecValidationKinds {
// 		if kind == required {
// 			return true
// 		}
// 	}
// 	return false
// }

// HasSchema reports whether a validation schema was loaded for the given resource plural.
func (v *SchemaValidator) HasSchema(resourcePlural string) bool {
	return v.schemas[resourcePlural] != nil
}

// Validate validates a spec for the given resource plural (URL path segment).
// Returns nil when no schema is loaded for the plural (validation skipped).
func (v *SchemaValidator) Validate(resourcePlural string, spec map[string]interface{}) error {
	resourceSchema := v.schemas[resourcePlural]
	if resourceSchema == nil {
		return nil
	}

	return v.validateSpec(spec, resourceSchema.Schema, resourceSchema.TypeName)
}

// ValidateClusterSpec validates a cluster spec against the ClusterSpec schema
//
// Deprecated: Use Validate("clusters", spec) instead
func (v *SchemaValidator) ValidateClusterSpec(spec map[string]interface{}) error {
	return v.Validate("clusters", spec)
}

// ValidateNodePoolSpec validates a nodepool spec against the NodePoolSpec schema
//
// Deprecated: Use Validate("nodepools", spec) instead
func (v *SchemaValidator) ValidateNodePoolSpec(spec map[string]interface{}) error {
	return v.Validate("nodepools", spec)
}

// validateSpec performs the actual validation and converts errors to our error format
func (v *SchemaValidator) validateSpec(
	spec map[string]interface{}, schemaRef *openapi3.SchemaRef, specTypeName string,
) error {
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
			Field:   field,
			Message: e.Reason,
		})
	default:
		// Fallback for unknown error types
		// Error message already contains the full description
		details = append(details, errors.ValidationDetail{
			Field:   prefix,
			Message: err.Error(),
		})
	}

	return details
}
