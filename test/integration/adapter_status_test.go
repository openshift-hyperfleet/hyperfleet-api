package integration

import (
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

// TestClusterStatusPost tests creating adapter status for a cluster
func TestClusterStatusPost(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster first
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Create an adapter status for the cluster
	statusInput := openapi.AdapterStatusCreateRequest{
		Adapter:            "test-adapter",
		ObservedGeneration: cluster.Generation,
		Conditions: []openapi.ConditionRequest{
			{
				Type:   "Ready",
				Status: "True",
				Reason: openapi.PtrString("AdapterReady"),
			},
		},
		Data: map[string]map[string]interface{}{
			"test_key": {"value": "test_value"},
		},
	}

	statusOutput, resp, err := client.DefaultAPI.PostClusterStatuses(ctx, cluster.ID).AdapterStatusCreateRequest(statusInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting cluster status: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(statusOutput.Adapter).To(Equal("test-adapter"))
	Expect(statusOutput.ObservedGeneration).To(Equal(cluster.Generation))
	Expect(len(statusOutput.Conditions)).To(BeNumerically(">", 0))
}

// TestClusterStatusGet tests retrieving adapter statuses for a cluster
func TestClusterStatusGet(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster first
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Create a few adapter statuses
	for i := 0; i < 3; i++ {
		statusInput := openapi.AdapterStatusCreateRequest{
			Adapter:            fmt.Sprintf("adapter-%d", i),
			ObservedGeneration: cluster.Generation,
			Conditions: []openapi.ConditionRequest{
				{
					Type:   "Ready",
					Status: "True",
				},
			},
		}
		_, _, err := client.DefaultAPI.PostClusterStatuses(ctx, cluster.ID).AdapterStatusCreateRequest(statusInput).Execute()
		Expect(err).NotTo(HaveOccurred())
	}

	// Get all statuses for the cluster
	list, resp, err := client.DefaultAPI.GetClusterStatuses(ctx, cluster.ID).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting cluster statuses: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(list).NotTo(BeNil())
	Expect(len(list.Items)).To(BeNumerically(">=", 3))
}

// TestNodePoolStatusPost tests creating adapter status for a nodepool
func TestNodePoolStatusPost(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a nodepool (which also creates its parent cluster)
	nodePool, err := h.Factories.NewNodePools(h.NewID())
	Expect(err).NotTo(HaveOccurred())
	Expect(nodePool).NotTo(BeNil(), "nodePool should not be nil")
	Expect(nodePool.OwnerID).NotTo(BeEmpty(), "nodePool.OwnerID should not be empty")
	Expect(nodePool.ID).NotTo(BeEmpty(), "nodePool.ID should not be empty")

	// Create an adapter status for the nodepool
	statusInput := openapi.AdapterStatusCreateRequest{
		Adapter:            "test-nodepool-adapter",
		ObservedGeneration: 1,
		Conditions: []openapi.ConditionRequest{
			{
				Type:   "Ready",
				Status: "False",
				Reason: openapi.PtrString("Initializing"),
			},
		},
		Data: map[string]map[string]interface{}{
			"nodepool_data": {"value": "test_value"},
		},
	}

	// Use nodePool.OwnerID as the cluster_id parameter
	statusOutput, resp, err := client.DefaultAPI.PostNodePoolStatuses(ctx, nodePool.OwnerID, nodePool.ID).AdapterStatusCreateRequest(statusInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting nodepool status: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(statusOutput.Adapter).To(Equal("test-nodepool-adapter"))
	Expect(len(statusOutput.Conditions)).To(BeNumerically(">", 0))
}

// TestNodePoolStatusGet tests retrieving adapter statuses for a nodepool
func TestNodePoolStatusGet(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a nodepool (which also creates its parent cluster)
	nodePool, err := h.Factories.NewNodePools(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Create a few adapter statuses
	for i := 0; i < 2; i++ {
		statusInput := openapi.AdapterStatusCreateRequest{
			Adapter:            fmt.Sprintf("nodepool-adapter-%d", i),
			ObservedGeneration: 1,
			Conditions: []openapi.ConditionRequest{
				{
					Type:   "Ready",
					Status: "True",
				},
			},
		}
		// Use nodePool.OwnerID as the cluster_id parameter
		_, _, err := client.DefaultAPI.PostNodePoolStatuses(ctx, nodePool.OwnerID, nodePool.ID).AdapterStatusCreateRequest(statusInput).Execute()
		Expect(err).NotTo(HaveOccurred())
	}

	// Get all statuses for the nodepool
	list, resp, err := client.DefaultAPI.GetNodePoolsStatuses(ctx, nodePool.OwnerID, nodePool.ID).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting nodepool statuses: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(list).NotTo(BeNil())
	Expect(len(list.Items)).To(BeNumerically(">=", 2))
}

// TestAdapterStatusPaging tests paging for adapter statuses
func TestAdapterStatusPaging(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Create multiple statuses
	for i := 0; i < 10; i++ {
		statusInput := openapi.AdapterStatusCreateRequest{
			Adapter:            fmt.Sprintf("adapter-%d", i),
			ObservedGeneration: cluster.Generation,
			Conditions: []openapi.ConditionRequest{
				{
					Type:   "Ready",
					Status: "True",
				},
			},
		}
		_, _, err := client.DefaultAPI.PostClusterStatuses(ctx, cluster.ID).AdapterStatusCreateRequest(statusInput).Execute()
		Expect(err).NotTo(HaveOccurred())
	}

	// Test paging
	list, _, err := client.DefaultAPI.GetClusterStatuses(ctx, cluster.ID).Page(1).PageSize(5).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(len(list.Items)).To(BeNumerically("<=", 5))
	Expect(list.Page).To(Equal(int32(1)))
}

// TestAdapterStatusIdempotency tests that posting the same adapter twice updates instead of creating duplicate
func TestAdapterStatusIdempotency(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// First POST: Create adapter status
	statusInput1 := openapi.AdapterStatusCreateRequest{
		Adapter:            "idempotency-test-adapter",
		ObservedGeneration: cluster.Generation,
		Conditions: []openapi.ConditionRequest{
			{
				Type:   "Ready",
				Status: "False",
				Reason: openapi.PtrString("Initializing"),
			},
		},
		Data: map[string]map[string]interface{}{
			"version": {"value": "1.0"},
		},
	}

	status1, resp, err := client.DefaultAPI.PostClusterStatuses(ctx, cluster.ID).AdapterStatusCreateRequest(statusInput1).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(status1.Adapter).To(Equal("idempotency-test-adapter"))
	Expect(status1.Conditions[0].Status).To(Equal("False"))

	// Second POST: Update the same adapter with different conditions
	statusInput2 := openapi.AdapterStatusCreateRequest{
		Adapter:            "idempotency-test-adapter",
		ObservedGeneration: cluster.Generation,
		Conditions: []openapi.ConditionRequest{
			{
				Type:   "Ready",
				Status: "True",
				Reason: openapi.PtrString("AdapterReady"),
			},
		},
		Data: map[string]map[string]interface{}{
			"version": {"value": "2.0"},
		},
	}

	status2, resp, err := client.DefaultAPI.PostClusterStatuses(ctx, cluster.ID).AdapterStatusCreateRequest(statusInput2).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(status2.Adapter).To(Equal("idempotency-test-adapter"))
	Expect(status2.Conditions[0].Status).To(Equal("True"))

	// GET all statuses - should have only ONE status for "idempotency-test-adapter"
	list, _, err := client.DefaultAPI.GetClusterStatuses(ctx, cluster.ID).Execute()
	Expect(err).NotTo(HaveOccurred())

	// Count how many times this adapter appears
	adapterCount := 0
	var finalStatus openapi.AdapterStatus
	for _, s := range list.Items {
		if s.Adapter == "idempotency-test-adapter" {
			adapterCount++
			finalStatus = s
		}
	}

	// Verify: should have exactly ONE entry for this adapter (updated, not duplicated)
	Expect(adapterCount).To(Equal(1), "Adapter should be updated, not duplicated")
	Expect(finalStatus.Conditions[0].Status).To(Equal("True"), "Conditions should be updated to latest")
}

// TestAdapterStatusPagingEdgeCases tests edge cases in pagination
func TestAdapterStatusPagingEdgeCases(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Create exactly 10 statuses
	for i := 0; i < 10; i++ {
		statusInput := openapi.AdapterStatusCreateRequest{
			Adapter:            fmt.Sprintf("edge-adapter-%d", i),
			ObservedGeneration: cluster.Generation,
			Conditions: []openapi.ConditionRequest{
				{
					Type:   "Ready",
					Status: "True",
				},
			},
		}
		_, _, err := client.DefaultAPI.PostClusterStatuses(ctx, cluster.ID).AdapterStatusCreateRequest(statusInput).Execute()
		Expect(err).NotTo(HaveOccurred())
	}

	// Test 1: Empty dataset pagination (different cluster with no statuses)
	emptyCluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	emptyList, _, err := client.DefaultAPI.GetClusterStatuses(ctx, emptyCluster.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(emptyList.Total).To(Equal(int32(0)))
	Expect(len(emptyList.Items)).To(Equal(0))

	// Test 2: Page beyond total pages
	beyondList, _, err := client.DefaultAPI.GetClusterStatuses(ctx, cluster.ID).Page(100).PageSize(5).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(len(beyondList.Items)).To(Equal(0), "Should return empty when page exceeds total pages")
	Expect(beyondList.Total).To(Equal(int32(10)), "Total should still reflect actual count")

	// Test 3: Single item dataset
	singleCluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	singleStatus := openapi.AdapterStatusCreateRequest{
		Adapter:            "single-adapter",
		ObservedGeneration: singleCluster.Generation,
		Conditions: []openapi.ConditionRequest{
			{
				Type:   "Ready",
				Status: "True",
			},
		},
	}
	_, _, err = client.DefaultAPI.PostClusterStatuses(ctx, singleCluster.ID).AdapterStatusCreateRequest(singleStatus).Execute()
	Expect(err).NotTo(HaveOccurred())

	singleList, _, err := client.DefaultAPI.GetClusterStatuses(ctx, singleCluster.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(singleList.Total).To(Equal(int32(1)))
	Expect(len(singleList.Items)).To(Equal(1))
	Expect(singleList.Page).To(Equal(int32(1)))

	// Test 4: Pagination consistency - verify no duplicates and no missing items
	allItems := make(map[string]bool)
	page := 1
	pageSize := 3

	for {
		list, _, err := client.DefaultAPI.GetClusterStatuses(ctx, cluster.ID).Page(int32(page)).PageSize(int32(pageSize)).Execute()
		Expect(err).NotTo(HaveOccurred())

		if len(list.Items) == 0 {
			break
		}

		for _, item := range list.Items {
			adapter := item.Adapter
			Expect(allItems[adapter]).To(BeFalse(), "Duplicate adapter found in pagination: %s", adapter)
			allItems[adapter] = true
		}

		page++
		if page > 10 {
			break // Safety limit
		}
	}

	// Verify we got all 10 unique adapters
	Expect(len(allItems)).To(Equal(10), "Should retrieve all items exactly once across pages")
}
