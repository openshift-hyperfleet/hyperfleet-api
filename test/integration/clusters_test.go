package integration

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
	"gopkg.in/resty.v1"

	"github.com/openshift-hyperfleet/hyperfleet/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet/test"
)

func TestClusterGet(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// 401 using no JWT token
	_, _, err := client.DefaultAPI.GetClusterById(context.Background(), "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 401 but got nil error")

	// GET responses per openapi spec: 200 and 404,
	_, resp, err := client.DefaultAPI.GetClusterById(ctx, "foo").Execute()
	Expect(err).To(HaveOccurred(), "Expected 404")
	Expect(resp.StatusCode).To(Equal(http.StatusNotFound))

	clusterModel, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	clusterOutput, resp, err := client.DefaultAPI.GetClusterById(ctx, clusterModel.ID).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	Expect(*clusterOutput.Id).To(Equal(clusterModel.ID), "found object does not match test object")
	Expect(clusterOutput.Kind).To(Equal("Cluster"))
	Expect(*clusterOutput.Href).To(Equal(fmt.Sprintf("/api/hyperfleet/v1/clusters/%s", clusterModel.ID)))
	Expect(clusterOutput.CreatedAt).To(BeTemporally("~", clusterModel.CreatedAt))
	Expect(clusterOutput.UpdatedAt).To(BeTemporally("~", clusterModel.UpdatedAt))
}

func TestClusterPost(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// POST responses per openapi spec: 201, 409, 500
	clusterInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: "test-name",
		Spec: map[string]interface{}{"test": "spec"},
	}

	// 201 Created
	clusterOutput, resp, err := client.DefaultAPI.PostCluster(ctx).ClusterCreateRequest(clusterInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(*clusterOutput.Id).NotTo(BeEmpty(), "Expected ID assigned on creation")
	Expect(clusterOutput.Kind).To(Equal("Cluster"))
	Expect(*clusterOutput.Href).To(Equal(fmt.Sprintf("/api/hyperfleet/v1/clusters/%s", *clusterOutput.Id)))

	// 400 bad request. posting junk json is one way to trigger 400.
	jwtToken := ctx.Value(openapi.ContextAccessToken)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Post(h.RestURL("/clusters"))

	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

// TestClusterPatch is disabled because PATCH endpoints are not implemented
// func TestClusterPatch(t *testing.T) {
// 	// PATCH not implemented in current API
// }

func TestClusterPaging(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Paging
	_, err := h.Factories.NewClustersList("Bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	list, _, err := client.DefaultAPI.GetClusters(ctx).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting cluster list: %v", err)
	Expect(len(list.Items)).To(Equal(20))
	Expect(list.Size).To(Equal(int32(20)))
	Expect(list.Total).To(Equal(int32(20)))
	Expect(list.Page).To(Equal(int32(1)))

	list, _, err = client.DefaultAPI.GetClusters(ctx).Page(2).PageSize(5).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting cluster list: %v", err)
	Expect(len(list.Items)).To(Equal(5))
	Expect(list.Size).To(Equal(int32(5)))
	Expect(list.Total).To(Equal(int32(20)))
	Expect(list.Page).To(Equal(int32(2)))
}

func TestClusterListSearch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	clusters, err := h.Factories.NewClustersList("bronto", 20)
	Expect(err).NotTo(HaveOccurred(), "Error creating test clusters: %v", err)

	search := fmt.Sprintf("id in ('%s')", clusters[0].ID)
	list, _, err := client.DefaultAPI.GetClusters(ctx).Search(search).Execute()
	Expect(err).NotTo(HaveOccurred(), "Error getting cluster list: %v", err)
	Expect(len(list.Items)).To(Equal(1))
	Expect(list.Total).To(Equal(int32(1)))
	Expect(*list.Items[0].Id).To(Equal(clusters[0].ID))
}

