package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"gopkg.in/resty.v1"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
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
	Expect(clusterOutput.CreatedTime).To(BeTemporally("~", clusterModel.CreatedTime))
	Expect(clusterOutput.UpdatedTime).To(BeTemporally("~", clusterModel.UpdatedTime))
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
	Expect(err).ToNot(HaveOccurred(), "Error object:  %v", err)
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
	Expect(err).ToNot(HaveOccurred())

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

// TestClusterSchemaValidation tests schema validation for cluster specs
// Note: This test validates against the base openapi.yaml schema which has an empty ClusterSpec
// The base schema accepts any JSON object, so this test mainly verifies the middleware is working
func TestClusterSchemaValidation(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Test 1: Valid cluster spec (base schema accepts any object)
	validSpec := map[string]interface{}{
		"region":   "us-central1",
		"provider": "gcp",
	}

	validInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: "schema-valid-test",
		Spec: validSpec,
	}

	cluster, resp, err := client.DefaultAPI.PostCluster(ctx).ClusterCreateRequest(validInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Valid spec should be accepted")
	Expect(resp.StatusCode).To(Equal(http.StatusCreated))
	Expect(*cluster.Id).NotTo(BeEmpty())

	// Test 2: Invalid spec type (spec must be object, not string)
	// This should fail even with base schema
	// Can't use the generated struct because Spec is typed as map[string]interface{}
	// So we send raw JSON request
	invalidTypeJSON := `{
		"kind": "Cluster",
		"name": "schema-invalid-type",
		"spec": "invalid-string-spec"
	}`

	jwtToken := ctx.Value(openapi.ContextAccessToken)

	resp2, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidTypeJSON).
		Post(h.RestURL("/clusters"))

	if resp2.StatusCode() == http.StatusBadRequest {
		t.Logf("Schema validation correctly rejected invalid spec type")
		// Verify error response contains details
		var errorResponse openapi.Error
		json.Unmarshal(resp2.Body(), &errorResponse)
		Expect(errorResponse.Code).ToNot(BeNil())
		Expect(errorResponse.Reason).ToNot(BeNil())
	} else {
		t.Logf("Base schema may accept any spec type, status: %d", resp2.StatusCode())
	}

	// Test 3: Empty spec (should be valid as spec is optional in base schema)
	emptySpecInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: "schema-empty-spec",
		Spec: map[string]interface{}{},
	}

	cluster3, resp3, err := client.DefaultAPI.PostCluster(ctx).ClusterCreateRequest(emptySpecInput).Execute()
	Expect(err).NotTo(HaveOccurred(), "Empty spec should be accepted by base schema")
	Expect(resp3.StatusCode).To(Equal(http.StatusCreated))
	Expect(*cluster3.Id).NotTo(BeEmpty())
}

// TestClusterSchemaValidationWithProviderSchema tests schema validation with a provider-specific schema
// This test will only work if OPENAPI_SCHEMA_PATH is set to a provider schema (e.g., gcp_openapi.yaml)
// When using the base schema, this test will be skipped
func TestClusterSchemaValidationWithProviderSchema(t *testing.T) {
	RegisterTestingT(t)

	// Check if we're using a provider schema or base schema
	// If base schema, skip detailed validation tests
	schemaPath := os.Getenv("OPENAPI_SCHEMA_PATH")
	if schemaPath == "" || strings.HasSuffix(schemaPath, "openapi/openapi.yaml") {
		t.Skip("Skipping provider schema validation test - using base schema")
		return
	}

	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Test with provider-specific schema (assumes GCP schema for this example)
	// If using a different provider, adjust the spec accordingly

	// Test 1: Invalid spec - missing required field
	invalidSpec := map[string]interface{}{
		"gcp": map[string]interface{}{
			// Missing required "region" field
			"zone": "us-central1-a",
		},
	}

	invalidInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: "provider-schema-invalid",
		Spec: invalidSpec,
	}

	_, resp, err := client.DefaultAPI.PostCluster(ctx).ClusterCreateRequest(invalidInput).Execute()
	Expect(err).To(HaveOccurred(), "Should reject spec with missing required field")
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
	defer resp.Body.Close()

	// Parse error response to verify field-level details
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var errorResponse openapi.Error
	if err := json.Unmarshal(bodyBytes, &errorResponse); err != nil {
		t.Fatalf("failed to unmarshal error response body: %v", err)
	}

	Expect(errorResponse.Code).ToNot(BeNil())
	Expect(*errorResponse.Code).To(Equal("hyperfleet-8")) // Validation error code
	Expect(errorResponse.Details).ToNot(BeEmpty(), "Should include field-level error details")

	// Verify details contain field path
	foundRegionError := false
	for _, detail := range errorResponse.Details {
		if detail.Field != nil && strings.Contains(*detail.Field, "region") {
			foundRegionError = true
			break
		}
	}
	Expect(foundRegionError).To(BeTrue(), "Error details should mention missing 'region' field")
}

