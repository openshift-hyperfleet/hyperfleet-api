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

// Condition type constants
const (
	ConditionTypeAvailable           = "Available"
	ConditionTypeLastKnownReconciled = "LastKnownReconciled"
	ConditionTypeApplied             = "Applied"
	ConditionTypeHealth              = "Health"
	ConditionTypeReady               = "Ready"
	ConditionTypeReconciled          = "Reconciled"
	ConditionTypeFinalized           = "Finalized"
)

// ResourceCondition represents a condition of a resource
// Domain equivalent of openapi.ResourceCondition
// JSON tags match database JSONB structure
type ResourceCondition struct {
	CreatedTime        time.Time               `json:"created_time"`
	LastUpdatedTime    time.Time               `json:"last_updated_time"`
	LastTransitionTime time.Time               `json:"last_transition_time"`
	Reason             *string                 `json:"reason,omitempty"`
	Message            *string                 `json:"message,omitempty"`
	Type               string                  `json:"type"`
	Status             ResourceConditionStatus `json:"status"`
	ObservedGeneration int32                   `json:"observed_generation"`
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
