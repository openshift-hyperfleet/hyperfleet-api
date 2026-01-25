package api

import (
	"testing"

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
