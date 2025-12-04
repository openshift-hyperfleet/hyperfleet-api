package api

import (
	"encoding/json"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// AdapterStatus database model
type AdapterStatus struct {
	Meta // Contains ID, CreatedTime, UpdatedTime, DeletedAt

	// Polymorphic association
	ResourceType string `json:"resource_type" gorm:"size:20;index:idx_resource;not null"`
	ResourceID   string `json:"resource_id" gorm:"size:255;index:idx_resource;not null"`

	// Adapter information
	Adapter            string `json:"adapter" gorm:"size:255;not null;uniqueIndex:idx_resource_adapter"`
	ObservedGeneration int32  `json:"observed_generation" gorm:"not null"`

	// API-managed timestamps
	LastReportTime *time.Time `json:"last_report_time" gorm:"not null"` // Updated on every POST
	CreatedTime    *time.Time `json:"created_time" gorm:"not null"`     // Set on first creation

	// Stored as JSON
	Conditions datatypes.JSON `json:"conditions" gorm:"type:jsonb;not null"`
	Data       datatypes.JSON `json:"data,omitempty" gorm:"type:jsonb"`
	Metadata   datatypes.JSON `json:"metadata,omitempty" gorm:"type:jsonb"`
}

type AdapterStatusList []*AdapterStatus
type AdapterStatusIndex map[string]*AdapterStatus

func (l AdapterStatusList) Index() AdapterStatusIndex {
	index := AdapterStatusIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

func (as *AdapterStatus) BeforeCreate(tx *gorm.DB) error {
	as.ID = NewID()
	return nil
}

// ToOpenAPI converts to OpenAPI model
func (as *AdapterStatus) ToOpenAPI() *openapi.AdapterStatus {
	// Unmarshal Conditions
	var conditions []openapi.AdapterCondition
	if len(as.Conditions) > 0 {
		if err := json.Unmarshal(as.Conditions, &conditions); err != nil {
			// If unmarshal fails, use empty slice
			conditions = []openapi.AdapterCondition{}
		}
	}

	// Unmarshal Data
	var data map[string]map[string]interface{}
	if len(as.Data) > 0 {
		if err := json.Unmarshal(as.Data, &data); err != nil {
			// If unmarshal fails, use empty map
			data = make(map[string]map[string]interface{})
		}
	}

	// Unmarshal Metadata
	var metadata *openapi.AdapterStatusBaseMetadata
	if len(as.Metadata) > 0 {
		_ = json.Unmarshal(as.Metadata, &metadata)
	}

	// Set default times if nil (shouldn't happen in normal operation)
	createdTime := time.Time{}
	if as.CreatedTime != nil {
		createdTime = *as.CreatedTime
	}

	lastReportTime := time.Time{}
	if as.LastReportTime != nil {
		lastReportTime = *as.LastReportTime
	}

	return &openapi.AdapterStatus{
		Adapter:            as.Adapter,
		ObservedGeneration: as.ObservedGeneration,
		Conditions:         conditions,
		Data:               data,
		Metadata:           metadata,
		CreatedTime:        createdTime,
		LastReportTime:     lastReportTime,
	}
}

// AdapterStatusFromOpenAPICreate creates GORM model from CreateRequest
func AdapterStatusFromOpenAPICreate(
	resourceType, resourceID string,
	req *openapi.AdapterStatusCreateRequest,
) *AdapterStatus {
	// Set timestamps
	// CreatedTime and LastReportTime should be set from req.ObservedTime
	now := time.Now()
	if !req.ObservedTime.IsZero() {
		now = req.ObservedTime
	}

	// Convert ConditionRequest to AdapterCondition (adding LastTransitionTime)
	adapterConditions := make([]openapi.AdapterCondition, len(req.Conditions))
	for i, condReq := range req.Conditions {
		adapterConditions[i] = openapi.AdapterCondition{
			Type:               condReq.Type,
			Status:             condReq.Status,
			Reason:             condReq.Reason,
			Message:            condReq.Message,
			LastTransitionTime: now,
		}
	}

	// Marshal Conditions - if this fails, it's a programming error
	conditionsJSON, err := json.Marshal(adapterConditions)
	if err != nil {
		// Fallback to empty array JSON
		// Log marshal failure - this indicates a programming error
		// logger.Errorf("Failed to marshal adapter conditions: %v", err)
		conditionsJSON = []byte("[]")
	}

	dataJSON := []byte("{}") // default fallback

	if req.Data != nil {
		if b, err := json.Marshal(req.Data); err == nil {
			dataJSON = b
		}
	}

	// Marshal Metadata (if provided)
	metadataJSON := []byte("{}") // safe fallback

	if req.Metadata != nil {
		if b, err := json.Marshal(req.Metadata); err == nil {
			metadataJSON = b
		}
	}

	return &AdapterStatus{
		ResourceType:       resourceType,
		ResourceID:         resourceID,
		Adapter:            req.Adapter,
		ObservedGeneration: req.ObservedGeneration,
		Conditions:         conditionsJSON,
		Data:               dataJSON,
		Metadata:           metadataJSON,
		CreatedTime:        &now,
		LastReportTime:     &now,
	}
}
