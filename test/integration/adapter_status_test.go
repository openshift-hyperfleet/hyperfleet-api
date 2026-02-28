package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

// Helper to create AdapterStatusCreateRequest
func newAdapterStatusRequest(
	adapter string, observedGen int32, conditions []openapi.ConditionRequest, data *map[string]interface{},
) openapi.AdapterStatusCreateRequest {
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
				Type:   api.ConditionTypeAvailable,
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("ClusterAvailable"),
			},
			{
				Type:   api.ConditionTypeApplied,
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("ConfigurationApplied"),
			},
			{
				Type:   api.ConditionTypeHealth,
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("HealthyCluster"),
			},
			{
				Type:   api.ConditionTypeReady,
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("AdapterReady"),
			},
		},
		&data,
	)

	resp, err := client.PostClusterStatusesWithResponse(
		ctx, cluster.ID,
		openapi.PostClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
	)
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
					Type:   api.ConditionTypeAvailable,
					Status: openapi.AdapterConditionStatusTrue,
				},
				{
					Type:   api.ConditionTypeApplied,
					Status: openapi.AdapterConditionStatusTrue,
				},
				{
					Type:   api.ConditionTypeHealth,
					Status: openapi.AdapterConditionStatusTrue,
				},
				{
					Type:   api.ConditionTypeReady,
					Status: openapi.AdapterConditionStatusTrue,
				},
			},
			nil,
		)
		_, err := client.PostClusterStatusesWithResponse(
			ctx, cluster.ID,
			openapi.PostClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
		)
		Expect(err).NotTo(HaveOccurred())
	}

	// Get all statuses for the cluster
	resp, err := client.GetClusterStatusesWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Error getting cluster statuses: %v", err)
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))
	Expect(resp.JSON200).NotTo(BeNil())
	Expect(len(resp.JSON200.Items)).To(BeNumerically(">=", 3))
}

