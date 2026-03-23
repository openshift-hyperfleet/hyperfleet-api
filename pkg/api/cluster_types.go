package api

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Cluster database model
type Cluster struct {
	Meta
	Kind             string         `json:"kind" gorm:"default:'Cluster'"`
	UUID             string         `json:"uuid" gorm:"uniqueIndex;size:36;not null"`
	Name             string         `json:"name" gorm:"uniqueIndex;size:53;not null"`
	Href             string         `json:"href,omitempty" gorm:"size:500"`
	CreatedBy        string         `json:"created_by" gorm:"size:255;not null"`
	UpdatedBy        string         `json:"updated_by" gorm:"size:255;not null"`
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
	now := time.Now()
	// Only generate if not already set (idempotent)
	if c.ID == "" {
		c.ID = NewID()
	}
	if c.UUID == "" {
		c.UUID = uuid.New().String()
	}
	c.CreatedTime = now
	c.UpdatedTime = now
	if c.Generation == 0 {
		c.Generation = 1
	}
	// Set Href if not already set
	if c.Href == "" {
		c.Href = fmt.Sprintf("/api/hyperfleet/v1/clusters/%s", c.ID)
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
