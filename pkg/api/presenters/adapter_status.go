package presenters

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"gorm.io/datatypes"
)

// ConvertAdapterStatus converts openapi.AdapterStatusCreateRequest to api.AdapterStatus (GORM model)
func ConvertAdapterStatus(
	resourceType, resourceID string,
	req *openapi.AdapterStatusCreateRequest,
) (*api.AdapterStatus, error) {
	// Set timestamps
	// CreatedTime and LastReportTime should be set from req.ObservedTime
	now := time.Now()
	if !req.ObservedTime.IsZero() {
		now = req.ObservedTime
	}

	// Convert ConditionRequest to AdapterCondition (adding LastTransitionTime)
	adapterConditions := make([]api.AdapterCondition, len(req.Conditions))
	for i, condReq := range req.Conditions {
		adapterConditions[i] = api.AdapterCondition{
			Type:               condReq.Type,
			Status:             api.AdapterConditionStatus(condReq.Status),
			Reason:             condReq.Reason,
			Message:            condReq.Message,
			LastTransitionTime: now,
		}
	}

	// Marshal Conditions
	conditionsJSON, err := json.Marshal(adapterConditions)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal adapter conditions: %w", err)
	}

	// Marshal Data - req.Data is *map[string]interface{}
	data := make(map[string]interface{})
	if req.Data != nil {
		data = *req.Data
	}
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal adapter data: %w", err)
	}

	// Marshal Metadata (if provided)
	var metadataJSON datatypes.JSON
	if req.Metadata != nil {
		metadataJSON, err = json.Marshal(req.Metadata)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal adapter metadata: %w", err)
		}
	}

	return &api.AdapterStatus{
		ResourceType:       resourceType,
		ResourceID:         resourceID,
		Adapter:            req.Adapter,
		ObservedGeneration: req.ObservedGeneration,
		Conditions:         conditionsJSON,
		Data:               dataJSON,
		Metadata:           metadataJSON,
		CreatedTime:        &now,
		LastReportTime:     &now,
	}, nil
}

// PresentAdapterStatus converts api.AdapterStatus (GORM model) to openapi.AdapterStatus
func PresentAdapterStatus(adapterStatus *api.AdapterStatus) (openapi.AdapterStatus, error) {
	// Unmarshal Conditions
	var conditions []api.AdapterCondition
	if len(adapterStatus.Conditions) > 0 {
		if err := json.Unmarshal(adapterStatus.Conditions, &conditions); err != nil {
			return openapi.AdapterStatus{}, fmt.Errorf("failed to unmarshal adapter status conditions: %w", err)
		}
	}

	// Convert domain AdapterConditions to openapi format
	openapiConditions := make([]openapi.AdapterCondition, len(conditions))
	for i, cond := range conditions {
		openapiConditions[i] = openapi.AdapterCondition{
			Type:               cond.Type,
			Status:             openapi.AdapterConditionStatus(cond.Status),
			Reason:             cond.Reason,
			Message:            cond.Message,
			LastTransitionTime: cond.LastTransitionTime,
		}
	}

	// Unmarshal Data
	var data map[string]interface{}
	if len(adapterStatus.Data) > 0 {
		if err := json.Unmarshal(adapterStatus.Data, &data); err != nil {
			return openapi.AdapterStatus{}, fmt.Errorf("failed to unmarshal adapter status data: %w", err)
		}
	}

	// Unmarshal Metadata - inline struct type
	var metadata *api.AdapterStatusMetadata

	if len(adapterStatus.Metadata) > 0 {
		if err := json.Unmarshal(adapterStatus.Metadata, &metadata); err != nil {
			return openapi.AdapterStatus{}, fmt.Errorf("failed to unmarshal adapter status metadata: %w", err)
		}
	}

	var openapiMetadata *struct {
		Attempt       *int32     `json:"attempt,omitempty"`
		CompletedTime *time.Time `json:"completed_time,omitempty"`
		Duration      *string    `json:"duration,omitempty"`
		JobName       *string    `json:"job_name,omitempty"`
		JobNamespace  *string    `json:"job_namespace,omitempty"`
		StartedTime   *time.Time `json:"started_time,omitempty"`
	}

	if metadata != nil {
		openapiMetadata = &struct {
			Attempt       *int32     `json:"attempt,omitempty"`
			CompletedTime *time.Time `json:"completed_time,omitempty"`
			Duration      *string    `json:"duration,omitempty"`
			JobName       *string    `json:"job_name,omitempty"`
			JobNamespace  *string    `json:"job_namespace,omitempty"`
			StartedTime   *time.Time `json:"started_time,omitempty"`
		}{
			Attempt:       metadata.Attempt,
			CompletedTime: metadata.CompletedTime,
			Duration:      metadata.Duration,
			JobName:       metadata.JobName,
			JobNamespace:  metadata.JobNamespace,
			StartedTime:   metadata.StartedTime,
		}
	}

	// Set default times if nil (shouldn't happen in normal operation)
	createdTime := time.Time{}
	if adapterStatus.CreatedTime != nil {
		createdTime = *adapterStatus.CreatedTime
	}

	lastReportTime := time.Time{}
	if adapterStatus.LastReportTime != nil {
		lastReportTime = *adapterStatus.LastReportTime
	}

	return openapi.AdapterStatus{
		Adapter:            adapterStatus.Adapter,
		Conditions:         openapiConditions,
		CreatedTime:        createdTime,
		Data:               &data,
		LastReportTime:     lastReportTime,
		Metadata:           openapiMetadata,
		ObservedGeneration: adapterStatus.ObservedGeneration,
	}, nil
}