// TestClusterStatusGet_NonExistentCluster tests that getting status for non-existent cluster returns 404
func TestClusterStatusGet_NonExistentCluster(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Get status for non-existent cluster
	resp, err := client.GetClusterStatusesWithResponse(
		ctx, "non-existent-cluster", nil, test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusNotFound),
		"Expected 404 Not Found for non-existent cluster")
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
				Type:   api.ConditionTypeAvailable,
				Status: openapi.AdapterConditionStatusFalse,
				Reason: util.PtrString("Initializing"),
			},
			{
				Type:   api.ConditionTypeApplied,
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("ConfigurationApplied"),
			},
			{
				Type:   api.ConditionTypeHealth,
				Status: openapi.AdapterConditionStatusFalse,
				Reason: util.PtrString("Initializing"),
			},
			{
				Type:   api.ConditionTypeReady,
				Status: openapi.AdapterConditionStatusFalse,
				Reason: util.PtrString("Initializing"),
			},
		},
		&data,
	)

	// Use nodePool.OwnerID as the cluster_id parameter
	resp, err := client.PostNodePoolStatusesWithResponse(
		ctx, nodePool.OwnerID, nodePool.ID,
		openapi.PostNodePoolStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
	)
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
					Type:   api.ConditionTypeAvailable,
					Status: openapi.AdapterConditionStatusTrue,
				},
				{
					Type:   api.ConditionTypeApplied,
					Status: openapi.AdapterConditionStatusTrue,
				},
				{
					Type:   api.ConditionTypeHealth,
					Status: openapi.AdapterConditionStatusTrue,
				},
				{
					Type:   api.ConditionTypeReady,
					Status: openapi.AdapterConditionStatusTrue,
				},
			},
			nil,
		)
		// Use nodePool.OwnerID as the cluster_id parameter
		_, err := client.PostNodePoolStatusesWithResponse(
			ctx, nodePool.OwnerID, nodePool.ID,
			openapi.PostNodePoolStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
		)
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
					Type:   api.ConditionTypeAvailable,
					Status: openapi.AdapterConditionStatusTrue,
				},
				{
					Type:   api.ConditionTypeApplied,
					Status: openapi.AdapterConditionStatusTrue,
				},
				{
					Type:   api.ConditionTypeHealth,
					Status: openapi.AdapterConditionStatusTrue,
				},
				{
					Type:   api.ConditionTypeReady,
					Status: openapi.AdapterConditionStatusTrue,
				},
			},
			nil,
		)
		_, err := client.PostClusterStatusesWithResponse(
			ctx, cluster.ID,
			openapi.PostClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
		)
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
				Type:   api.ConditionTypeAvailable,
				Status: openapi.AdapterConditionStatusFalse,
				Reason: util.PtrString("Initializing"),
			},
			{
				Type:   api.ConditionTypeApplied,
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("ConfigurationApplied"),
			},
			{
				Type:   api.ConditionTypeHealth,
				Status: openapi.AdapterConditionStatusFalse,
				Reason: util.PtrString("Initializing"),
			},
			{
				Type:   api.ConditionTypeReady,
				Status: openapi.AdapterConditionStatusFalse,
				Reason: util.PtrString("Initializing"),
			},
		},
		&data1,
	)

	resp1, err := client.PostClusterStatusesWithResponse(
		ctx, cluster.ID,
		openapi.PostClusterStatusesJSONRequestBody(statusInput1), test.WithAuthToken(ctx),
	)
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
				Type:   api.ConditionTypeAvailable,
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("ClusterAvailable"),
			},
			{
				Type:   api.ConditionTypeApplied,
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("ConfigurationApplied"),
			},
			{
				Type:   api.ConditionTypeHealth,
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("HealthyCluster"),
			},
			{
				Type:   api.ConditionTypeReady,
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("AdapterReady"),
			},
		},
		&data2,
	)

	resp2, err := client.PostClusterStatusesWithResponse(
		ctx, cluster.ID,
		openapi.PostClusterStatusesJSONRequestBody(statusInput2), test.WithAuthToken(ctx),
	)
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
	Expect(finalStatus.Conditions[0].Status).
		To(Equal(openapi.AdapterConditionStatusTrue), "Conditions should be updated to latest")
}

// TestClusterStatusPost_UnknownReturns204 tests that status reports with Unknown
// mandatory conditions are always rejected (HYPERFLEET-657)
func TestClusterStatusPost_UnknownReturns204(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster first
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Create an adapter status with all mandatory conditions but Available=Unknown
	statusInput := newAdapterStatusRequest(
		"test-adapter-unknown",
		cluster.Generation,
		[]openapi.ConditionRequest{
			{
				Type:   api.ConditionTypeAvailable,
				Status: openapi.AdapterConditionStatusUnknown,
				Reason: util.PtrString("StartupPending"),
			},
			{
				Type:   api.ConditionTypeApplied,
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("ConfigurationApplied"),
			},
			{
				Type:   api.ConditionTypeHealth,
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("HealthyCluster"),
			},
		},
		nil,
	)

	// First report with Unknown mandatory condition: should be rejected (204 No Content)
	resp, err := client.PostClusterStatusesWithResponse(
		ctx, cluster.ID,
		openapi.PostClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred(), "Error posting cluster status: %v", err)
	Expect(resp.StatusCode()).
		To(Equal(http.StatusNoContent), "Expected 204 No Content for status with Unknown mandatory condition")

	// Verify no status was stored
	listResp, err := client.GetClusterStatusesWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(listResp.JSON200).NotTo(BeNil())

	found := false
	for _, s := range listResp.JSON200.Items {
		if s.Adapter == "test-adapter-unknown" {
			found = true
			break
		}
	}
	Expect(found).To(BeFalse(), "Status with Unknown mandatory condition should not be stored")

	// Subsequent report with same adapter: should also be rejected (204 No Content)
	resp2, err := client.PostClusterStatusesWithResponse(
		ctx, cluster.ID,
		openapi.PostClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred(), "Error posting cluster status: %v", err)
	Expect(resp2.StatusCode()).
		To(Equal(http.StatusNoContent), "Expected 204 No Content for subsequent Unknown status report")
}

