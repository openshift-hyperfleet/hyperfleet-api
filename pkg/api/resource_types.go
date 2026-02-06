/*
Copyright (c) 2018 Red Hat, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

  http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// This file contains the generic Resource model for the CRD-driven API.

package api

import (
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// Resource is a generic database model that can represent any resource type.
// The resource type is determined by the Kind field and CRD definitions.
type Resource struct {
	Meta // Contains ID, CreatedTime, UpdatedTime, DeletedAt

	// Core fields
	Kind   string         `json:"kind" gorm:"size:63;not null;index"`
	Name   string         `json:"name" gorm:"size:63;not null"`
	Spec   datatypes.JSON `json:"spec" gorm:"type:jsonb;not null"`
	Labels datatypes.JSON `json:"labels,omitempty" gorm:"type:jsonb"`
	Href   string         `json:"href,omitempty" gorm:"size:500"`

	// Version control
	Generation int32 `json:"generation" gorm:"default:1;not null"`

	// Owner references (for owned resources like NodePools under Clusters)
	OwnerID   *string `json:"owner_id,omitempty" gorm:"size:255;index"`
	OwnerKind *string `json:"owner_kind,omitempty" gorm:"size:63"`
	OwnerHref *string `json:"owner_href,omitempty" gorm:"size:500"`

	// Status (conditions-only model with synthetic Available/Ready conditions)
	StatusConditions datatypes.JSON `json:"status_conditions" gorm:"type:jsonb"`

	// Audit fields
	CreatedBy string `json:"created_by" gorm:"size:255;not null"`
	UpdatedBy string `json:"updated_by" gorm:"size:255;not null"`
}

// TableName specifies the database table name for GORM.
func (Resource) TableName() string {
	return "resources"
}

// ResourceList is a slice of Resource pointers.
type ResourceList []*Resource

// ResourceIndex maps resource IDs to Resource pointers.
type ResourceIndex map[string]*Resource

// Index creates a map of resources indexed by ID.
func (l ResourceList) Index() ResourceIndex {
	index := ResourceIndex{}
	for _, o := range l {
		index[o.ID] = o
	}
	return index
}

// BeforeCreate is a GORM hook that sets ID, timestamps, and defaults before insert.
func (r *Resource) BeforeCreate(tx *gorm.DB) error {
	now := time.Now()
	r.ID = NewID()
	r.CreatedTime = now
	r.UpdatedTime = now
	if r.Generation == 0 {
		r.Generation = 1
	}
	return nil
}

// BeforeUpdate is a GORM hook that updates the timestamp before update.
func (r *Resource) BeforeUpdate(tx *gorm.DB) error {
	r.UpdatedTime = time.Now()
	return nil
}

// IsRoot returns true if this resource has no owner.
func (r *Resource) IsRoot() bool {
	return r.OwnerID == nil || *r.OwnerID == ""
}

// IsOwned returns true if this resource has an owner.
func (r *Resource) IsOwned() bool {
	return r.OwnerID != nil && *r.OwnerID != ""
}

// ResourcePatchRequest represents a PATCH request for a generic resource.
type ResourcePatchRequest struct {
	Spec   *map[string]interface{} `json:"spec,omitempty"`
	Labels *map[string]string      `json:"labels,omitempty"`
}

// ResourceCreateRequest represents a POST request for creating a generic resource.
type ResourceCreateRequest struct {
	Kind   *string                `json:"kind,omitempty"`
	Name   string                 `json:"name"`
	Spec   map[string]interface{} `json:"spec"`
	Labels *map[string]string     `json:"labels,omitempty"`
}
