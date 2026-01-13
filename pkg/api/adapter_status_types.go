package api

import (
	"time"

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

type AdapterStatusMetadata struct {
	Attempt       *int32     `json:"attempt,omitempty"`
	CompletedTime *time.Time `json:"completed_time,omitempty"`
	Duration      *string    `json:"duration,omitempty"`
	JobName       *string    `json:"job_name,omitempty"`
	JobNamespace  *string    `json:"job_namespace,omitempty"`
	StartedTime   *time.Time `json:"started_time,omitempty"`
}

type (
	AdapterStatusList  []*AdapterStatus
	AdapterStatusIndex map[string]*AdapterStatus
)

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
