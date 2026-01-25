package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

// Helper to create AdapterStatusCreateRequest
func newAdapterStatusRequest(adapter string, observedGen int32, conditions []openapi.ConditionRequest, data *map[string]interface{}) openapi.AdapterStatusCreateRequest {
	return openapi.AdapterStatusCreateRequest{
		Adapter:            adapter,
		ObservedGeneration: observedGen,
		Data:               data,
		Conditions:         conditions,
		ObservedTime:       time.Now(),
	}
}

// TestClusterStatusPost tests creating adapter status for a cluster
func TestClusterStatusPost(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster first
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Create an adapter status for the cluster
	data := map[string]interface{}{
		"test_key": map[string]interface{}{"value": "test_value"},
	}
	statusInput := newAdapterStatusRequest(
		"test-adapter",
		cluster.Generation,
		[]openapi.ConditionRequest{
			{
				Type:   "Ready",
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("AdapterReady"),
			},
		},
		&data,
	)

	resp, err := client.PostClusterStatusesWithResponse(ctx, cluster.ID, openapi.PostClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Error posting cluster status: %v", err)
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated))
	Expect(resp.JSON201).NotTo(BeNil())
	Expect(resp.JSON201.Adapter).To(Equal("test-adapter"))
	Expect(resp.JSON201.ObservedGeneration).To(Equal(cluster.Generation))
	Expect(len(resp.JSON201.Conditions)).To(BeNumerically(">", 0))
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
		statusInput := newAdapterStatusRequest(
			fmt.Sprintf("adapter-%d", i),
			cluster.Generation,
			[]openapi.ConditionRequest{
				{
					Type:   "Ready",
					Status: openapi.AdapterConditionStatusTrue,
				},
			},
			nil,
		)
		_, err := client.PostClusterStatusesWithResponse(ctx, cluster.ID, openapi.PostClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx))
		Expect(err).NotTo(HaveOccurred())
	}

	// Get all statuses for the cluster
	resp, err := client.GetClusterStatusesWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Error getting cluster statuses: %v", err)
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))
	Expect(resp.JSON200).NotTo(BeNil())
	Expect(len(resp.JSON200.Items)).To(BeNumerically(">=", 3))
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
	data := map[string]interface{}{
		"nodepool_data": map[string]interface{}{"value": "test_value"},
	}
	statusInput := newAdapterStatusRequest(
		"test-nodepool-adapter",
		1,
		[]openapi.ConditionRequest{
			{
				Type:   "Ready",
				Status: openapi.AdapterConditionStatusFalse,
				Reason: util.PtrString("Initializing"),
			},
		},
		&data,
	)

	// Use nodePool.OwnerID as the cluster_id parameter
	resp, err := client.PostNodePoolStatusesWithResponse(ctx, nodePool.OwnerID, nodePool.ID, openapi.PostNodePoolStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Error posting nodepool status: %v", err)
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated))
	Expect(resp.JSON201).NotTo(BeNil())
	Expect(resp.JSON201.Adapter).To(Equal("test-nodepool-adapter"))
	Expect(len(resp.JSON201.Conditions)).To(BeNumerically(">", 0))
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
		statusInput := newAdapterStatusRequest(
			fmt.Sprintf("nodepool-adapter-%d", i),
			1,
			[]openapi.ConditionRequest{
				{
					Type:   "Ready",
					Status: openapi.AdapterConditionStatusTrue,
				},
			},
			nil,
		)
		// Use nodePool.OwnerID as the cluster_id parameter
		_, err := client.PostNodePoolStatusesWithResponse(ctx, nodePool.OwnerID, nodePool.ID, openapi.PostNodePoolStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx))
		Expect(err).NotTo(HaveOccurred())
	}

	// Get all statuses for the nodepool
	resp, err := client.GetNodePoolsStatusesWithResponse(ctx, nodePool.OwnerID, nodePool.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Error getting nodepool statuses: %v", err)
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))
	Expect(resp.JSON200).NotTo(BeNil())
	Expect(len(resp.JSON200.Items)).To(BeNumerically(">=", 2))
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
		statusInput := newAdapterStatusRequest(
			fmt.Sprintf("adapter-%d", i),
			cluster.Generation,
			[]openapi.ConditionRequest{
				{
					Type:   "Ready",
					Status: openapi.AdapterConditionStatusTrue,
				},
			},
			nil,
		)
		_, err := client.PostClusterStatusesWithResponse(ctx, cluster.ID, openapi.PostClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx))
		Expect(err).NotTo(HaveOccurred())
	}

	// Test paging
	page := openapi.QueryParamsPage(1)
	pageSize := openapi.QueryParamsPageSize(5)
	params := &openapi.GetClusterStatusesParams{
		Page:     &page,
		PageSize: &pageSize,
	}
	resp, err := client.GetClusterStatusesWithResponse(ctx, cluster.ID, params, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.JSON200).NotTo(BeNil())
	Expect(len(resp.JSON200.Items)).To(BeNumerically("<=", 5))
	Expect(resp.JSON200.Page).To(Equal(int32(1)))
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
	data1 := map[string]interface{}{
		"version": map[string]interface{}{"value": "1.0"},
	}
	statusInput1 := newAdapterStatusRequest(
		"idempotency-test-adapter",
		cluster.Generation,
		[]openapi.ConditionRequest{
			{
				Type:   "Ready",
				Status: openapi.AdapterConditionStatusFalse,
				Reason: util.PtrString("Initializing"),
			},
		},
		&data1,
	)

	resp1, err := client.PostClusterStatusesWithResponse(ctx, cluster.ID, openapi.PostClusterStatusesJSONRequestBody(statusInput1), test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp1.StatusCode()).To(Equal(http.StatusCreated))
	Expect(resp1.JSON201).NotTo(BeNil())
	Expect(resp1.JSON201.Adapter).To(Equal("idempotency-test-adapter"))
	Expect(resp1.JSON201.Conditions[0].Status).To(Equal(openapi.AdapterConditionStatusFalse))

	// Second POST: Update the same adapter with different conditions
	data2 := map[string]interface{}{
		"version": map[string]interface{}{"value": "2.0"},
	}
	statusInput2 := newAdapterStatusRequest(
		"idempotency-test-adapter",
		cluster.Generation,
		[]openapi.ConditionRequest{
			{
				Type:   "Ready",
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("AdapterReady"),
			},
		},
		&data2,
	)

	resp2, err := client.PostClusterStatusesWithResponse(ctx, cluster.ID, openapi.PostClusterStatusesJSONRequestBody(statusInput2), test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp2.StatusCode()).To(Equal(http.StatusCreated))
	Expect(resp2.JSON201).NotTo(BeNil())
	Expect(resp2.JSON201.Adapter).To(Equal("idempotency-test-adapter"))
	Expect(resp2.JSON201.Conditions[0].Status).To(Equal(openapi.AdapterConditionStatusTrue))

	// GET all statuses - should have only ONE status for "idempotency-test-adapter"
	listResp, err := client.GetClusterStatusesWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(listResp.JSON200).NotTo(BeNil())

	// Count how many times this adapter appears
	adapterCount := 0
	var finalStatus openapi.AdapterStatus
	for _, s := range listResp.JSON200.Items {
		if s.Adapter == "idempotency-test-adapter" {
			adapterCount++
			finalStatus = s
		}
	}

	// Verify: should have exactly ONE entry for this adapter (updated, not duplicated)
	Expect(adapterCount).To(Equal(1), "Adapter should be updated, not duplicated")
	Expect(finalStatus.Conditions[0].Status).To(Equal(openapi.AdapterConditionStatusTrue), "Conditions should be updated to latest")
}

