package api

import (
	"testing"

	. "github.com/onsi/gomega"
)

// TestNodePoolList_Index tests the Index() method for NodePoolList
func TestNodePoolList_Index(t *testing.T) {
	RegisterTestingT(t)

	// Test empty list
	emptyList := NodePoolList{}
	emptyIndex := emptyList.Index()
	Expect(len(emptyIndex)).To(Equal(0))

	// Test single nodepool
	nodepool1 := &NodePool{}
	nodepool1.ID = "nodepool-1"
	nodepool1.Name = "test-nodepool-1"

	singleList := NodePoolList{nodepool1}
	singleIndex := singleList.Index()
	Expect(len(singleIndex)).To(Equal(1))
	Expect(singleIndex["nodepool-1"]).To(Equal(nodepool1))

	// Test multiple nodepools
	nodepool2 := &NodePool{}
	nodepool2.ID = "nodepool-2"
	nodepool2.Name = "test-nodepool-2"

	nodepool3 := &NodePool{}
	nodepool3.ID = "nodepool-3"
	nodepool3.Name = "test-nodepool-3"

	multiList := NodePoolList{nodepool1, nodepool2, nodepool3}
	multiIndex := multiList.Index()
	Expect(len(multiIndex)).To(Equal(3))
	Expect(multiIndex["nodepool-1"]).To(Equal(nodepool1))
	Expect(multiIndex["nodepool-2"]).To(Equal(nodepool2))
	Expect(multiIndex["nodepool-3"]).To(Equal(nodepool3))

	// Test duplicate IDs (later one overwrites earlier one)
	nodepool1Duplicate := &NodePool{}
	nodepool1Duplicate.ID = "nodepool-1"
	nodepool1Duplicate.Name = "duplicate-nodepool"

	duplicateList := NodePoolList{nodepool1, nodepool1Duplicate}
	duplicateIndex := duplicateList.Index()
	Expect(len(duplicateIndex)).To(Equal(1))
	Expect(duplicateIndex["nodepool-1"]).To(Equal(nodepool1Duplicate))
	Expect(duplicateIndex["nodepool-1"].Name).To(Equal("duplicate-nodepool"))
}

// TestNodePool_BeforeCreate_IDGeneration tests ID auto-generation
func TestNodePool_BeforeCreate_IDGeneration(t *testing.T) {
	RegisterTestingT(t)

	nodepool := &NodePool{
		Name:    "test-nodepool",
		OwnerID: "cluster-123",
	}

	err := nodepool.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(nodepool.ID).ToNot(BeEmpty())
	Expect(len(nodepool.ID)).To(BeNumerically(">", 0))
}

// TestNodePool_BeforeCreate_KindDefault tests Kind default value
func TestNodePool_BeforeCreate_KindDefault(t *testing.T) {
	RegisterTestingT(t)

	// Test default Kind
	nodepool := &NodePool{
		Name:    "test-nodepool",
		OwnerID: "cluster-123",
	}

	err := nodepool.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(nodepool.Kind).To(Equal("NodePool"))
}

// TestNodePool_BeforeCreate_KindPreserved tests Kind is not overwritten
func TestNodePool_BeforeCreate_KindPreserved(t *testing.T) {
	RegisterTestingT(t)

	// Test Kind preservation
	nodepool := &NodePool{
		Name:    "test-nodepool",
		OwnerID: "cluster-123",
		Kind:    "CustomNodePool",
	}

	err := nodepool.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(nodepool.Kind).To(Equal("CustomNodePool"))
}

// TestNodePool_BeforeCreate_OwnerKindDefault tests OwnerKind default value
func TestNodePool_BeforeCreate_OwnerKindDefault(t *testing.T) {
	RegisterTestingT(t)

	// Test default OwnerKind
	nodepool := &NodePool{
		Name:    "test-nodepool",
		OwnerID: "cluster-123",
	}

	err := nodepool.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(nodepool.OwnerKind).To(Equal("Cluster"))
}

