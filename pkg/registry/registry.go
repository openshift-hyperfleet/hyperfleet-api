package registry

import (
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

var descriptors = make(map[string]EntityDescriptor)

// Register adds a descriptor to the global registry. Panics on empty Kind or duplicate Kind.
func Register(d EntityDescriptor) {
	if d.Kind == "" {
		panic("entity kind cannot be empty")
	}
	if d.Plural == "" {
		panic(fmt.Sprintf("entity kind %q has empty plural", d.Kind))
	}
	if _, exists := descriptors[d.Kind]; exists {
		panic(fmt.Sprintf("entity kind %q already registered", d.Kind))
	}
	descriptors[d.Kind] = d
}

// LoadDescriptors registers entity descriptors loaded from the application config.
// Called during startup after config is parsed and before services/routes are built.
func LoadDescriptors(descriptors []EntityDescriptor) {
	for _, d := range descriptors {
		Register(d)
	}
}

// Get returns a descriptor by Kind, or (zero, false) if not found.
func Get(entityKind string) (EntityDescriptor, bool) {
	d, ok := descriptors[entityKind]
	return d, ok
}

// MustGet returns a descriptor by Kind. Panics if not found.
func MustGet(entityKind string) EntityDescriptor {
	d, ok := Get(entityKind)
	if !ok {
		panic(fmt.Sprintf("entity kind %q not registered", entityKind))
	}
	return d
}

// Validated returns a descriptor by Kind, or a 400 ServiceError if kind is unregistered.
// Shared by callers (services, handlers) that need to turn an unknown kind into a
// client-facing validation error rather than a panic or a bare bool.
func Validated(entityKind string) (EntityDescriptor, *errors.ServiceError) {
	d, ok := Get(entityKind)
	if !ok {
		return EntityDescriptor{}, errors.Validation("Unknown entity kind: %s", entityKind)
	}
	return d, nil
}

// All returns a snapshot of all registered descriptors.
func All() []EntityDescriptor {
	result := make([]EntityDescriptor, 0, len(descriptors))
	for _, d := range descriptors {
		result = append(result, d)
	}
	return result
}

// WithSpecSchema returns descriptors that declare an OpenAPI spec schema name.
func WithSpecSchema() []EntityDescriptor {
	var result []EntityDescriptor
	for _, d := range descriptors {
		if d.SpecSchemaName != "" {
			result = append(result, d)
		}
	}
	return result
}

// ChildrenOf returns descriptors whose ParentKind matches the given kind.
func ChildrenOf(parentKind string) []EntityDescriptor {
	var children []EntityDescriptor
	for _, d := range descriptors {
		if d.ParentKind == parentKind {
			children = append(children, d)
		}
	}
	return children
}

// Validate checks registry integrity. Panics on:
//   - empty Kind or Plural on any descriptor
//   - any ParentKind that references an unregistered kind
//   - duplicate Plural values across descriptors
//   - ReferenceDescriptor with TargetKind that doesn't resolve
//   - duplicate RefType within a single entity's References
//   - Max < Min (when Max > 0)
//   - circular required references (Min > 0 cycle between two or more kinds)
func Validate() {
	plurals := make(map[string]string, len(descriptors))

	for _, d := range descriptors {
		if d.Kind == "" {
			panic("entity kind cannot be empty")
		}
		if d.Plural == "" {
			panic(fmt.Sprintf("entity kind %q has empty plural", d.Kind))
		}

		if d.ParentKind != "" {
			if _, ok := descriptors[d.ParentKind]; !ok {
				panic(fmt.Sprintf(
					"entity kind %q references unregistered parent kind %q",
					d.Kind, d.ParentKind,
				))
			}
		}

		if existing, ok := plurals[d.Plural]; ok {
			panic(fmt.Sprintf(
				"duplicate plural %q: registered by both %q and %q",
				d.Plural, existing, d.Kind,
			))
		}
		plurals[d.Plural] = d.Kind

		// Track seen RefType values to detect duplicates within this entity's References
		refTypes := make(map[string]bool, len(d.References))
		for _, ref := range d.References {
			if _, ok := descriptors[ref.TargetKind]; !ok {
				panic(fmt.Sprintf(
					"entity %q: reference %q targets unregistered kind %q",
					d.Kind, ref.RefType, ref.TargetKind,
				))
			}
			if refTypes[ref.RefType] {
				panic(fmt.Sprintf(
					"entity %q: duplicate ref_type %q in references",
					d.Kind, ref.RefType,
				))
			}
			refTypes[ref.RefType] = true
			if ref.Max > 0 && ref.Max < ref.Min {
				panic(fmt.Sprintf(
					"entity %q: reference %q has max (%d) < min (%d)",
					d.Kind, ref.RefType, ref.Max, ref.Min,
				))
			}
		}
	}
}

// ValidateSpecSchemas checks descriptors that set RequireSpecSchema and panics if
// their SpecSchemaName is absent from the OpenAPI spec. Entities without
// RequireSpecSchema are left to buildSchemasMap, which warns and skips them.
// See also Validate, which checks registry structural integrity.
func ValidateSpecSchemas(schemaExists func(string) bool) {
	for _, d := range descriptors {
		if d.SpecSchemaName != "" && d.RequireSpecSchema && !schemaExists(d.SpecSchemaName) {
			panic(fmt.Sprintf(
				"entity kind %q declares SpecSchemaName %q but it does not resolve to an existing component in the OpenAPI spec",
				d.Kind, d.SpecSchemaName,
			))
		}
	}
}

// Reset clears all registrations. Only for use in tests.
func Reset() {
	descriptors = make(map[string]EntityDescriptor)
}

// UpdateDescriptor modifies an existing descriptor in-place. Panics if kind not found.
// Used by tests to temporarily override descriptor fields.
func UpdateDescriptor(kind string, updateFn func(*EntityDescriptor)) {
	desc, ok := descriptors[kind]
	if !ok {
		panic(fmt.Sprintf("entity kind %q not registered", kind))
	}
	updateFn(&desc)
	descriptors[kind] = desc
}
