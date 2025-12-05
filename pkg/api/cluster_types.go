package api

import (
	"encoding/json"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Cluster database model
type Cluster struct {
	Meta // Contains ID, CreatedAt, UpdatedAt, DeletedAt

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
	c.ID = NewID()
	if c.Kind == "" {
		c.Kind = "Cluster"
	}
	if c.Generation == 0 {
		c.Generation = 1
	}
	if c.StatusPhase == "" {
		c.StatusPhase = "NotReady"
	}
	// Set Href if not already set
	if c.Href == "" {
		c.Href = "/api/hyperfleet/v1/clusters/" + c.ID
	}
	return nil
}

// ToOpenAPI converts to OpenAPI model
func (c *Cluster) ToOpenAPI() *openapi.Cluster {
	// Unmarshal Spec
	spec := make(map[string]interface{})
	if len(c.Spec) > 0 {
		_ = json.Unmarshal(c.Spec, &spec)
	}

	// Unmarshal Labels
	var labels map[string]string
	if len(c.Labels) > 0 {
		if err := json.Unmarshal(c.Labels, &labels); err != nil {
			labels = make(map[string]string)
		}
	}

	// Unmarshal StatusConditions (stored as ResourceCondition in DB)
	var statusConditions []openapi.ResourceCondition
	if len(c.StatusConditions) > 0 {
		if err := json.Unmarshal(c.StatusConditions, &statusConditions); err != nil {
			statusConditions = []openapi.ResourceCondition{}
		}
	}

	// Generate Href if not set (fallback)
	href := c.Href
	if href == "" {
		href = "/api/hyperfleet/v1/clusters/" + c.ID
	}

	cluster := &openapi.Cluster{
		Id:          &c.ID,
		Kind:        c.Kind,
		Href:        &href,
		Name:        c.Name,
		Spec:        spec,
		Labels:      &labels,
		Generation:  c.Generation,
		CreatedTime: c.CreatedTime,
		UpdatedTime: c.UpdatedTime,
		CreatedBy:   c.CreatedBy,
		UpdatedBy:   c.UpdatedBy,
	}

	// Build ClusterStatus - set required fields with defaults if nil
	lastTransitionTime := time.Time{}
	if c.StatusLastTransitionTime != nil {
		lastTransitionTime = *c.StatusLastTransitionTime
	}

	lastUpdatedTime := time.Time{}
	if c.StatusLastUpdatedTime != nil {
		lastUpdatedTime = *c.StatusLastUpdatedTime
	}

	cluster.Status = openapi.ClusterStatus{
		Phase:              c.StatusPhase,
		ObservedGeneration: c.StatusObservedGeneration,
		Conditions:         statusConditions,
		LastTransitionTime: lastTransitionTime,
		LastUpdatedTime:    lastUpdatedTime,
	}

	return cluster
}

// ClusterFromOpenAPICreate creates GORM model from OpenAPI CreateRequest
func ClusterFromOpenAPICreate(req *openapi.ClusterCreateRequest, createdBy string) *Cluster {
	// Marshal Spec
	specJSON, err := json.Marshal(req.Spec)
	if err != nil {
		specJSON = []byte("{}")
	}

	// Marshal Labels
	labels := make(map[string]string)
	if req.Labels != nil {
		labels = *req.Labels
	}
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		labelsJSON = []byte("{}")
	}

	// Marshal empty StatusConditions
	statusConditionsJSON, err := json.Marshal([]openapi.ResourceCondition{})
	if err != nil {
		statusConditionsJSON = []byte("[]")
	}

	return &Cluster{
		Kind:                     req.Kind,
		Name:                     req.Name,
		Spec:                     specJSON,
		Labels:                   labelsJSON,
		Generation:               1,
		StatusPhase:              "NotReady",
		StatusObservedGeneration: 0,
		StatusConditions:         statusConditionsJSON,
		CreatedBy:                createdBy,
		UpdatedBy:                createdBy,
	}
}

type ClusterPatchRequest struct {
	Name       *string                 `json:"name,omitempty"`
	Spec       *map[string]interface{} `json:"spec,omitempty"`
	Generation *int32                  `json:"generation,omitempty"`
	Labels     *map[string]string      `json:"labels,omitempty"`
}
