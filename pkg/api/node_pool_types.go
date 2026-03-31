package api

import (
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// NodePool database model
type NodePool struct {
	Cluster *Cluster `gorm:"foreignKey:OwnerID;references:ID"`
	Meta
	Kind             string         `json:"kind" gorm:"default:'NodePool'"`
	Name             string         `json:"name" gorm:"size:15;not null"`
	UpdatedBy        string         `json:"updated_by" gorm:"size:255;not null"`
	Href             string         `json:"href,omitempty" gorm:"size:500"`
	CreatedBy        string         `json:"created_by" gorm:"size:255;not null"`
	OwnerID          string         `json:"owner_id" gorm:"size:255;not null;index"`
	OwnerKind        string         `json:"owner_kind" gorm:"size:50;not null"`
	OwnerHref        string         `json:"owner_href,omitempty" gorm:"size:500"`
	Spec             datatypes.JSON `json:"spec" gorm:"type:jsonb;not null"`
	StatusConditions datatypes.JSON `json:"status_conditions" gorm:"type:jsonb"`
	Labels           datatypes.JSON `json:"labels,omitempty" gorm:"type:jsonb"`
	Generation       int32          `json:"generation" gorm:"default:1;not null"`
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
	id, err := NewID()
	if err != nil {
		return fmt.Errorf("failed to generate node pool ID: %w", err)
	}
	np.ID = id

	now := time.Now()
	np.CreatedTime = now
	np.UpdatedTime = now
	if np.Generation == 0 {
		np.Generation = 1
	}
	if np.OwnerKind == "" {
		np.OwnerKind = "Cluster"
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
	Spec   *map[string]interface{} `json:"spec,omitempty"`
	Labels *map[string]string      `json:"labels,omitempty"`
}