// TestClusterStatusPost_UnknownReturns204 tests that posting Unknown Available status returns 204 No Content
func TestClusterStatusPost_UnknownReturns204(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster first
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Create an adapter status with Available=Unknown
	statusInput := newAdapterStatusRequest(
		"test-adapter-unknown",
		cluster.Generation,
		[]openapi.ConditionRequest{
			{
				Type:   "Available",
				Status: openapi.AdapterConditionStatusUnknown,
				Reason: util.PtrString("StartupPending"),
			},
		},
		nil,
	)

	resp, err := client.PostClusterStatusesWithResponse(ctx, cluster.ID, openapi.PostClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Error posting cluster status: %v", err)
	Expect(resp.StatusCode()).To(Equal(http.StatusNoContent), "Expected 204 No Content for Unknown status")

	// Verify the status was NOT stored
	listResp, err := client.GetClusterStatusesWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(listResp.JSON200).NotTo(BeNil())

	// Check that no adapter status with "test-adapter-unknown" exists
	for _, s := range listResp.JSON200.Items {
		Expect(s.Adapter).NotTo(Equal("test-adapter-unknown"), "Unknown status should not be stored")
	}
}

// TestNodePoolStatusPost_UnknownReturns204 tests that posting Unknown Available status returns 204 No Content
func TestNodePoolStatusPost_UnknownReturns204(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a nodepool (which also creates its parent cluster)
	nodePool, err := h.Factories.NewNodePools(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Create an adapter status with Available=Unknown
	statusInput := newAdapterStatusRequest(
		"test-nodepool-adapter-unknown",
		1,
		[]openapi.ConditionRequest{
			{
				Type:   "Available",
				Status: openapi.AdapterConditionStatusUnknown,
				Reason: util.PtrString("StartupPending"),
			},
		},
		nil,
	)

	resp, err := client.PostNodePoolStatusesWithResponse(ctx, nodePool.OwnerID, nodePool.ID, openapi.PostNodePoolStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Error posting nodepool status: %v", err)
	Expect(resp.StatusCode()).To(Equal(http.StatusNoContent), "Expected 204 No Content for Unknown status")

	// Verify the status was NOT stored
	listResp, err := client.GetNodePoolsStatusesWithResponse(ctx, nodePool.OwnerID, nodePool.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(listResp.JSON200).NotTo(BeNil())

	// Check that no adapter status with "test-nodepool-adapter-unknown" exists
	for _, s := range listResp.JSON200.Items {
		Expect(s.Adapter).NotTo(Equal("test-nodepool-adapter-unknown"), "Unknown status should not be stored")
	}
}

// TestClusterStatusPost_MultipleConditionsWithUnknownAvailable tests that Unknown Available is detected among multiple conditions
func TestClusterStatusPost_MultipleConditionsWithUnknownAvailable(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster first
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Create an adapter status with multiple conditions including Available=Unknown
	statusInput := newAdapterStatusRequest(
		"test-adapter-multi-unknown",
		cluster.Generation,
		[]openapi.ConditionRequest{
			{
				Type:   "Ready",
				Status: openapi.AdapterConditionStatusTrue,
			},
			{
				Type:   "Available",
				Status: openapi.AdapterConditionStatusUnknown,
				Reason: util.PtrString("StartupPending"),
			},
			{
				Type:   "Progressing",
				Status: openapi.AdapterConditionStatusTrue,
			},
		},
		nil,
	)

	resp, err := client.PostClusterStatusesWithResponse(ctx, cluster.ID, openapi.PostClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Error posting cluster status: %v", err)
	Expect(resp.StatusCode()).To(Equal(http.StatusNoContent), "Expected 204 No Content when Available=Unknown among multiple conditions")
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
		statusInput := newAdapterStatusRequest(
			fmt.Sprintf("edge-adapter-%d", i),
			cluster.Generation,
			[]openapi.ConditionRequest{
				{
					Type:   "Ready",
					Status: openapi.AdapterConditionStatusTrue,
				},
			},
			nil,
		)
		_, err := client.PostClusterStatusesWithResponse(ctx, cluster.ID, openapi.PostClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx))
		Expect(err).NotTo(HaveOccurred())
	}

	// Test 1: Empty dataset pagination (different cluster with no statuses)
	emptyCluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	emptyResp, err := client.GetClusterStatusesWithResponse(ctx, emptyCluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(emptyResp.JSON200).NotTo(BeNil())
	Expect(emptyResp.JSON200.Total).To(Equal(int32(0)))
	Expect(len(emptyResp.JSON200.Items)).To(Equal(0))

	// Test 2: Page beyond total pages
	page100 := openapi.QueryParamsPage(100)
	pageSize5 := openapi.QueryParamsPageSize(5)
	beyondParams := &openapi.GetClusterStatusesParams{
		Page:     &page100,
		PageSize: &pageSize5,
	}
	beyondResp, err := client.GetClusterStatusesWithResponse(ctx, cluster.ID, beyondParams, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(beyondResp.JSON200).NotTo(BeNil())
	Expect(len(beyondResp.JSON200.Items)).To(Equal(0), "Should return empty when page exceeds total pages")
	Expect(beyondResp.JSON200.Total).To(Equal(int32(10)), "Total should still reflect actual count")

	// Test 3: Single item dataset
	singleCluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	singleStatus := newAdapterStatusRequest(
		"single-adapter",
		singleCluster.Generation,
		[]openapi.ConditionRequest{
			{
				Type:   "Ready",
				Status: openapi.AdapterConditionStatusTrue,
			},
		},
		nil,
	)
	_, err = client.PostClusterStatusesWithResponse(ctx, singleCluster.ID, openapi.PostClusterStatusesJSONRequestBody(singleStatus), test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())

	singleResp, err := client.GetClusterStatusesWithResponse(ctx, singleCluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(singleResp.JSON200).NotTo(BeNil())
	Expect(singleResp.JSON200.Total).To(Equal(int32(1)))
	Expect(len(singleResp.JSON200.Items)).To(Equal(1))
	Expect(singleResp.JSON200.Page).To(Equal(int32(1)))

	// Test 4: Pagination consistency - verify no duplicates and no missing items
	allItems := make(map[string]bool)
	pageNum := openapi.QueryParamsPage(1)
	pageSz := openapi.QueryParamsPageSize(3)

	for {
		params := &openapi.GetClusterStatusesParams{
			Page:     &pageNum,
			PageSize: &pageSz,
		}
		listResp, err := client.GetClusterStatusesWithResponse(ctx, cluster.ID, params, test.WithAuthToken(ctx))
		Expect(err).NotTo(HaveOccurred())
		Expect(listResp.JSON200).NotTo(BeNil())

		if len(listResp.JSON200.Items) == 0 {
			break
		}

		for _, item := range listResp.JSON200.Items {
			adapter := item.Adapter
			Expect(allItems[adapter]).To(BeFalse(), "Duplicate adapter found in pagination: %s", adapter)
			allItems[adapter] = true
		}

		pageNum++
		if pageNum > 10 {
			break // Safety limit
		}
	}

	// Verify we got all 10 unique adapters
	Expect(len(allItems)).To(Equal(10), "Should retrieve all items exactly once across pages")
}
