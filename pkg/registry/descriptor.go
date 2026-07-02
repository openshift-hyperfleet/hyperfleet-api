package registry

// OnParentDeletePolicy determines child behavior when its parent is deleted.
type OnParentDeletePolicy string

const (
	OnParentDeleteRestrict OnParentDeletePolicy = "restrict"
	OnParentDeleteCascade  OnParentDeletePolicy = "cascade"
)

// ReferenceDescriptor declares a non-ownership association from one entity type to another.
// See HYPERFLEET-1156 for the full resource references implementation.
type ReferenceDescriptor struct {
	// key in the references map on the Resource API type, e.g. "wif_config"
	RefType string `mapstructure:"ref_type" json:"ref_type"`
	// Kind of the referenced entity, e.g. "WifConfig"
	TargetKind string `mapstructure:"target_kind" json:"target_kind"`
	// minimum references of this type (0 = optional)
	Min int `mapstructure:"min" json:"min,omitempty"`
	// maximum references of this type (0 = unlimited)
	Max int `mapstructure:"max" json:"max,omitempty"`
}

// EntityDescriptor defines everything specific to a HyperFleet entity type.
// Descriptors are loaded from the application config YAML at startup via LoadDescriptors.
// Cluster and NodePool use legacy typed plugins and are not registered here.
type EntityDescriptor struct {
	// discriminator value stored in Resource.Kind
	Kind string `mapstructure:"kind" json:"kind"`
	// URL path segment, e.g. "channels"
	Plural string `mapstructure:"plural" json:"plural"`
	// "" for top-level entities
	ParentKind string `mapstructure:"parent_kind" json:"parent_kind,omitempty"`
	// OpenAPI component name for spec validation
	SpecSchemaName string `mapstructure:"spec_schema_name" json:"spec_schema_name,omitempty"`
	// only meaningful when ParentKind != ""
	OnParentDelete OnParentDeletePolicy `mapstructure:"on_parent_delete" json:"on_parent_delete,omitempty"`
	// adapters that must finalize before hard-delete
	RequiredAdapters []string `mapstructure:"required_adapters" json:"required_adapters,omitempty"`
	// fields blocked from TSL search
	SearchDisallowedFields []string `mapstructure:"search_disallowed_fields" json:"search_disallowed_fields,omitempty"` //nolint:lll
	// non-ownership associations to other entity types (HYPERFLEET-1156)
	References []ReferenceDescriptor `mapstructure:"references" json:"references,omitempty"`
	// panic at startup if SpecSchemaName missing from spec
	RequireSpecSchema bool `mapstructure:"require_spec_schema" json:"require_spec_schema,omitempty"`
}
