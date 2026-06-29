package api

import "time"

// ResourceConditionStatus represents the status of a resource condition (True/False only)
// Domain equivalent of openapi.ResourceConditionStatus
type ResourceConditionStatus string

const (
	ConditionTrue  ResourceConditionStatus = "True"  // String value matches openapi.TRUE
	ConditionFalse ResourceConditionStatus = "False" // String value matches openapi.FALSE
)

// AdapterConditionStatus represents the status of an adapter condition (includes Unknown)
// Domain equivalent of openapi.AdapterConditionStatus
type AdapterConditionStatus string

const (
	AdapterConditionTrue    AdapterConditionStatus = "True"
	AdapterConditionFalse   AdapterConditionStatus = "False"
	AdapterConditionUnknown AdapterConditionStatus = "Unknown"
)

// IsValid returns true if the status is one of the accepted enum values (True, False, Unknown).
func (s AdapterConditionStatus) IsValid() bool {
	return s == AdapterConditionTrue || s == AdapterConditionFalse || s == AdapterConditionUnknown
}

// Resource type constants
const (
	ResourceTypeCluster  = "Cluster"
	ResourceTypeNodePool = "NodePool"
)

// Adapter condition type constants (used in adapter.conditions reported by adapters)
const (
	AdapterConditionTypeAvailable  = "Available"
	AdapterConditionTypeApplied    = "Applied"
	AdapterConditionTypeHealth     = "Health"
	AdapterConditionTypeReconciled = "Reconciled"
	AdapterConditionTypeFinalized  = "Finalized"
)

// Resource condition type constants (used in resource.status.conditions aggregated from adapters)
const (
	ResourceConditionTypeAvailable           = "Available"
	ResourceConditionTypeHealth              = "Health"
	ResourceConditionTypeReconciled          = "Reconciled"
	ResourceConditionTypeFinalized           = "Finalized"
	ResourceConditionTypeLastKnownReconciled = "LastKnownReconciled"
)

// ResourceCondition represents a condition of a resource.
// Dual-use: GORM model for the resource_conditions table (generic resources)
// and JSON-deserializable struct for JSONB columns (clusters/node pools).
// ResourceID is excluded from JSON (json:"-") to preserve JSONB backward compat.
type ResourceCondition struct {
	CreatedTime        time.Time               `json:"created_time" gorm:"not null"`
	LastUpdatedTime    time.Time               `json:"last_updated_time" gorm:"not null"`
	LastTransitionTime time.Time               `json:"last_transition_time" gorm:"not null"`
	Reason             *string                 `json:"reason,omitempty" gorm:"type:text"`
	Message            *string                 `json:"message,omitempty" gorm:"type:text"`
	ResourceID         string                  `json:"-" gorm:"primaryKey;size:255;not null"`
	Type               string                  `json:"type" gorm:"primaryKey;size:100;not null"`
	Status             ResourceConditionStatus `json:"status" gorm:"size:10;not null"`
	ObservedGeneration int32                   `json:"observed_generation" gorm:"default:0;not null"`
}

func (ResourceCondition) TableName() string {
	return "resource_conditions"
}

// AdapterCondition represents a condition of an adapter
// Domain equivalent of openapi.AdapterCondition
// JSON tags match database JSONB structure
type AdapterCondition struct {
	LastTransitionTime time.Time              `json:"last_transition_time"`
	Reason             *string                `json:"reason,omitempty"`
	Message            *string                `json:"message,omitempty"`
	Type               string                 `json:"type"`
	Status             AdapterConditionStatus `json:"status"`
}