// TestNodePoolStatusPost_UnknownReturns204 tests that status reports with Unknown
// mandatory conditions are always rejected (HYPERFLEET-657)
func TestNodePoolStatusPost_UnknownReturns204(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a nodepool (which also creates its parent cluster)
	nodePool, err := h.Factories.NewNodePools(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Create an adapter status with all mandatory conditions but Available=Unknown
	statusInput := newAdapterStatusRequest(
		"test-nodepool-adapter-unknown",
		1,
		[]openapi.ConditionRequest{
			{
				Type:   api.ConditionTypeAvailable,
				Status: openapi.AdapterConditionStatusUnknown,
				Reason: util.PtrString("StartupPending"),
			},
			{
				Type:   api.ConditionTypeApplied,
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("ConfigurationApplied"),
			},
			{
				Type:   api.ConditionTypeHealth,
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("HealthyCluster"),
			},
		},
		nil,
	)

	// First report with Unknown mandatory condition: should be rejected (204 No Content)
	resp, err := client.PostNodePoolStatusesWithResponse(
		ctx, nodePool.OwnerID, nodePool.ID,
		openapi.PostNodePoolStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred(), "Error posting nodepool status: %v", err)
	Expect(resp.StatusCode()).
		To(Equal(http.StatusNoContent), "Expected 204 No Content for status with Unknown mandatory condition")

	// Verify no status was stored
	listResp, err := client.GetNodePoolsStatusesWithResponse(
		ctx, nodePool.OwnerID, nodePool.ID, nil, test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(listResp.JSON200).NotTo(BeNil())

	found := false
	for _, s := range listResp.JSON200.Items {
		if s.Adapter == "test-nodepool-adapter-unknown" {
			found = true
			break
		}
	}
	Expect(found).To(BeFalse(), "Status with Unknown mandatory condition should not be stored")

	// Subsequent report with same adapter: should also be rejected (204 No Content)
	resp2, err := client.PostNodePoolStatusesWithResponse(
		ctx, nodePool.OwnerID, nodePool.ID,
		openapi.PostNodePoolStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred(), "Error posting nodepool status: %v", err)
	Expect(resp2.StatusCode()).
		To(Equal(http.StatusNoContent), "Expected 204 No Content for subsequent Unknown status report")
}

// TestClusterStatusPost_MultipleConditionsWithUnknownAvailable tests that
// Unknown Available is always rejected even among multiple conditions (HYPERFLEET-657)
func TestClusterStatusPost_MultipleConditionsWithUnknownAvailable(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster first
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Create an adapter status with all mandatory conditions but Available=Unknown
	statusInput := newAdapterStatusRequest(
		"test-adapter-multi-unknown",
		cluster.Generation,
		[]openapi.ConditionRequest{
			{
				Type:   api.ConditionTypeAvailable,
				Status: openapi.AdapterConditionStatusUnknown,
				Reason: util.PtrString("StartupPending"),
			},
			{
				Type:   api.ConditionTypeApplied,
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("ConfigurationApplied"),
			},
			{
				Type:   api.ConditionTypeHealth,
				Status: openapi.AdapterConditionStatusTrue,
				Reason: util.PtrString("HealthyCluster"),
			},
			{
				Type:   api.ConditionTypeReady,
				Status: openapi.AdapterConditionStatusTrue,
			},
			{
				Type:   "Progressing",
				Status: openapi.AdapterConditionStatusTrue,
			},
		},
		nil,
	)

	// First report with Unknown mandatory condition: should be rejected (204 No Content)
	resp, err := client.PostClusterStatusesWithResponse(
		ctx, cluster.ID,
		openapi.PostClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred(), "Error posting cluster status: %v", err)
	Expect(resp.StatusCode()).To(Equal(http.StatusNoContent),
		"Expected 204 No Content for first report with Available=Unknown among multiple conditions")

	// Subsequent report: should also be rejected (204 No Content)
	resp2, err := client.PostClusterStatusesWithResponse(
		ctx, cluster.ID,
		openapi.PostClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred(), "Error posting cluster status: %v", err)
	Expect(resp2.StatusCode()).To(Equal(http.StatusNoContent),
		"Expected 204 No Content for subsequent report with Available=Unknown among multiple conditions")
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
					Type:   api.ConditionTypeAvailable,
					Status: openapi.AdapterConditionStatusTrue,
				},
				{
					Type:   api.ConditionTypeApplied,
					Status: openapi.AdapterConditionStatusTrue,
				},
				{
					Type:   api.ConditionTypeHealth,
					Status: openapi.AdapterConditionStatusTrue,
				},
				{
					Type:   api.ConditionTypeReady,
					Status: openapi.AdapterConditionStatusTrue,
				},
			},
			nil,
		)
		_, err := client.PostClusterStatusesWithResponse(
			ctx, cluster.ID,
			openapi.PostClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
		)
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
				Type:   api.ConditionTypeAvailable,
				Status: openapi.AdapterConditionStatusTrue,
			},
			{
				Type:   api.ConditionTypeApplied,
				Status: openapi.AdapterConditionStatusTrue,
			},
			{
				Type:   api.ConditionTypeHealth,
				Status: openapi.AdapterConditionStatusTrue,
			},
			{
				Type:   api.ConditionTypeReady,
				Status: openapi.AdapterConditionStatusTrue,
			},
		},
		nil,
	)
	_, err = client.PostClusterStatusesWithResponse(
		ctx, singleCluster.ID,
		openapi.PostClusterStatusesJSONRequestBody(singleStatus), test.WithAuthToken(ctx),
	)
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
// TestHYPERFLEET657_MandatoryConditionsPreserved tests that adapter status updates
// without mandatory conditions are rejected and existing conditions are preserved
func TestHYPERFLEET657_MandatoryConditionsPreserved(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster first
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Send initial valid status with all mandatory conditions
	initialStatus := newAdapterStatusRequest(
		"adapter1",
		cluster.Generation,
		[]openapi.ConditionRequest{
			{
				Type:    api.ConditionTypeAvailable,
				Status:  openapi.AdapterConditionStatusFalse,
				Reason:  util.PtrString("ConfigMap data not yet available"),
				Message: util.PtrString("ConfigMap data not yet available"),
			},
			{
				Type:    api.ConditionTypeApplied,
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("ConfigMapApplied"),
				Message: util.PtrString("ConfigMap has been applied correctly"),
			},
			{
				Type:    api.ConditionTypeHealth,
				Status:  openapi.AdapterConditionStatusFalse,
				Reason:  util.PtrString("ConfigMap data not yet available"),
				Message: util.PtrString("ConfigMap data not yet available"),
			},
		},
		nil,
	)

	resp1, err := client.PostClusterStatusesWithResponse(
		ctx, cluster.ID,
		openapi.PostClusterStatusesJSONRequestBody(initialStatus), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp1.StatusCode()).To(Equal(http.StatusCreated))
	Expect(resp1.JSON201).ToNot(BeNil())
	Expect(len(resp1.JSON201.Conditions)).To(Equal(3))

	// Verify Available, Applied, Health are present
	conditionTypes := make(map[string]bool)
	for _, cond := range resp1.JSON201.Conditions {
		conditionTypes[cond.Type] = true
	}
	Expect(conditionTypes[api.ConditionTypeAvailable]).To(BeTrue())
	Expect(conditionTypes[api.ConditionTypeApplied]).To(BeTrue())
	Expect(conditionTypes[api.ConditionTypeHealth]).To(BeTrue())

	// Now send an incomplete update (missing mandatory conditions) - this is the bug scenario
	incompleteStatus := newAdapterStatusRequest(
		"adapter1",
		cluster.Generation,
		[]openapi.ConditionRequest{
			{
				Type:    "Available2",
				Status:  openapi.AdapterConditionStatusUnknown,
				Reason:  util.PtrString("ClusterProvisioned"),
				Message: util.PtrString("Cluster successfully provisioned"),
			},
			{
				Type:    "Health2",
				Status:  openapi.AdapterConditionStatusUnknown,
				Reason:  util.PtrString("ClusterProvisioned"),
				Message: util.PtrString("Cluster successfully provisioned"),
			},
		},
		&map[string]interface{}{
			"duration": "10m",
			"job_name": "provision-job-123",
		},
	)

	resp2, err := client.PostClusterStatusesWithResponse(
		ctx, cluster.ID,
		openapi.PostClusterStatusesJSONRequestBody(incompleteStatus), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	// Should return 204 No Content (update was discarded)
	Expect(resp2.StatusCode()).To(Equal(http.StatusNoContent))

	// Verify that the original conditions are preserved
	respGet, err := client.GetClusterStatusesWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(respGet.StatusCode()).To(Equal(http.StatusOK))
	Expect(respGet.JSON200).ToNot(BeNil())
	Expect(len(respGet.JSON200.Items)).To(Equal(1))

	// Verify Available, Applied, Health are still present (not replaced by Available2/Health2)
	storedConditionTypes := make(map[string]bool)
	for _, cond := range respGet.JSON200.Items[0].Conditions {
		storedConditionTypes[cond.Type] = true
	}
	Expect(storedConditionTypes[api.ConditionTypeAvailable]).To(BeTrue(), "Available condition should be preserved")
	Expect(storedConditionTypes[api.ConditionTypeApplied]).To(BeTrue(), "Applied condition should be preserved")
	Expect(storedConditionTypes[api.ConditionTypeHealth]).To(BeTrue(), "Health condition should be preserved")
	Expect(storedConditionTypes["Available2"]).To(BeFalse(), "Available2 should not be present")
	Expect(storedConditionTypes["Health2"]).To(BeFalse(), "Health2 should not be present")
}

// TestHYPERFLEET657_UnknownMandatoryConditionsRejected tests that updates with
// Unknown status for mandatory conditions are rejected
func TestHYPERFLEET657_UnknownMandatoryConditionsRejected(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster first
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Try to send status with all mandatory conditions but Available=Unknown
	statusWithUnknown := newAdapterStatusRequest(
		"adapter1",
		cluster.Generation,
		[]openapi.ConditionRequest{
			{
				Type:    api.ConditionTypeAvailable,
				Status:  openapi.AdapterConditionStatusUnknown,
				Reason:  util.PtrString("Checking"),
				Message: util.PtrString("Checking availability"),
			},
			{
				Type:    api.ConditionTypeApplied,
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("Applied"),
				Message: util.PtrString("Configuration applied"),
			},
			{
				Type:    api.ConditionTypeHealth,
				Status:  openapi.AdapterConditionStatusTrue,
				Reason:  util.PtrString("Healthy"),
				Message: util.PtrString("Cluster is healthy"),
			},
		},
		nil,
	)

	resp, err := client.PostClusterStatusesWithResponse(
		ctx, cluster.ID,
		openapi.PostClusterStatusesJSONRequestBody(statusWithUnknown), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	// Should return 204 No Content (update was discarded because Available=Unknown)
	Expect(resp.StatusCode()).To(Equal(http.StatusNoContent))

	// Verify no status was stored
	respGet, err := client.GetClusterStatusesWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(respGet.StatusCode()).To(Equal(http.StatusOK))
	Expect(respGet.JSON200).ToNot(BeNil())
	Expect(len(respGet.JSON200.Items)).To(Equal(0), "No status should be stored when mandatory condition is Unknown")
}