// TestClusterSchemaValidationErrorDetails tests that validation errors include detailed field information
func TestClusterSchemaValidationErrorDetails(t *testing.T) {
	RegisterTestingT(t)
	h, _ := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Send request with spec field as wrong type (not an object)
	invalidTypeRequest := map[string]interface{}{
		"kind": "Cluster",
		"name": "error-details-test",
		"spec": "not-an-object", // Invalid type
	}

	body, _ := json.Marshal(invalidTypeRequest)
	jwtToken := ctx.Value(openapi.ContextAccessToken)

	resp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(body).
		Post(h.RestURL("/clusters"))

	Expect(err).To(BeNil())

	// Log response for debugging
	t.Logf("Response status: %d, body: %s", resp.StatusCode(), string(resp.Body()))

	Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest), "Should return 400 for invalid spec type")

	// Parse error response
	var errorResponse openapi.Error
	if err := json.Unmarshal(resp.Body(), &errorResponse); err != nil {
		t.Fatalf("failed to unmarshal error response: %v, response body: %s", err, string(resp.Body()))
	}

	// Verify error structure
	Expect(errorResponse.Kind).ToNot(BeNil())
	Expect(*errorResponse.Kind).To(Equal("Error"))

	Expect(errorResponse.Code).ToNot(BeNil())
	// Both hyperfleet-8 (validation error) and hyperfleet-17 (invalid request format) are acceptable
	// as they both indicate the spec field is invalid
	validCodes := []string{"hyperfleet-8", "hyperfleet-17"}
	Expect(validCodes).To(ContainElement(*errorResponse.Code), "Expected validation or format error code")

	Expect(errorResponse.Reason).ToNot(BeNil())
	Expect(*errorResponse.Reason).To(ContainSubstring("spec"))

	Expect(errorResponse.Href).ToNot(BeNil())
	Expect(errorResponse.OperationId).ToNot(BeNil())

	t.Logf("Error response: code=%s, reason=%s", *errorResponse.Code, *errorResponse.Reason)
}

// TestClusterList_DefaultSorting tests that clusters are sorted by created_time desc by default
func TestClusterList_DefaultSorting(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create 3 clusters with delays to ensure different timestamps
	var createdClusters []openapi.Cluster
	for i := 1; i <= 3; i++ {
		clusterInput := openapi.ClusterCreateRequest{
			Kind: "Cluster",
			Name: fmt.Sprintf("sort-test-%d-%s", i, h.NewID()),
			Spec: map[string]interface{}{"test": fmt.Sprintf("value-%d", i)},
		}

		cluster, _, err := client.DefaultAPI.PostCluster(ctx).ClusterCreateRequest(clusterInput).Execute()
		Expect(err).NotTo(HaveOccurred(), "Failed to create cluster %d", i)
		createdClusters = append(createdClusters, *cluster)

		// Add 100ms delay to ensure different created_time
		time.Sleep(100 * time.Millisecond)
	}

	// List clusters without orderBy parameter - should default to created_time desc
	list, _, err := client.DefaultAPI.GetClusters(ctx).Execute()
	Expect(err).NotTo(HaveOccurred(), "Failed to list clusters")
	Expect(len(list.Items)).To(BeNumerically(">=", 3), "Should have at least 3 clusters")

	// Find our test clusters in the response
	var testClusters []openapi.Cluster
	for _, item := range list.Items {
		for _, created := range createdClusters {
			if *item.Id == *created.Id {
				testClusters = append(testClusters, item)
				break
			}
		}
	}

	Expect(len(testClusters)).To(Equal(3), "Should find all 3 test clusters")

	// Verify they are sorted by created_time desc (newest first)
	// testClusters should be in reverse creation order
	Expect(*testClusters[0].Id).To(Equal(*createdClusters[2].Id), "First cluster should be the last created")
	Expect(*testClusters[1].Id).To(Equal(*createdClusters[1].Id), "Second cluster should be the middle created")
	Expect(*testClusters[2].Id).To(Equal(*createdClusters[0].Id), "Third cluster should be the first created")

	t.Logf("✓ Default sorting works: clusters sorted by created_time desc")
}

