package registry

// OnParentDeletePolicy determines child behavior when its parent is deleted.
type OnParentDeletePolicy string

const (
	OnParentDeleteRestrict OnParentDeletePolicy = "restrict"
	OnParentDeleteCascade  OnParentDeletePolicy = "cascade"
)

// EntityDescriptor defines everything specific to a HyperFleet entity type.
// Descriptors are loaded from application config YAML via LoadDescriptors().
type EntityDescriptor struct {
	Kind                   string               `mapstructure:"kind" validate:"required"`
	Plural                 string               `mapstructure:"plural" validate:"required"`
	ParentKind             string               `mapstructure:"parent_kind"`
	SpecSchemaName         string               `mapstructure:"spec_schema_name"`
	OnParentDelete         OnParentDeletePolicy `mapstructure:"on_parent_delete"`
	SearchDisallowedFields []string             `mapstructure:"search_disallowed_fields"`
}