// TestNodePool_BeforeCreate_OwnerKindPreserved tests OwnerKind is not overwritten
func TestNodePool_BeforeCreate_OwnerKindPreserved(t *testing.T) {
	RegisterTestingT(t)

	// Test OwnerKind preservation
	nodepool := &NodePool{
		Name:      "test-nodepool",
		OwnerID:   "custom-owner-123",
		OwnerKind: "CustomOwner",
	}

	err := nodepool.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(nodepool.OwnerKind).To(Equal("CustomOwner"))
}

// TestNodePool_BeforeCreate_StatusPhaseDefault tests StatusPhase default value
func TestNodePool_BeforeCreate_StatusPhaseDefault(t *testing.T) {
	RegisterTestingT(t)

	// Test default StatusPhase
	nodepool := &NodePool{
		Name:    "test-nodepool",
		OwnerID: "cluster-123",
	}

	err := nodepool.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(nodepool.StatusPhase).To(Equal("NotReady"))
}

// TestNodePool_BeforeCreate_StatusPhasePreserved tests StatusPhase is not overwritten
func TestNodePool_BeforeCreate_StatusPhasePreserved(t *testing.T) {
	RegisterTestingT(t)

	// Test StatusPhase preservation
	nodepool := &NodePool{
		Name:        "test-nodepool",
		OwnerID:     "cluster-123",
		StatusPhase: "Ready",
	}

	err := nodepool.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(nodepool.StatusPhase).To(Equal("Ready"))
}

// TestNodePool_BeforeCreate_HrefGeneration tests Href auto-generation
func TestNodePool_BeforeCreate_HrefGeneration(t *testing.T) {
	RegisterTestingT(t)

	// Test Href generation
	nodepool := &NodePool{
		Name:    "test-nodepool",
		OwnerID: "cluster-abc",
	}

	err := nodepool.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(nodepool.Href).To(Equal("/api/hyperfleet/v1/clusters/cluster-abc/nodepools/" + nodepool.ID))
}

// TestNodePool_BeforeCreate_HrefPreserved tests Href is not overwritten
func TestNodePool_BeforeCreate_HrefPreserved(t *testing.T) {
	RegisterTestingT(t)

	// Test Href preservation
	nodepool := &NodePool{
		Name:    "test-nodepool",
		OwnerID: "cluster-abc",
		Href:    "/custom/href",
	}

	err := nodepool.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(nodepool.Href).To(Equal("/custom/href"))
}

// TestNodePool_BeforeCreate_OwnerHrefGeneration tests OwnerHref auto-generation
func TestNodePool_BeforeCreate_OwnerHrefGeneration(t *testing.T) {
	RegisterTestingT(t)

	// Test OwnerHref generation
	nodepool := &NodePool{
		Name:    "test-nodepool",
		OwnerID: "cluster-xyz",
	}

	err := nodepool.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(nodepool.OwnerHref).To(Equal("/api/hyperfleet/v1/clusters/cluster-xyz"))
}

// TestNodePool_BeforeCreate_OwnerHrefPreserved tests OwnerHref is not overwritten
func TestNodePool_BeforeCreate_OwnerHrefPreserved(t *testing.T) {
	RegisterTestingT(t)

	// Test OwnerHref preservation
	nodepool := &NodePool{
		Name:      "test-nodepool",
		OwnerID:   "cluster-xyz",
		OwnerHref: "/custom/owner/href",
	}

	err := nodepool.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(nodepool.OwnerHref).To(Equal("/custom/owner/href"))
}

// TestNodePool_BeforeCreate_Complete tests all defaults set together
func TestNodePool_BeforeCreate_Complete(t *testing.T) {
	RegisterTestingT(t)

	nodepool := &NodePool{
		Name:    "test-nodepool",
		OwnerID: "cluster-complete",
	}

	err := nodepool.BeforeCreate(nil)
	Expect(err).To(BeNil())

	// Verify all defaults
	Expect(nodepool.ID).ToNot(BeEmpty())
	Expect(nodepool.Kind).To(Equal("NodePool"))
	Expect(nodepool.OwnerKind).To(Equal("Cluster"))
	Expect(nodepool.StatusPhase).To(Equal("NotReady"))
	Expect(nodepool.Href).To(Equal("/api/hyperfleet/v1/clusters/cluster-complete/nodepools/" + nodepool.ID))
	Expect(nodepool.OwnerHref).To(Equal("/api/hyperfleet/v1/clusters/cluster-complete"))
}
