package api

import (
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"
)

// TestClusterList_Index tests the Index() method for ClusterList
func TestClusterList_Index(t *testing.T) {
	RegisterTestingT(t)

	// Test empty list
	emptyList := ClusterList{}
	emptyIndex := emptyList.Index()
	Expect(len(emptyIndex)).To(Equal(0))

	// Test single cluster
	cluster1 := &Cluster{}
	cluster1.ID = "cluster-1"
	cluster1.Name = "test-cluster-1"

	singleList := ClusterList{cluster1}
	singleIndex := singleList.Index()
	Expect(len(singleIndex)).To(Equal(1))
	Expect(singleIndex["cluster-1"]).To(Equal(cluster1))

	// Test multiple clusters
	cluster2 := &Cluster{}
	cluster2.ID = "cluster-2"
	cluster2.Name = "test-cluster-2"

	cluster3 := &Cluster{}
	cluster3.ID = "cluster-3"
	cluster3.Name = "test-cluster-3"

	multiList := ClusterList{cluster1, cluster2, cluster3}
	multiIndex := multiList.Index()
	Expect(len(multiIndex)).To(Equal(3))
	Expect(multiIndex["cluster-1"]).To(Equal(cluster1))
	Expect(multiIndex["cluster-2"]).To(Equal(cluster2))
	Expect(multiIndex["cluster-3"]).To(Equal(cluster3))

	// Test duplicate IDs (later one overwrites earlier one)
	cluster1Duplicate := &Cluster{}
	cluster1Duplicate.ID = "cluster-1"
	cluster1Duplicate.Name = "duplicate-cluster"

	duplicateList := ClusterList{cluster1, cluster1Duplicate}
	duplicateIndex := duplicateList.Index()
	Expect(len(duplicateIndex)).To(Equal(1))
	Expect(duplicateIndex["cluster-1"]).To(Equal(cluster1Duplicate))
	Expect(duplicateIndex["cluster-1"].Name).To(Equal("duplicate-cluster"))
}

// TestCluster_BeforeCreate_IDGeneration tests ID auto-generation
func TestCluster_BeforeCreate_IDGeneration(t *testing.T) {
	RegisterTestingT(t)

	cluster := &Cluster{
		Name: "test-cluster",
	}

	err := cluster.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(cluster.ID).ToNot(BeEmpty())
	Expect(len(cluster.ID)).To(BeNumerically(">", 0))
}

// TestCluster_BeforeCreate_KindPreservation tests Kind is preserved (not auto-set)
func TestCluster_BeforeCreate_KindPreservation(t *testing.T) {
	RegisterTestingT(t)

	// Kind must be set before BeforeCreate (by handler validation)
	cluster := &Cluster{
		Name: "test-cluster",
		Kind: "Cluster",
	}

	err := cluster.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(cluster.Kind).To(Equal("Cluster"))
}

// TestCluster_BeforeCreate_KindPreserved tests Kind is not overwritten
func TestCluster_BeforeCreate_KindPreserved(t *testing.T) {
	RegisterTestingT(t)

	// Test Kind preservation
	cluster := &Cluster{
		Name: "test-cluster",
		Kind: "CustomCluster",
	}

	err := cluster.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(cluster.Kind).To(Equal("CustomCluster"))
}

// TestCluster_BeforeCreate_GenerationDefault tests Generation default value
func TestCluster_BeforeCreate_GenerationDefault(t *testing.T) {
	RegisterTestingT(t)

	// Test default Generation
	cluster := &Cluster{
		Name: "test-cluster",
	}

	err := cluster.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(cluster.Generation).To(Equal(int32(1)))
}

// TestCluster_BeforeCreate_GenerationPreserved tests Generation is not overwritten
func TestCluster_BeforeCreate_GenerationPreserved(t *testing.T) {
	RegisterTestingT(t)

	// Test Generation preservation
	cluster := &Cluster{
		Name:       "test-cluster",
		Generation: 5,
	}

	err := cluster.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(cluster.Generation).To(Equal(int32(5)))
}

// TestCluster_BeforeCreate_HrefGeneration tests Href auto-generation
func TestCluster_BeforeCreate_HrefGeneration(t *testing.T) {
	RegisterTestingT(t)

	// Test Href generation
	cluster := &Cluster{
		Name: "test-cluster",
	}

	err := cluster.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(cluster.Href).To(Equal("/api/hyperfleet/v1/clusters/" + cluster.ID))
}

// TestCluster_BeforeCreate_HrefPreserved tests Href is not overwritten
func TestCluster_BeforeCreate_HrefPreserved(t *testing.T) {
	RegisterTestingT(t)

	// Test Href preservation
	cluster := &Cluster{
		Name: "test-cluster",
		Href: "/custom/href",
	}

	err := cluster.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(cluster.Href).To(Equal("/custom/href"))
}

