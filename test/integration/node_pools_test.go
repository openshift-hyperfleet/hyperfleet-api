package integration

import (
	"encoding/json"
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
	kind := "NodePool"
	nodePoolInput := openapi.NodePoolCreateRequest{
		Kind: &kind,
		Name: "test-name",
		Spec: map[string]interface{}{"test": "spec"},
	}

	// 201 Created
	resp, err := client.CreateNodePoolWithResponse(
		ctx, cluster.ID, openapi.CreateNodePoolJSONRequestBody(nodePoolInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated))

	nodePoolOutput := resp.JSON201
	Expect(nodePoolOutput).NotTo(BeNil())
	Expect(*nodePoolOutput.Id).NotTo(BeEmpty(), "Expected ID assigned on creation")
	Expect(*nodePoolOutput.Kind).To(Equal("NodePool"))
	Expect(*nodePoolOutput.Href).
		To(Equal(fmt.Sprintf("/api/hyperfleet/v1/clusters/%s/nodepools/%s", cluster.ID, *nodePoolOutput.Id)))

	// 400 bad request. posting junk json is one way to trigger 400.
	jwtToken := test.GetAccessTokenFromContext(ctx)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Post(h.RestURL(fmt.Sprintf("/clusters/%s/nodepools", cluster.ID)))

	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
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

	resp, err := client.GetNodePoolsWithResponse(ctx, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Error getting nodePool list: %v", err)
	list := resp.JSON200
	Expect(list).NotTo(BeNil())
	Expect(len(list.Items)).To(Equal(20))
	Expect(list.Size).To(Equal(int32(20)))
	Expect(list.Total).To(Equal(int32(20)))
	Expect(list.Page).To(Equal(int32(1)))

	page := openapi.QueryParamsPage(2)
	pageSize := openapi.QueryParamsPageSize(5)
	params := &openapi.GetNodePoolsParams{
		Page:     &page,
		PageSize: &pageSize,
	}
	resp, err = client.GetNodePoolsWithResponse(ctx, params, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Error getting nodePool list: %v", err)
	list = resp.JSON200
	Expect(list).NotTo(BeNil())
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

	searchStr := fmt.Sprintf("id in ('%s')", nodePools[0].ID)
	search := openapi.SearchParams(searchStr)
	params := &openapi.GetNodePoolsParams{
		Search: &search,
	}
	resp, err := client.GetNodePoolsWithResponse(ctx, params, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Error getting nodePool list: %v", err)
	list := resp.JSON200
	Expect(list).NotTo(BeNil())
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
	resp, err := client.GetNodePoolsByClusterIdWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Error getting nodepools by cluster ID: %v", err)
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))
	Expect(resp.JSON200).NotTo(BeNil())
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
	kind := "NodePool"
	nodePoolInput := openapi.NodePoolCreateRequest{
		Kind: &kind,
		Name: "test-np-get",
		Spec: map[string]interface{}{"instance_type": "m5.large", "replicas": 2},
	}

	createResp, err := client.CreateNodePoolWithResponse(
		ctx, cluster.ID, openapi.CreateNodePoolJSONRequestBody(nodePoolInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred(), "Error creating nodepool: %v", err)
	Expect(createResp.StatusCode()).To(Equal(http.StatusCreated))
	Expect(*createResp.JSON201.Id).NotTo(BeEmpty())

	nodePoolID := *createResp.JSON201.Id

	// Test 1: Get the nodepool by cluster ID and nodepool ID (200 OK)
	getResp, err := client.GetNodePoolByIdWithResponse(ctx, cluster.ID, nodePoolID, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Error getting nodepool by cluster and nodepool ID: %v", err)
	Expect(getResp.StatusCode()).To(Equal(http.StatusOK))
	retrieved := getResp.JSON200
	Expect(retrieved).NotTo(BeNil())
	Expect(*retrieved.Id).To(Equal(nodePoolID), "Retrieved nodepool ID should match")
	Expect(*retrieved.Kind).To(Equal("NodePool"))
	Expect(retrieved.Name).To(Equal("test-np-get"))

	// Test 2: Try to get with non-existent nodepool ID (404)
	notFoundResp, err := client.GetNodePoolByIdWithResponse(ctx, cluster.ID, "non-existent-id", test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(notFoundResp.StatusCode()).
		To(Equal(http.StatusNotFound), "Expected 404 for non-existent nodepool")

	// Test 3: Try to get with non-existent cluster ID (404)
	notFoundResp, err = client.GetNodePoolByIdWithResponse(
		ctx, "non-existent-cluster", nodePoolID, test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(notFoundResp.StatusCode()).
		To(Equal(http.StatusNotFound), "Expected 404 for non-existent cluster")

	// Test 4: Create another cluster and verify that nodepool is not accessible from wrong cluster
	cluster2, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	wrongClusterResp, err := client.GetNodePoolByIdWithResponse(
		ctx, cluster2.ID, nodePoolID, test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(wrongClusterResp.StatusCode()).To(Equal(http.StatusNotFound),
		"Expected 404 when accessing nodepool from wrong cluster")
}

// TestNodePoolPost_EmptyKind tests that empty kind field returns 400
func TestNodePoolPost_EmptyKind(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	// Create a cluster first
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Send request with empty kind
	invalidInput := `{
		"kind": "",
		"name": "test-nodepool",
		"spec": {}
	}`

	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidInput).
		Post(h.RestURL(fmt.Sprintf("/clusters/%s/nodepools", cluster.ID)))

	Expect(err).ToNot(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))

	// Parse error response
	var errorResponse map[string]interface{}
	err = json.Unmarshal(restyResp.Body(), &errorResponse)
	Expect(err).ToNot(HaveOccurred())

	// Verify error message contains "kind is required" (RFC 9457 uses "detail" field)
	detail, ok := errorResponse["detail"].(string)
	Expect(ok).To(BeTrue())
	Expect(detail).To(ContainSubstring("kind is required"))
}

// TestNodePoolPost_WrongKind tests that wrong kind field returns 400
func TestNodePoolPost_WrongKind(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	// Create a cluster first
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Send request with wrong kind
	invalidInput := `{
		"kind": "Cluster",
		"name": "test-nodepool",
		"spec": {}
	}`

	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidInput).
		Post(h.RestURL(fmt.Sprintf("/clusters/%s/nodepools", cluster.ID)))

	Expect(err).ToNot(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))

	// Parse error response
	var errorResponse map[string]interface{}
	err = json.Unmarshal(restyResp.Body(), &errorResponse)
	Expect(err).ToNot(HaveOccurred())

	// Verify error message contains "kind must be 'NodePool'" (RFC 9457 uses "detail" field)
	detail, ok := errorResponse["detail"].(string)
	Expect(ok).To(BeTrue())
	Expect(detail).To(ContainSubstring("kind must be 'NodePool'"))
}
