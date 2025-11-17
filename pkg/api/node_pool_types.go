package api

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// NodePool database model
type NodePool struct {
	Meta // Contains ID, CreatedAt, UpdatedAt, DeletedAt

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
	StatusUpdatedAt          *time.Time     `json:"status_updated_at,omitempty"`
	StatusAdapters           datatypes.JSON `json:"status_adapters" gorm:"type:jsonb"`

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
	np.ID = NewID()
	if np.Kind == "" {
		np.Kind = "NodePool"
	}
	if np.OwnerKind == "" {
		np.OwnerKind = "Cluster"
	}
	if np.StatusPhase == "" {
		np.StatusPhase = "NotReady"
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

// ToOpenAPI converts to OpenAPI model
func (np *NodePool) ToOpenAPI() *openapi.NodePool {
	// Unmarshal Spec
	var spec map[string]interface{}
	if len(np.Spec) > 0 {
		_ = json.Unmarshal(np.Spec, &spec)
	}

	// Unmarshal Labels
	var labels map[string]string
	if len(np.Labels) > 0 {
		_ = json.Unmarshal(np.Labels, &labels)
	}

	// Unmarshal StatusAdapters
	var statusAdapters []openapi.ConditionAvailable
	if len(np.StatusAdapters) > 0 {
		_ = json.Unmarshal(np.StatusAdapters, &statusAdapters)
	}

	// Generate Href if not set (fallback)
	href := np.Href
	if href == "" {
		href = fmt.Sprintf("/api/hyperfleet/v1/clusters/%s/nodepools/%s", np.OwnerID, np.ID)
	}

	// Generate OwnerHref if not set (fallback)
	ownerHref := np.OwnerHref
	if ownerHref == "" {
		ownerHref = "/api/hyperfleet/v1/clusters/" + np.OwnerID
	}

	kind := np.Kind
	nodePool := &openapi.NodePool{
		Id:     &np.ID,
		Kind:   &kind,
		Href:   &href,
		Name:   np.Name,
		Spec:   spec,
		Labels: &labels,
		OwnerReferences: openapi.ObjectReference{
			Id:   &np.OwnerID,
			Kind: &np.OwnerKind,
			Href: &ownerHref,
		},
		CreatedAt: np.CreatedAt,
		UpdatedAt: np.UpdatedAt,
		CreatedBy: np.CreatedBy,
		UpdatedBy: np.UpdatedBy,
	}

	// Build NodePoolStatus
	nodePool.Status = openapi.NodePoolStatus{
		Phase:              np.StatusPhase,
		ObservedGeneration: np.StatusObservedGeneration,
		Adapters:           statusAdapters,
	}

	if np.StatusLastTransitionTime != nil {
		nodePool.Status.LastTransitionTime = *np.StatusLastTransitionTime
	}

	if np.StatusUpdatedAt != nil {
		nodePool.Status.UpdatedAt = *np.StatusUpdatedAt
	}

	return nodePool
}

// NodePoolFromOpenAPICreate creates GORM model from OpenAPI CreateRequest
func NodePoolFromOpenAPICreate(req *openapi.NodePoolCreateRequest, ownerID, createdBy string) *NodePool {
	// Marshal Spec
	specJSON, _ := json.Marshal(req.Spec)

	// Marshal Labels
	labels := make(map[string]string)
	if req.Labels != nil {
		labels = *req.Labels
	}
	labelsJSON, _ := json.Marshal(labels)

	// Marshal empty StatusAdapters
	statusAdaptersJSON, _ := json.Marshal([]openapi.ConditionAvailable{})

	kind := "NodePool"
	if req.Kind != nil {
		kind = *req.Kind
	}

	return &NodePool{
		Kind:                     kind,
		Name:                     req.Name,
		Spec:                     specJSON,
		Labels:                   labelsJSON,
		OwnerID:                  ownerID,
		OwnerKind:                "Cluster",
		StatusPhase:              "NotReady",
		StatusObservedGeneration: 0,
		StatusAdapters:           statusAdaptersJSON,
		CreatedBy:                createdBy,
		UpdatedBy:                createdBy,
	}
}

type NodePoolPatchRequest struct {
	Name   *string                 `json:"name,omitempty"`
	Spec   *map[string]interface{} `json:"spec,omitempty"`
	Labels *map[string]string      `json:"labels,omitempty"`
}
