package api

import (
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Cluster database model
type Cluster struct {
	Meta
	Kind             string         `json:"kind" gorm:"default:'Cluster'"`
	Name             string         `json:"name" gorm:"uniqueIndex;size:53;not null"`
	Href             string         `json:"href,omitempty" gorm:"size:500"`
	CreatedBy        string         `json:"created_by" gorm:"size:255;not null"`
	UpdatedBy        string         `json:"updated_by" gorm:"size:255;not null"`
	DeletedBy        *string        `json:"deleted_by,omitempty" gorm:"size:255"`
	DeletedTime      *time.Time     `json:"deleted_time,omitempty"`
	Spec             datatypes.JSON `json:"spec" gorm:"type:jsonb;not null"`
	Labels           datatypes.JSON `json:"labels,omitempty" gorm:"type:jsonb"`
	StatusConditions datatypes.JSON `json:"status_conditions" gorm:"type:jsonb"`
	Generation       int32          `json:"generation" gorm:"default:1;not null"`
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
	id, err := NewID()
	if err != nil {
		return fmt.Errorf("failed to generate cluster ID: %w", err)
	}
	c.ID = id

	now := time.Now()
	c.CreatedTime = now
	c.UpdatedTime = now
	if c.Generation == 0 {
		c.Generation = 1
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
	Spec   *map[string]interface{} `json:"spec,omitempty"`
	Labels *map[string]string      `json:"labels,omitempty"`
}
