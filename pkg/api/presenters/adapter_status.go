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
			Status:             api.ConditionStatus(string(condReq.Status)),
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

	// Marshal Data
	data := make(map[string]map[string]interface{})
	if req.Data != nil {
		data = req.Data
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
func PresentAdapterStatus(adapterStatus *api.AdapterStatus) openapi.AdapterStatus {
	// Unmarshal Conditions
	var conditions []api.AdapterCondition
	if len(adapterStatus.Conditions) > 0 {
		_ = json.Unmarshal(adapterStatus.Conditions, &conditions)
	}

	// Convert domain AdapterConditions to openapi format
	openapiConditions := make([]openapi.AdapterCondition, len(conditions))
	for i, cond := range conditions {
		openapiConditions[i] = openapi.AdapterCondition{
			Type:               cond.Type,
			Status:             openapi.ConditionStatus(string(cond.Status)),
			Reason:             cond.Reason,
			Message:            cond.Message,
			LastTransitionTime: cond.LastTransitionTime,
		}
	}

	// Unmarshal Data
	var data map[string]map[string]interface{}
	if len(adapterStatus.Data) > 0 {
		_ = json.Unmarshal(adapterStatus.Data, &data)
	}

	// Unmarshal Metadata
	var metadata *openapi.AdapterStatusBaseMetadata
	if len(adapterStatus.Metadata) > 0 {
		_ = json.Unmarshal(adapterStatus.Metadata, &metadata)
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
		ObservedGeneration: adapterStatus.ObservedGeneration,
		Conditions:         openapiConditions,
		Data:               data,
		Metadata:           metadata,
		CreatedTime:        createdTime,
		LastReportTime:     lastReportTime,
	}
}
