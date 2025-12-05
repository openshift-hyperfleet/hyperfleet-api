package api

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Cluster database model
type Cluster struct {
	Meta // Contains ID, CreatedTime, UpdatedTime, DeletedTime

	// Core fields
	Kind   string         `json:"kind" gorm:"default:'Cluster'"`
	Name   string         `json:"name" gorm:"uniqueIndex;size:63;not null"`
	Spec   datatypes.JSON `json:"spec" gorm:"type:jsonb;not null"`
	Labels datatypes.JSON `json:"labels,omitempty" gorm:"type:jsonb"`
	Href   string         `json:"href,omitempty" gorm:"size:500"`

	// Version control
	Generation int32 `json:"generation" gorm:"default:1;not null"`

	// Status fields (expanded to database columns)
	StatusPhase              string         `json:"status_phase" gorm:"default:'NotReady'"`
	StatusLastTransitionTime *time.Time     `json:"status_last_transition_time,omitempty"`
	StatusObservedGeneration int32          `json:"status_observed_generation" gorm:"default:0"`
	StatusLastUpdatedTime    *time.Time     `json:"status_last_updated_time,omitempty"`
	StatusConditions         datatypes.JSON `json:"status_conditions" gorm:"type:jsonb"`

	// Audit fields
	CreatedBy string `json:"created_by" gorm:"size:255;not null"`
	UpdatedBy string `json:"updated_by" gorm:"size:255;not null"`
}

type ClusterList []*Cluster
type ClusterIndex map[string]*Cluster

func (l ClusterList) Index() ClusterIndex {
	index := ClusterIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

func (c *Cluster) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	c.ID = NewID()
	c.CreatedTime = now
	c.UpdatedTime = now
	if c.Kind == "" {
		c.Kind = "Cluster"
	}
	if c.Generation == 0 {
		c.Generation = 1
	}
	if c.StatusPhase == "" {
		c.StatusPhase = string(PhaseNotReady)
	}
	// Set Href if not already set
	if c.Href == "" {
		c.Href = "/api/hyperfleet/v1/clusters/" + c.ID
	}
	return nil
}

func (c *Cluster) BeforeUpdate(tx *gorm.DB) error {
	c.UpdatedTime = time.Now()
	return nil
}

type ClusterPatchRequest struct {
	Name       *string                 `json:"name,omitempty"`
	Spec       *map[string]interface{} `json:"spec,omitempty"`
	Generation *int32                  `json:"generation,omitempty"`
	Labels     *map[string]string      `json:"labels,omitempty"`
}
