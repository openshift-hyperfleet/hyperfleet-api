package integration

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet/test"
)

// TestControllerBasicAuth tests that the API requires authentication
func TestControllerBasicAuth(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	_ = h.NewAuthenticatedContext(account)

	// Test that endpoints require authentication
	// This is a basic test to ensure the controller framework is working
	_, _, err := client.DefaultAPI.GetClusters(nil).Execute()
	Expect(err).To(HaveOccurred(), "Expected error for unauthenticated request")
}

// TestControllerClusterLifecycle tests basic CRUD operations through the controller
func TestControllerClusterLifecycle(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster
	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Retrieve the cluster
	retrieved, _, err := client.DefaultAPI.GetClusterById(ctx, cluster.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(*retrieved.Id).To(Equal(cluster.ID))
	Expect(retrieved.Kind).To(Equal("Cluster"))
}

// TestControllerNodePoolLifecycle is disabled because GET /nodepools/{id} is not in the OpenAPI spec
// The API only supports:
// - GET /api/hyperfleet/v1/nodepools (list all nodepools)
// - GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools (list nodepools by cluster)
// - POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools (create nodepool)
// func TestControllerNodePoolLifecycle(t *testing.T) {
// 	h, client := test.RegisterIntegration(t)
//
// 	account := h.NewRandAccount()
// 	ctx := h.NewAuthenticatedContext(account)
//
// 	// Create a cluster first (NodePools need a parent cluster)
// 	_, err := h.Factories.NewClusters(h.NewID())
// 	Expect(err).NotTo(HaveOccurred())
//
// 	// Create a nodepool
// 	nodePool, err := h.Factories.NewNodePools(h.NewID())
// 	Expect(err).NotTo(HaveOccurred())
//
// 	// Retrieve the nodepool
// 	retrieved, _, err := client.DefaultAPI.GetNodePoolById(ctx, nodePool.ID).Execute()
// 	Expect(err).NotTo(HaveOccurred())
// 	Expect(*retrieved.Id).To(Equal(nodePool.ID))
// 	Expect(retrieved.Kind).To(Equal("NodePool"))
// }
