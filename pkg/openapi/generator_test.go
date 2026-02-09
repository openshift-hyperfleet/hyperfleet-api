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

package openapi

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/crd"
)

func TestGenerateSpec_EmptyRegistry(t *testing.T) {
	RegisterTestingT(t)

	registry := crd.NewRegistry()
	spec := GenerateSpec(registry)

	Expect(spec).ToNot(BeNil())
	Expect(spec.OpenAPI).To(Equal("3.0.0"))
	Expect(spec.Info.Title).To(Equal("HyperFleet API"))
	Expect(spec.Components.Schemas).ToNot(BeNil())
	Expect(spec.Components.Parameters).ToNot(BeNil())

	// Should have common schemas even with empty registry
	Expect(spec.Components.Schemas["Error"]).ToNot(BeNil())
	Expect(spec.Components.Schemas["ValidationError"]).ToNot(BeNil())
	Expect(spec.Components.Schemas["ResourceCondition"]).ToNot(BeNil())
	Expect(spec.Components.Schemas["AdapterStatus"]).ToNot(BeNil())

	// Should have common parameters
	Expect(spec.Components.Parameters["page"]).ToNot(BeNil())
	Expect(spec.Components.Parameters["pageSize"]).ToNot(BeNil())
	Expect(spec.Components.Parameters["search"]).ToNot(BeNil())
}

func TestGenerateSpec_WithRootResource(t *testing.T) {
	RegisterTestingT(t)

	registry := crd.NewRegistry()
	err := registry.Register(&api.ResourceDefinition{
		APIVersion: "hyperfleet.io/v1",
		Kind:       "Cluster",
		Plural:     "clusters",
		Singular:   "cluster",
		Scope:      api.ResourceScopeRoot,
		Enabled:    true,
	})
	Expect(err).To(BeNil())

	spec := GenerateSpec(registry)

	// Should have Cluster schemas
	Expect(spec.Components.Schemas["Cluster"]).ToNot(BeNil())
	Expect(spec.Components.Schemas["ClusterSpec"]).ToNot(BeNil())
	Expect(spec.Components.Schemas["ClusterStatus"]).ToNot(BeNil())
	Expect(spec.Components.Schemas["ClusterList"]).ToNot(BeNil())
	Expect(spec.Components.Schemas["ClusterCreateRequest"]).ToNot(BeNil())

	// Should have paths for root resource
	Expect(spec.Paths.Find("/api/hyperfleet/v1/clusters")).ToNot(BeNil())
	Expect(spec.Paths.Find("/api/hyperfleet/v1/clusters/{cluster_id}")).ToNot(BeNil())
	Expect(spec.Paths.Find("/api/hyperfleet/v1/clusters/{cluster_id}/statuses")).ToNot(BeNil())
}

func TestGenerateSpec_WithOwnedResource(t *testing.T) {
	RegisterTestingT(t)

	registry := crd.NewRegistry()

	// First register the owner resource
	err := registry.Register(&api.ResourceDefinition{
		APIVersion: "hyperfleet.io/v1",
		Kind:       "Cluster",
		Plural:     "clusters",
		Singular:   "cluster",
		Scope:      api.ResourceScopeRoot,
		Enabled:    true,
	})
	Expect(err).To(BeNil())

	// Then register the owned resource
	err = registry.Register(&api.ResourceDefinition{
		APIVersion: "hyperfleet.io/v1",
		Kind:       "NodePool",
		Plural:     "nodepools",
		Singular:   "nodepool",
		Scope:      api.ResourceScopeOwned,
		Owner: &api.OwnerRef{
			Kind:      "Cluster",
			PathParam: "cluster_id",
		},
		Enabled: true,
	})
	Expect(err).To(BeNil())

	spec := GenerateSpec(registry)

	// Should have NodePool schemas
	Expect(spec.Components.Schemas["NodePool"]).ToNot(BeNil())
	Expect(spec.Components.Schemas["NodePoolSpec"]).ToNot(BeNil())
	Expect(spec.Components.Schemas["NodePoolStatus"]).ToNot(BeNil())
	Expect(spec.Components.Schemas["NodePoolList"]).ToNot(BeNil())
	Expect(spec.Components.Schemas["NodePoolCreateRequest"]).ToNot(BeNil())

	// Should have paths for owned resource under owner
	Expect(spec.Paths.Find("/api/hyperfleet/v1/clusters/{cluster_id}/nodepools")).ToNot(BeNil())
	Expect(spec.Paths.Find("/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}")).ToNot(BeNil())

	// Should have global list path for owned resource
	Expect(spec.Paths.Find("/api/hyperfleet/v1/nodepools")).ToNot(BeNil())
}

func TestGenerateSpec_JSON(t *testing.T) {
	RegisterTestingT(t)

	registry := crd.NewRegistry()
	err := registry.Register(&api.ResourceDefinition{
		APIVersion: "hyperfleet.io/v1",
		Kind:       "Cluster",
		Plural:     "clusters",
		Singular:   "cluster",
		Scope:      api.ResourceScopeRoot,
		Enabled:    true,
	})
	Expect(err).To(BeNil())

	spec := GenerateSpec(registry)

	// Should be able to marshal to JSON
	data, err := spec.MarshalJSON()
	Expect(err).To(BeNil())
	Expect(data).ToNot(BeEmpty())
	Expect(string(data)).To(ContainSubstring(`"openapi":"3.0.0"`))
	Expect(string(data)).To(ContainSubstring(`"Cluster"`))
}