// TestCluster_BeforeCreate_Complete tests all defaults set together
func TestCluster_BeforeCreate_Complete(t *testing.T) {
	RegisterTestingT(t)

	cluster := &Cluster{
		Name: "test-cluster",
		Kind: "Cluster", // Kind must be set before BeforeCreate
	}

	err := cluster.BeforeCreate(nil)
	Expect(err).To(BeNil())

	// Verify all defaults
	Expect(cluster.ID).ToNot(BeEmpty())
	Expect(cluster.Kind).To(Equal("Cluster")) // Kind is preserved, not auto-set
	Expect(cluster.Generation).To(Equal(int32(1)))
	Expect(cluster.Href).To(Equal("/api/hyperfleet/v1/clusters/" + cluster.ID))
}

// TestCluster_BeforeCreate_UUIDGeneration tests UUID auto-generation
func TestCluster_BeforeCreate_UUIDGeneration(t *testing.T) {
	RegisterTestingT(t)

	cluster := &Cluster{
		Name: "test-cluster",
	}

	err := cluster.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(cluster.UUID).ToNot(BeEmpty())

	// Verify UUID format (RFC4122 v4)
	parsedUUID, err := uuid.Parse(cluster.UUID)
	Expect(err).To(BeNil())
	Expect(parsedUUID.String()).To(Equal(cluster.UUID))
}

// TestCluster_BeforeCreate_UUIDUniqueness tests that each cluster gets a unique UUID
func TestCluster_BeforeCreate_UUIDUniqueness(t *testing.T) {
	RegisterTestingT(t)

	cluster1 := &Cluster{Name: "cluster-1"}
	cluster2 := &Cluster{Name: "cluster-2"}
	cluster3 := &Cluster{Name: "cluster-3"}

	_ = cluster1.BeforeCreate(nil)
	_ = cluster2.BeforeCreate(nil)
	_ = cluster3.BeforeCreate(nil)

	// All UUIDs should be different
	Expect(cluster1.UUID).ToNot(Equal(cluster2.UUID))
	Expect(cluster1.UUID).ToNot(Equal(cluster3.UUID))
	Expect(cluster2.UUID).ToNot(Equal(cluster3.UUID))

	// All should be valid UUIDs
	_, err1 := uuid.Parse(cluster1.UUID)
	_, err2 := uuid.Parse(cluster2.UUID)
	_, err3 := uuid.Parse(cluster3.UUID)
	Expect(err1).To(BeNil())
	Expect(err2).To(BeNil())
	Expect(err3).To(BeNil())
}

// TestCluster_BeforeCreate_UUIDAndIDDifferent tests that UUID and ID are independent
func TestCluster_BeforeCreate_UUIDAndIDDifferent(t *testing.T) {
	RegisterTestingT(t)

	cluster := &Cluster{Name: "test-cluster"}

	err := cluster.BeforeCreate(nil)
	Expect(err).To(BeNil())

	// UUID and ID should both be set
	Expect(cluster.UUID).ToNot(BeEmpty())
	Expect(cluster.ID).ToNot(BeEmpty())

	// UUID and ID should be different (UUID is hyphenated, ID is KSUID)
	Expect(cluster.UUID).ToNot(Equal(cluster.ID))

	// UUID should contain hyphens (RFC4122 format)
	Expect(cluster.UUID).To(ContainSubstring("-"))

	// ID should not contain hyphens (KSUID format)
	Expect(cluster.ID).ToNot(ContainSubstring("-"))
}

// TestCluster_BeforeCreate_UUIDImmutable tests UUID is set once and preserved on subsequent calls
func TestCluster_BeforeCreate_UUIDImmutable(t *testing.T) {
	RegisterTestingT(t)

	cluster := &Cluster{Name: "test-cluster"}

	// First BeforeCreate call
	err := cluster.BeforeCreate(nil)
	Expect(err).To(BeNil())
	firstUUID := cluster.UUID
	firstID := cluster.ID

	// Second BeforeCreate call (simulating accidental double-call)
	err = cluster.BeforeCreate(nil)
	Expect(err).To(BeNil())

	// UUID and ID should be preserved (idempotent behavior)
	// This prevents data corruption if BeforeCreate is accidentally called multiple times
	Expect(cluster.UUID).To(Equal(firstUUID))
	Expect(cluster.ID).To(Equal(firstID))

	// UUID should still be valid
	_, err1 := uuid.Parse(cluster.UUID)
	Expect(err1).To(BeNil())
}
