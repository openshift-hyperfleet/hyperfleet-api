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

// This file contains the CRD (Custom Resource Definition) types for the generic resource API.

package api

// ResourceScope defines whether a resource is root-level or owned by another resource.
type ResourceScope string

const (
	// ResourceScopeRoot indicates a top-level resource with no owner.
	ResourceScopeRoot ResourceScope = "Root"
	// ResourceScopeOwned indicates a resource that belongs to another resource.
	ResourceScopeOwned ResourceScope = "Owned"
)

// OwnerRef defines the parent resource for owned resources.
type OwnerRef struct {
	// Kind is the kind of the owner resource (e.g., "Cluster").
	Kind string `yaml:"kind" json:"kind"`
	// PathParam is the URL path parameter name for the owner ID (e.g., "cluster_id").
	PathParam string `yaml:"pathParam" json:"pathParam"`
}

// StatusConfig defines the status aggregation configuration for a resource.
type StatusConfig struct {
	// RequiredAdapters is the list of adapter names required for this resource type.
	RequiredAdapters []string `yaml:"requiredAdapters" json:"requiredAdapters"`
}

// ResourceDefinition defines a custom resource type (CRD).
// It specifies the resource's identity, scope, ownership, and status configuration.
type ResourceDefinition struct {
	// APIVersion is the API version (e.g., "hyperfleet.io/v1").
	APIVersion string `yaml:"apiVersion" json:"apiVersion"`
	// Kind is the resource type name (e.g., "Cluster", "NodePool").
	Kind string `yaml:"kind" json:"kind"`
	// Plural is the plural form for API paths (e.g., "clusters", "nodepools").
	Plural string `yaml:"plural" json:"plural"`
	// Singular is the singular form (e.g., "cluster", "nodepool").
	Singular string `yaml:"singular" json:"singular"`
	// Scope indicates whether this is a Root or Owned resource.
	Scope ResourceScope `yaml:"scope" json:"scope"`
	// Owner defines the parent resource for Owned scope resources.
	Owner *OwnerRef `yaml:"owner,omitempty" json:"owner,omitempty"`
	// StatusConfig defines the status aggregation settings.
	StatusConfig StatusConfig `yaml:"statusConfig" json:"statusConfig"`
	// Enabled indicates whether this resource type is active.
	Enabled bool `yaml:"enabled" json:"enabled"`
}

// IsRoot returns true if this is a root-level resource.
func (rd *ResourceDefinition) IsRoot() bool {
	return rd.Scope == ResourceScopeRoot
}

// IsOwned returns true if this resource has an owner.
func (rd *ResourceDefinition) IsOwned() bool {
	return rd.Scope == ResourceScopeOwned && rd.Owner != nil
}

// GetOwnerKind returns the owner's kind, or empty string if not owned.
func (rd *ResourceDefinition) GetOwnerKind() string {
	if rd.Owner == nil {
		return ""
	}
	return rd.Owner.Kind
}

// GetOwnerPathParam returns the URL path parameter for the owner ID.
func (rd *ResourceDefinition) GetOwnerPathParam() string {
	if rd.Owner == nil {
		return ""
	}
	return rd.Owner.PathParam
}
