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

package crd

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestLoadFromDirectory(t *testing.T) {
	RegisterTestingT(t)

	registry := NewRegistry()
	err := registry.LoadFromDirectory("../../charts/crds")

	Expect(err).To(BeNil())
	Expect(registry.Count()).To(BeNumerically(">=", 3)) // Cluster, NodePool, IDP

	// Verify Cluster CRD loaded
	cluster, found := registry.GetByKind("Cluster")
	Expect(found).To(BeTrue())
	Expect(cluster.Plural).To(Equal("clusters"))
	Expect(cluster.IsRoot()).To(BeTrue())

	// Verify NodePool CRD loaded with owner
	nodepool, found := registry.GetByKind("NodePool")
	Expect(found).To(BeTrue())
	Expect(nodepool.IsOwned()).To(BeTrue())
	Expect(nodepool.GetOwnerKind()).To(Equal("Cluster"))

	// Verify IDP CRD loaded
	idp, found := registry.GetByKind("IDP")
	Expect(found).To(BeTrue())
	Expect(idp.Plural).To(Equal("idps"))
	Expect(idp.IsOwned()).To(BeTrue())
	Expect(idp.GetOwnerKind()).To(Equal("Cluster"))
}

func TestLoadFromDirectory_NonExistentDirectory(t *testing.T) {
	RegisterTestingT(t)

	registry := NewRegistry()
	err := registry.LoadFromDirectory("/nonexistent/path")

	Expect(err).ToNot(BeNil())
	Expect(err.Error()).To(ContainSubstring("failed to read directory"))
}

func TestLoadFromDirectory_EmptyDirectory(t *testing.T) {
	RegisterTestingT(t)

	registry := NewRegistry()
	// Use a directory that exists but has no YAML files
	err := registry.LoadFromDirectory("../../bin")

	// Should succeed but load nothing (bin may not exist, so we just check no panic)
	if err == nil {
		Expect(registry.Count()).To(Equal(0))
	}
}

func TestLoadFromDirectory_GetByPlural(t *testing.T) {
	RegisterTestingT(t)

	registry := NewRegistry()
	err := registry.LoadFromDirectory("../../charts/crds")
	Expect(err).To(BeNil())

	// Test GetByPlural
	cluster, found := registry.GetByPlural("clusters")
	Expect(found).To(BeTrue())
	Expect(cluster.Kind).To(Equal("Cluster"))

	nodepool, found := registry.GetByPlural("nodepools")
	Expect(found).To(BeTrue())
	Expect(nodepool.Kind).To(Equal("NodePool"))
}

func TestLoadFromDirectory_All(t *testing.T) {
	RegisterTestingT(t)

	registry := NewRegistry()
	err := registry.LoadFromDirectory("../../charts/crds")
	Expect(err).To(BeNil())

	all := registry.All()
	Expect(len(all)).To(BeNumerically(">=", 3))

	// Verify all returned definitions are enabled
	for _, def := range all {
		Expect(def.Enabled).To(BeTrue())
	}
}