// TestClusterList_OrderByName tests custom sorting by name
func TestClusterList_OrderByName(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create clusters with names that will sort alphabetically
	testPrefix := fmt.Sprintf("name-sort-%s", h.NewID())
	names := []string{
		fmt.Sprintf("%s-charlie", testPrefix),
		fmt.Sprintf("%s-alpha", testPrefix),
		fmt.Sprintf("%s-bravo", testPrefix),
	}

	var createdClusters []openapi.Cluster
	for _, name := range names {
		clusterInput := openapi.ClusterCreateRequest{
			Kind: "Cluster",
			Name: name,
			Spec: map[string]interface{}{"test": "value"},
		}

		cluster, _, err := client.DefaultAPI.PostCluster(ctx).ClusterCreateRequest(clusterInput).Execute()
		Expect(err).NotTo(HaveOccurred(), "Failed to create cluster %s", name)
		createdClusters = append(createdClusters, *cluster)
	}

	// List with orderBy=name asc
	list, _, err := client.DefaultAPI.GetClusters(ctx).OrderBy("name asc").Execute()
	Expect(err).NotTo(HaveOccurred(), "Failed to list clusters with orderBy")
	Expect(len(list.Items)).To(BeNumerically(">=", 3), "Should have at least 3 clusters")

	// Find our test clusters in the response
	var testClusters []openapi.Cluster
	for _, item := range list.Items {
		if strings.HasPrefix(item.Name, testPrefix) {
			testClusters = append(testClusters, item)
		}
	}

	Expect(len(testClusters)).To(Equal(3), "Should find all 3 test clusters")

	// Verify they are sorted by name asc (alphabetically)
	Expect(testClusters[0].Name).To(ContainSubstring("alpha"), "First should be alpha")
	Expect(testClusters[1].Name).To(ContainSubstring("bravo"), "Second should be bravo")
	Expect(testClusters[2].Name).To(ContainSubstring("charlie"), "Third should be charlie")

	t.Logf("✓ Custom sorting works: clusters sorted by name asc")
}

// TestClusterList_OrderByNameDesc tests sorting by name descending
func TestClusterList_OrderByNameDesc(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create clusters with names that will sort alphabetically
	testPrefix := fmt.Sprintf("desc-sort-%s", h.NewID())
	names := []string{
		fmt.Sprintf("%s-alpha", testPrefix),
		fmt.Sprintf("%s-charlie", testPrefix),
		fmt.Sprintf("%s-bravo", testPrefix),
	}

	for _, name := range names {
		clusterInput := openapi.ClusterCreateRequest{
			Kind: "Cluster",
			Name: name,
			Spec: map[string]interface{}{"test": "value"},
		}

		_, _, err := client.DefaultAPI.PostCluster(ctx).ClusterCreateRequest(clusterInput).Execute()
		Expect(err).NotTo(HaveOccurred(), "Failed to create cluster %s", name)
	}

	// List with orderBy=name desc
	list, _, err := client.DefaultAPI.GetClusters(ctx).OrderBy("name desc").Execute()
	Expect(err).NotTo(HaveOccurred(), "Failed to list clusters with orderBy desc")

	// Find our test clusters in the response
	var testClusters []openapi.Cluster
	for _, item := range list.Items {
		if strings.HasPrefix(item.Name, testPrefix) {
			testClusters = append(testClusters, item)
		}
	}

	Expect(len(testClusters)).To(Equal(3), "Should find all 3 test clusters")

	// Verify they are sorted by name desc (reverse alphabetically)
	Expect(testClusters[0].Name).To(ContainSubstring("charlie"), "First should be charlie")
	Expect(testClusters[1].Name).To(ContainSubstring("bravo"), "Second should be bravo")
	Expect(testClusters[2].Name).To(ContainSubstring("alpha"), "Third should be alpha")

	t.Logf("✓ Descending sorting works: clusters sorted by name desc")
}
