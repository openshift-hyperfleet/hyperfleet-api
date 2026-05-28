package registry

// OnParentDeletePolicy determines child behavior when its parent is deleted.
type OnParentDeletePolicy string

const (
	OnParentDeleteRestrict OnParentDeletePolicy = "restrict"
	OnParentDeleteCascade  OnParentDeletePolicy = "cascade"
)

// EntityDescriptor defines everything specific to a HyperFleet entity type.
// Descriptors are registered at startup via Register() in plugin init() functions.
type EntityDescriptor struct {
	Kind                   string               // discriminator value stored in Resource.Kind
	Plural                 string               // URL path segment, e.g. "channels"
	ParentKind             string               // "" for top-level entities
	SpecSchemaName         string               // OpenAPI component name for spec validation
	OnParentDelete         OnParentDeletePolicy // only meaningful when ParentKind != ""
	SearchDisallowedFields []string             // fields blocked from TSL search
}