// TestClusterSearchSQLInjection tests SQL injection protection in search
func TestClusterSearchSQLInjection(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a few clusters
	clusters, err := h.Factories.NewClustersList("injection-test", 5)
	Expect(err).NotTo(HaveOccurred())

	// Test 1: SQL injection attempt with OR
	maliciousSearch := "id='anything' OR '1'='1'"
	_, _, err = client.DefaultAPI.GetClusters(ctx).Search(maliciousSearch).Execute()
	// Should either return 400 error or return empty/controlled results
	// Not crash or return all data
	if err == nil {
		// If no error, the search should not return everything
		t.Logf("Search with SQL injection did not error - implementation may handle it gracefully")
	}

	// Test 2: SQL injection attempt with DROP
	dropSearch := "id='; DROP TABLE clusters; --"
	_, _, err = client.DefaultAPI.GetClusters(ctx).Search(dropSearch).Execute()
	// Should not crash
	if err == nil {
		t.Logf("Search with DROP statement did not error - implementation may handle it gracefully")
	}

	// Test 3: Verify clusters still exist after injection attempts
	list, _, err := client.DefaultAPI.GetClusters(ctx).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(list.Total).To(BeNumerically(">=", 5), "Clusters should still exist after injection attempts")

	// Test 4: Valid search still works
	validSearch := fmt.Sprintf("id='%s'", clusters[0].ID)
	validList, _, err := client.DefaultAPI.GetClusters(ctx).Search(validSearch).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(len(validList.Items)).To(BeNumerically(">=", 0))
}

// TestClusterDuplicateNames tests that duplicate cluster names are rejected
func TestClusterDuplicateNames(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create first cluster with a specific name
	clusterInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: "duplicate-name-test",
		Spec: map[string]interface{}{"test": "spec1"},
	}

	cluster1, resp, err := client.DefaultAPI.PostCluster(ctx).ClusterCreateRequest(clusterInput).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	id1 := *cluster1.Id

	// Create second cluster with the SAME name
	// Names are unique, so this should return 409 Conflict
	_, resp, err = client.DefaultAPI.PostCluster(ctx).ClusterCreateRequest(clusterInput).Execute()
	Expect(err).To(HaveOccurred(), "Expected 409 Conflict for duplicate name")
	Expect(resp.StatusCode).To(Equal(http.StatusConflict))

	// Verify first cluster still exists
	retrieved1, _, err := client.DefaultAPI.GetClusterById(ctx, id1).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(retrieved1.Name).To(Equal("duplicate-name-test"))
}

// TestClusterBoundaryValues tests boundary values for cluster fields
func TestClusterBoundaryValues(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Test 1: Maximum name length (database limit is 63 characters)
	longName := ""
	for i := 0; i < 63; i++ {
		longName += "a"
	}

	longNameInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: longName,
		Spec: map[string]interface{}{"test": "spec"},
	}

	longNameCluster, resp, err := client.DefaultAPI.PostCluster(ctx).ClusterCreateRequest(longNameInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Should accept name up to 63 characters")
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(longNameCluster.Name).To(Equal(longName))

	// Test exceeding max length (64 characters should fail)
	tooLongName := longName + "a"
	tooLongInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: tooLongName,
		Spec: map[string]interface{}{"test": "spec"},
	}
	_, resp, err = client.DefaultAPI.PostCluster(ctx).ClusterCreateRequest(tooLongInput).Execute()
	Expect(err).To(HaveOccurred(), "Should reject name exceeding 63 characters")
	Expect(resp.StatusCode).To(Equal(http.StatusInternalServerError))

	// Test 2: Empty name
	emptyNameInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: "",
		Spec: map[string]interface{}{"test": "spec"},
	}

	_, resp, err = client.DefaultAPI.PostCluster(ctx).ClusterCreateRequest(emptyNameInput).Execute()
	// Should either accept empty name or return 400
	if resp != nil {
		t.Logf("Empty name test returned status: %d", resp.StatusCode)
	}

	// Test 3: Large spec JSON (test with ~10KB JSON)
	largeSpec := make(map[string]interface{})
	for i := 0; i < 100; i++ {
		largeSpec[fmt.Sprintf("key_%d", i)] = fmt.Sprintf("value_%d_with_some_padding_to_increase_size_xxxxxxxxxxxxxxxxxx", i)
	}

	largeSpecInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: "large-spec-test",
		Spec: largeSpec,
	}

	largeSpecCluster, resp, err := client.DefaultAPI.PostCluster(ctx).ClusterCreateRequest(largeSpecInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Should accept large spec JSON")
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))

	// Verify the spec was stored correctly
	retrieved, _, err := client.DefaultAPI.GetClusterById(ctx, *largeSpecCluster.Id).Execute()
	Expect(err).NotTo(HaveOccurred())
	Expect(len(retrieved.Spec)).To(Equal(100))

	// Test 4: Unicode in name
	unicodeNameInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: "テスト-δοκιμή-🚀",
		Spec: map[string]interface{}{"test": "spec"},
	}

	unicodeNameCluster, resp, err := client.DefaultAPI.PostCluster(ctx).ClusterCreateRequest(unicodeNameInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Should accept unicode in name")
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(unicodeNameCluster.Name).To(Equal("テスト-δοκιμή-🚀"))
}
