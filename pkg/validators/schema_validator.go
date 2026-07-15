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
// Panics if any registered entity with RequireSpecSchema has a SpecSchemaName that does not resolve.
// Entities without RequireSpecSchema whose schema is absent are skipped with a warning.
func NewSchemaValidator(schemaPath string) (*SchemaValidator, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(schemaPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI schema from %s: %w", schemaPath, err)
	}

	if validateErr := doc.Validate(context.Background()); validateErr != nil {
		return nil, fmt.Errorf("invalid OpenAPI schema: %w", validateErr)
	}

	registry.ValidateSpecSchemas(func(name string) bool {
		return doc.Components.Schemas[name] != nil
	})

	schemas := buildSchemasMap(doc)

	return &SchemaValidator{
		doc:     doc,
		schemas: schemas,
	}, nil
}

func buildSchemasMap(doc *openapi3.T) map[string]*ResourceSchema {
	ctx := context.Background()
	schemas := make(map[string]*ResourceSchema)

	for _, d := range registry.WithSpecSchema() {
		schemaRef := doc.Components.Schemas[d.SpecSchemaName]
		if schemaRef == nil {
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
	}

	return schemas
}

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

// validateSpec performs the actual validation and converts errors to our error format
func (v *SchemaValidator) validateSpec(
	spec map[string]interface{}, schemaRef *openapi3.SchemaRef, specTypeName string,
) error {
	var specData interface{} = spec

	if err := schemaRef.Value.VisitJSON(specData); err != nil {
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
		for _, subErr := range e {
			subDetails := convertValidationError(subErr, prefix)
			details = append(details, subDetails...)
		}
	case *openapi3.SchemaError:
		field := prefix
		if len(e.JSONPointer()) > 0 {
			jsonPath := strings.Join(e.JSONPointer(), ".")
			if jsonPath != "" {
				field = prefix + "." + jsonPath
			}
		}
		details = append(details, errors.ValidationDetail{
			Field:   field,
			Message: e.Reason,
		})
	default:
		details = append(details, errors.ValidationDetail{
			Field:   prefix,
			Message: err.Error(),
		})
	}

	return details
}
