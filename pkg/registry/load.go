package registry

import (
	"context"
	"fmt"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// LoadDescriptors registers entity types from application config.
// No-op when entities is nil or empty.
func LoadDescriptors(entities []EntityDescriptor) {
	if len(entities) == 0 {
		return
	}
	for _, d := range entities {
		Register(normalizeDescriptor(d))
	}
}

func normalizeDescriptor(d EntityDescriptor) EntityDescriptor {
	if d.ParentKind != "" && d.OnParentDelete == "" {
		d.OnParentDelete = OnParentDeleteRestrict
	}
	return d
}

// SearchDisallowedFieldsForKind returns a field blocklist map for TSL search, keyed by field name.
// Returns nil when the kind is unknown or has no disallowed fields configured.
func SearchDisallowedFieldsForKind(kind string) map[string]string {
	d, ok := Get(kind)
	if !ok || len(d.SearchDisallowedFields) == 0 {
		return nil
	}
	fields := make(map[string]string, len(d.SearchDisallowedFields))
	for _, f := range d.SearchDisallowedFields {
		fields[f] = f
	}
	return fields
}

// ValidateSchemas checks that every registered descriptor's SpecSchemaName resolves in the OpenAPI document.
// Empty SpecSchemaName is skipped. No-op when no descriptors declare a schema or schemaPath is empty.
// Panics with a descriptive message on failure.
func ValidateSchemas(schemaPath string) {
	registered := All()

	needsSchema := false
	for _, d := range registered {
		if d.SpecSchemaName != "" {
			needsSchema = true
			break
		}
	}
	if !needsSchema || schemaPath == "" {
		return
	}

	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(schemaPath)
	if err != nil {
		panic(fmt.Sprintf("failed to load OpenAPI schema from %s: %v", schemaPath, err))
	}
	if err := doc.Validate(context.Background()); err != nil {
		panic(fmt.Sprintf("invalid OpenAPI schema at %s: %v", schemaPath, err))
	}

	for _, d := range registered {
		if d.SpecSchemaName == "" {
			continue
		}
		if doc.Components.Schemas[d.SpecSchemaName] == nil {
			panic(fmt.Sprintf(
				`entity %q: spec_schema_name %q not found in OpenAPI spec at %s`,
				d.Kind, d.SpecSchemaName, schemaPath,
			))
		}
	}
}

// SchemaValidationKey returns the lookup key used by SchemaValidator for the given kind.
func SchemaValidationKey(kind string) string {
	return strings.ToLower(kind)
}
