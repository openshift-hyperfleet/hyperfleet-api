package integration

import (
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
	"gopkg.in/resty.v1"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

// TestNodePoolGet is disabled because GET /nodepools/{id} is not in the OpenAPI spec
// The API only supports:
// - GET /api/hyperfleet/v1/nodepools (list all nodepools)
// - GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools (list nodepools by cluster)
// - POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools (create nodepool)
// func TestNodePoolGet(t *testing.T) {
// 	h, client := test.RegisterIntegration(t)
//
// 	account := h.NewRandAccount()
// 	ctx := h.NewAuthenticatedContext(account)
//
// 	// 401 using no JWT token
// 	_, _, err := client.DefaultAPI.GetNodePoolById(context.Background(), "foo").Execute()
// 	Expect(err).To(HaveOccurred(), "Expected 401 but got nil error")
//
// 	// GET responses per openapi spec: 200 and 404,
// 	_, resp, err := client.DefaultAPI.GetNodePoolById(ctx, "foo").Execute()
// 	Expect(err).To(HaveOccurred(), "Expected 404")
// 	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
//
// 	nodePoolModel, err := h.Factories.NewNodePools(h.NewID())
// 	Expect(err).NotTo(HaveOccurred())
//
// 	nodePoolOutput, resp, err := client.DefaultAPI.GetNodePoolById(ctx, nodePoolModel.ID).Execute()
// 	Expect(err).NotTo(HaveOccurred())
// 	Expect(resp.StatusCode).To(Equal(http.StatusOK))
//
// 	Expect(*nodePoolOutput.Id).To(Equal(nodePoolModel.ID), "found object does not match test object")
// 	Expect(*nodePoolOutput.Kind).To(Equal("NodePool"))
// 	Expect(*nodePoolOutput.Href).To(Equal(fmt.Sprintf("/api/hyperfleet/v1/node_pools/%s", nodePoolModel.ID)))
// 	Expect(nodePoolOutput.CreatedAt).To(BeTemporally("~", nodePoolModel.CreatedAt))
// 	Expect(nodePoolOutput.UpdatedAt).To(BeTemporally("~", nodePoolModel.UpdatedAt))
// }

func TestNodePoolPost(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a parent cluster first
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// POST responses per openapi spec: 201, 409, 500
	nodePoolInput := openapi.NodePoolCreateRequest{
		Name: "test-name",
		Spec: map[string]interface{}{"test": "spec"},
	}

	// 201 Created
	nodePoolOutput, resp, err := client.DefaultAPI.CreateNodePool(ctx, cluster.ID).NodePoolCreateRequest(nodePoolInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(*nodePoolOutput.Id).NotTo(BeEmpty(), "Expected ID assigned on creation")
	Expect(*nodePoolOutput.Kind).To(Equal("NodePool"))
	Expect(*nodePoolOutput.Href).To(Equal(fmt.Sprintf("/api/hyperfleet/v1/clusters/%s/nodepools/%s", cluster.ID, *nodePoolOutput.Id)))

	// 400 bad request. posting junk json is one way to trigger 400.
	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Post(h.RestURL(fmt.Sprintf("/clusters/%s/nodepools", cluster.ID)))

	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

// TestNodePoolPatch is disabled because PATCH endpoints are not implemented
// func TestNodePoolPatch(t *testing.T) {
// 	// PATCH not implemented in current API
// }

func TestNodePoolPaging(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Paging
	_, err := h.Factories.NewNodePoolsList("Bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	list, _, err := client.DefaultAPI.GetNodePools(ctx).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting nodePool list: %v", err)
	Expect(len(list.Items)).To(Equal(20))
	Expect(list.Size).To(Equal(int32(20)))
	Expect(list.Total).To(Equal(int32(20)))
	Expect(list.Page).To(Equal(int32(1)))

	list, _, err = client.DefaultAPI.GetNodePools(ctx).Page(2).PageSize(5).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting nodePool list: %v", err)
	Expect(len(list.Items)).To(Equal(5))
	Expect(list.Size).To(Equal(int32(5)))
	Expect(list.Total).To(Equal(int32(20)))
	Expect(list.Page).To(Equal(int32(2)))
}

func TestNodePoolListSearch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	nodePools, err := h.Factories.NewNodePoolsList("bronto", 20)
	Expect(err).NotTo(HaveOccurred(), "Error creating test nodepools: %v", err)

	search := fmt.Sprintf("id in ('%s')", nodePools[0].ID)
	list, _, err := client.DefaultAPI.GetNodePools(ctx).Search(search).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting nodePool list: %v", err)
	Expect(len(list.Items)).To(Equal(1))
	Expect(list.Total).To(Equal(int32(1)))
	Expect(*list.Items[0].Id).To(Equal(nodePools[0].ID))
}

func TestNodePoolsByClusterId(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster first
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Create nodepools for this cluster
	// Note: In a real implementation, nodepools would be associated with the cluster
	// For now, we're just creating nodepools and testing the endpoint exists
	_, err = h.Factories.NewNodePoolsList("cluster-nodepools", 5)
	Expect(err).NotTo(HaveOccurred())

	// Get nodepools by cluster ID
	list, resp, err := client.DefaultAPI.GetNodePoolsByClusterId(ctx, cluster.ID).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting nodepools by cluster ID: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(list).NotTo(BeNil())
	// The list might be empty if nodepools aren't properly associated with the cluster
	// but the endpoint should work
}

func TestGetNodePoolByClusterIdAndNodePoolId(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster first
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Create a nodepool for this cluster using the API
	nodePoolInput := openapi.NodePoolCreateRequest{
		Name: "test-nodepool-get",
		Spec: map[string]interface{}{"instance_type": "m5.large", "replicas": 2},
	}

	nodePoolOutput, resp, err := client.DefaultAPI.CreateNodePool(ctx, cluster.ID).NodePoolCreateRequest(nodePoolInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error creating nodepool: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(*nodePoolOutput.Id).NotTo(BeEmpty())

	nodePoolID := *nodePoolOutput.Id

	// Test 1: Get the nodepool by cluster ID and nodepool ID (200 OK)
	retrieved, resp, err := client.DefaultAPI.GetNodePoolById(ctx, cluster.ID, nodePoolID).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting nodepool by cluster and nodepool ID: %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(*retrieved.Id).To(Equal(nodePoolID), "Retrieved nodepool ID should match")
	Expect(*retrieved.Kind).To(Equal("NodePool"))
	Expect(retrieved.Name).To(Equal("test-nodepool-get"))

	// Test 2: Try to get with non-existent nodepool ID (404)
	_, resp, err = client.DefaultAPI.GetNodePoolById(ctx, cluster.ID, "non-existent-id").Execute()
	Expect(err).To(HaveOccurred(), "Expected 404 for non-existent nodepool")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

	// Test 3: Try to get with non-existent cluster ID (404)
	_, resp, err = client.DefaultAPI.GetNodePoolById(ctx, "non-existent-cluster", nodePoolID).Execute()
	Expect(err).To(HaveOccurred(), "Expected 404 for non-existent cluster")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

	// Test 4: Create another cluster and verify that nodepool is not accessible from wrong cluster
	cluster2, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	_, resp, err = client.DefaultAPI.GetNodePoolById(ctx, cluster2.ID, nodePoolID).Execute()
	Expect(err).To(HaveOccurred(), "Expected 404 when accessing nodepool from wrong cluster")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))
}
