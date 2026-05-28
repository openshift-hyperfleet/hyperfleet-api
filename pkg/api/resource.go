package api

import (
	"fmt"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
)

// Resource is the generic GORM model for entity types managed by the entity
// registry (Channel, Version, WIF Config, etc.). Entity kinds are
// differentiated by the Kind field. Existing Cluster and NodePool types
// are NOT migrated to this model.
type Resource struct {
	Meta
	Kind        string         `json:"kind" gorm:"size:100;not null"`
	Name        string         `json:"name" gorm:"size:100;not null"`
	Href        string         `json:"href,omitempty" gorm:"size:500"`
	CreatedBy   string         `json:"created_by" gorm:"size:255;not null"`
	UpdatedBy   string         `json:"updated_by" gorm:"size:255;not null"`
	DeletedBy   *string        `json:"deleted_by,omitempty" gorm:"size:255"`
	DeletedTime *time.Time     `json:"deleted_time,omitempty"`
	OwnerID     *string        `json:"owner_id,omitempty" gorm:"size:255"`
	OwnerKind   *string        `json:"owner_kind,omitempty" gorm:"size:100"`
	OwnerHref   *string        `json:"owner_href,omitempty" gorm:"size:500"`
	Spec        datatypes.JSON `json:"spec" gorm:"type:jsonb;not null"`
	Labels      datatypes.JSON `json:"labels,omitempty" gorm:"type:jsonb"`
	Generation  int32          `json:"generation" gorm:"default:1;not null"`
}

type ResourcePatchRequest struct {
	Spec   *map[string]interface{} `json:"spec,omitempty"`
	Labels *map[string]string      `json:"labels,omitempty"`
}

type ResourceList []*Resource

func (r Resource) TableName() string {
	return "resources"
}

func (r *Resource) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		id, err := NewID()
		if err != nil {
			return fmt.Errorf("failed to generate resource ID: %w", err)
		}
		r.ID = id
	}

	now := time.Now()
	if r.CreatedTime.IsZero() {
		r.CreatedTime = now
	}
	r.UpdatedTime = now
	if r.Generation == 0 {
		r.Generation = 1
	}

	if r.Href == "" {
		desc := registry.MustGet(r.Kind)
		if r.OwnerID != nil && *r.OwnerID != "" {
			if r.OwnerKind == nil || *r.OwnerKind == "" {
				return fmt.Errorf("owner_kind is required when owner_id is set")
			}
			if r.OwnerHref == nil {
				parentDesc := registry.MustGet(*r.OwnerKind)
				ownerHref := fmt.Sprintf("/api/hyperfleet/v1/%s/%s",
					parentDesc.Plural, *r.OwnerID)
				r.OwnerHref = &ownerHref
			}
			r.Href = fmt.Sprintf("%s/%s/%s", *r.OwnerHref, desc.Plural, r.ID)
		} else {
			r.Href = fmt.Sprintf("/api/hyperfleet/v1/%s/%s", desc.Plural, r.ID)
		}
	}

	return nil
}

func (r *Resource) BeforeUpdate(tx *gorm.DB) error {
	r.UpdatedTime = time.Now()
	return nil
}

func (r *Resource) MarkDeleted(by string, t time.Time) {
	r.DeletedTime = &t
	r.DeletedBy = &by
}

func (r *Resource) SetOwner(id, kind, href string) {
	r.OwnerID = &id
	r.OwnerKind = &kind
	r.OwnerHref = &href
}

func (r *Resource) IncrementGeneration() {
	r.Generation++
}
