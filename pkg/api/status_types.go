package api

import "time"

// ResourcePhase represents the lifecycle phase of a resource
// Domain equivalent of openapi.ResourcePhase
type ResourcePhase string

const (
	PhaseNotReady ResourcePhase = "NotReady" // String value matches openapi.NOT_READY
	PhaseReady    ResourcePhase = "Ready"    // String value matches openapi.READY
	PhaseFailed   ResourcePhase = "Failed"   // String value matches openapi.FAILED
)

// ConditionStatus represents the status of a condition
// Domain equivalent of openapi.ConditionStatus
type ConditionStatus string

const (
	ConditionTrue    ConditionStatus = "True"    // String value matches openapi.TRUE
	ConditionFalse   ConditionStatus = "False"   // String value matches openapi.FALSE
	ConditionUnknown ConditionStatus = "Unknown" // String value matches openapi.UNKNOWN
)

// ResourceCondition represents a condition of a resource
// Domain equivalent of openapi.ResourceCondition
// JSON tags match database JSONB structure
type ResourceCondition struct {
	ObservedGeneration int32           `json:"observed_generation"`
	CreatedTime        time.Time       `json:"created_time"`
	LastUpdatedTime    time.Time       `json:"last_updated_time"`
	Type               string          `json:"type"`
	Status             ConditionStatus `json:"status"`
	Reason             *string         `json:"reason,omitempty"`
	Message            *string         `json:"message,omitempty"`
	LastTransitionTime time.Time       `json:"last_transition_time"`
}

// AdapterCondition represents a condition of an adapter
// Domain equivalent of openapi.AdapterCondition
// JSON tags match database JSONB structure
type AdapterCondition struct {
	Type               string          `json:"type"`
	Status             ConditionStatus `json:"status"`
	Reason             *string         `json:"reason,omitempty"`
	Message            *string         `json:"message,omitempty"`
	LastTransitionTime time.Time       `json:"last_transition_time"`
}
