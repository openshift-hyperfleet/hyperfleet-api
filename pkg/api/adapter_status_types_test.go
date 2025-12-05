package api

import (
	"testing"

	. "github.com/onsi/gomega"
)

// TestAdapterStatusList_Index tests the Index() method for AdapterStatusList
func TestAdapterStatusList_Index(t *testing.T) {
	RegisterTestingT(t)

	// Test empty list
	emptyList := AdapterStatusList{}
	emptyIndex := emptyList.Index()
	Expect(len(emptyIndex)).To(Equal(0))

	// Test single adapter status
	status1 := &AdapterStatus{}
	status1.ID = "status-1"
	status1.Adapter = "test-adapter-1"

	singleList := AdapterStatusList{status1}
	singleIndex := singleList.Index()
	Expect(len(singleIndex)).To(Equal(1))
	Expect(singleIndex["status-1"]).To(Equal(status1))

	// Test multiple adapter statuses
	status2 := &AdapterStatus{}
	status2.ID = "status-2"
	status2.Adapter = "test-adapter-2"

	status3 := &AdapterStatus{}
	status3.ID = "status-3"
	status3.Adapter = "test-adapter-3"

	multiList := AdapterStatusList{status1, status2, status3}
	multiIndex := multiList.Index()
	Expect(len(multiIndex)).To(Equal(3))
	Expect(multiIndex["status-1"]).To(Equal(status1))
	Expect(multiIndex["status-2"]).To(Equal(status2))
	Expect(multiIndex["status-3"]).To(Equal(status3))

	// Test duplicate IDs (later one overwrites earlier one)
	status1Duplicate := &AdapterStatus{}
	status1Duplicate.ID = "status-1"
	status1Duplicate.Adapter = "duplicate-adapter"

	duplicateList := AdapterStatusList{status1, status1Duplicate}
	duplicateIndex := duplicateList.Index()
	Expect(len(duplicateIndex)).To(Equal(1))
	Expect(duplicateIndex["status-1"]).To(Equal(status1Duplicate))
	Expect(duplicateIndex["status-1"].Adapter).To(Equal("duplicate-adapter"))
}

// TestAdapterStatus_BeforeCreate_IDGeneration tests ID auto-generation
func TestAdapterStatus_BeforeCreate_IDGeneration(t *testing.T) {
	RegisterTestingT(t)

	status := &AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         "cluster-123",
		Adapter:            "test-adapter",
		ObservedGeneration: 1,
	}

	err := status.BeforeCreate(nil)
	Expect(err).To(BeNil())
	Expect(status.ID).ToNot(BeEmpty())
	Expect(len(status.ID)).To(BeNumerically(">", 0))
}

// TestAdapterStatus_BeforeCreate_NoOtherDefaults tests that no other fields are modified
func TestAdapterStatus_BeforeCreate_NoOtherDefaults(t *testing.T) {
	RegisterTestingT(t)

	// Create status with all fields explicitly set
	status := &AdapterStatus{
		ResourceType:       "NodePool",
		ResourceID:         "nodepool-456",
		Adapter:            "custom-adapter",
		ObservedGeneration: 5,
	}

	err := status.BeforeCreate(nil)
	Expect(err).To(BeNil())

	// Verify only ID was set, other fields preserved
	Expect(status.ID).ToNot(BeEmpty())
	Expect(status.ResourceType).To(Equal("NodePool"))
	Expect(status.ResourceID).To(Equal("nodepool-456"))
	Expect(status.Adapter).To(Equal("custom-adapter"))
	Expect(status.ObservedGeneration).To(Equal(int32(5)))
}
