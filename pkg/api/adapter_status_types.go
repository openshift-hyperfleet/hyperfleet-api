package api

import (
	"encoding/json"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// AdapterStatus database model
type AdapterStatus struct {
	Meta // Contains ID, CreatedAt, UpdatedAt, DeletedAt

	// Polymorphic association
	ResourceType string `json:"resource_type" gorm:"size:20;index:idx_resource;not null"`
	ResourceID   string `json:"resource_id" gorm:"size:255;index:idx_resource;not null"`

	// Adapter information
	Adapter            string `json:"adapter" gorm:"size:255;not null;uniqueIndex:idx_resource_adapter"`
	ObservedGeneration int32  `json:"observed_generation" gorm:"not null"`

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
	var conditions []openapi.Condition
	if len(as.Conditions) > 0 {
		_ = json.Unmarshal(as.Conditions, &conditions)
	}

	// Unmarshal Data
	var data map[string]interface{}
	if len(as.Data) > 0 {
		_ = json.Unmarshal(as.Data, &data)
	}

	// Unmarshal Metadata
	var metadata *openapi.AdapterStatusMetadata
	if len(as.Metadata) > 0 {
		_ = json.Unmarshal(as.Metadata, &metadata)
	}

	return &openapi.AdapterStatus{
		Adapter:            as.Adapter,
		ObservedGeneration: as.ObservedGeneration,
		Conditions:         conditions,
		Data:               data,
		Metadata:           metadata,
	}
}

// AdapterStatusFromOpenAPICreate creates GORM model from CreateRequest
func AdapterStatusFromOpenAPICreate(
	resourceType, resourceID string,
	req *openapi.AdapterStatusCreateRequest,
) *AdapterStatus {
	// Marshal Conditions
	conditionsJSON, _ := json.Marshal(req.Conditions)

	// Marshal Data
	data := make(map[string]interface{})
	if req.Data != nil {
		data = req.Data
	}
	dataJSON, _ := json.Marshal(data)

	// Marshal Metadata (if provided)
	var metadataJSON datatypes.JSON
	if req.Metadata != nil {
		metadataJSON, _ = json.Marshal(req.Metadata)
	}

	return &AdapterStatus{
		ResourceType:       resourceType,
		ResourceID:         resourceID,
		Adapter:            req.Adapter,
		ObservedGeneration: req.ObservedGeneration,
		Conditions:         conditionsJSON,
		Data:               dataJSON,
		Metadata:           metadataJSON,
	}
}
