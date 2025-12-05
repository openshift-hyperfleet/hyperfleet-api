package api

import (
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// NodePool database model
type NodePool struct {
	Meta // Contains ID, CreatedTime, UpdatedTime, DeletedAt

	// Core fields
	Kind   string         `json:"kind" gorm:"default:'NodePool'"`
	Name   string         `json:"name" gorm:"size:255;not null"`
	Spec   datatypes.JSON `json:"spec" gorm:"type:jsonb;not null"`
	Labels datatypes.JSON `json:"labels,omitempty" gorm:"type:jsonb"`
	Href   string         `json:"href,omitempty" gorm:"size:500"`

	// Owner references (expanded)
	OwnerID   string `json:"owner_id" gorm:"size:255;not null;index"`
	OwnerKind string `json:"owner_kind" gorm:"size:50;not null"`
	OwnerHref string `json:"owner_href,omitempty" gorm:"size:500"`

	// Foreign key relationship
	Cluster *Cluster `gorm:"foreignKey:OwnerID;references:ID"`

	// Status fields (expanded)
	StatusPhase              string         `json:"status_phase" gorm:"default:'NotReady'"`
	StatusObservedGeneration int32          `json:"status_observed_generation" gorm:"default:0"`
	StatusLastTransitionTime *time.Time     `json:"status_last_transition_time,omitempty"`
	StatusLastUpdatedTime    *time.Time     `json:"status_last_updated_time,omitempty"`
	StatusConditions         datatypes.JSON `json:"status_conditions" gorm:"type:jsonb"`

	// Audit fields
	CreatedBy string `json:"created_by" gorm:"size:255;not null"`
	UpdatedBy string `json:"updated_by" gorm:"size:255;not null"`
}

type NodePoolList []*NodePool
type NodePoolIndex map[string]*NodePool

func (l NodePoolList) Index() NodePoolIndex {
	index := NodePoolIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

func (np *NodePool) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	np.ID = NewID()
	np.CreatedTime = now
	np.UpdatedTime = now
	if np.Kind == "" {
		np.Kind = "NodePool"
	}
	if np.OwnerKind == "" {
		np.OwnerKind = "Cluster"
	}
	if np.StatusPhase == "" {
		np.StatusPhase = string(PhaseNotReady)
	}
	// Set Href if not already set
	if np.Href == "" {
		np.Href = fmt.Sprintf("/api/hyperfleet/v1/clusters/%s/nodepools/%s", np.OwnerID, np.ID)
	}
	// Set OwnerHref if not already set
	if np.OwnerHref == "" {
		np.OwnerHref = "/api/hyperfleet/v1/clusters/" + np.OwnerID
	}
	return nil
}

func (np *NodePool) BeforeUpdate(tx *gorm.DB) error {
	np.UpdatedTime = time.Now()
	return nil
}

type NodePoolPatchRequest struct {
	Name   *string                 `json:"name,omitempty"`
	Spec   *map[string]interface{} `json:"spec,omitempty"`
	Labels *map[string]string      `json:"labels,omitempty"`
}
