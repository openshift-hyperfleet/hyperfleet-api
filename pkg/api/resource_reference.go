package api

// ResourceReference stores a single non-ownership association between two resources.
// Table: resource_references. Natural composite PK (source_id, ref_type, target_id).
type ResourceReference struct {
	SourceID   string `json:"source_id" gorm:"primaryKey;column:source_id;size:255"`
	RefType    string `json:"ref_type" gorm:"primaryKey;column:ref_type;size:255"`
	TargetID   string `json:"target_id" gorm:"primaryKey;column:target_id;size:255"`
	TargetKind string `json:"target_kind" gorm:"column:target_kind;size:100;not null"`
}

func (ResourceReference) TableName() string {
	return "resource_references"
}

// ResourceSummary carries kind + name for error messages (e.g. FindReferencers 409).
type ResourceSummary struct {
	Kind string
	Name string
}
