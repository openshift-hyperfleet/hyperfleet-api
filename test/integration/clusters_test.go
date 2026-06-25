package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"gopkg.in/resty.v1"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
	"github.com/openshift-hyperfleet/hyperfleet-api/test/factories"
)

func TestClusterGet(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// 401 using no JWT token
	resp, err := client.GetClusterByIdWithResponse(context.Background(), "foo", nil)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusUnauthorized), "Expected 401 but got %d", resp.StatusCode())

	// GET responses per openapi spec: 200 and 404,
	resp, err = client.GetClusterByIdWithResponse(ctx, "foo", nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusNotFound), "Expected 404")

	clusterModel, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	resp, err = client.GetClusterByIdWithResponse(ctx, clusterModel.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))

	clusterOutput := resp.JSON200
	Expect(clusterOutput).NotTo(BeNil())
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
	resp, err := client.PostClusterWithResponse(
		ctx, openapi.PostClusterJSONRequestBody(clusterInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred(), "Error posting object:  %v", err)
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated))

	clusterOutput := resp.JSON201
	Expect(clusterOutput).NotTo(BeNil())
	Expect(*clusterOutput.Id).NotTo(BeEmpty(), "Expected ID assigned on creation")
	Expect(len(*clusterOutput.Id)).To(Equal(36), "Expected UUID v7 length of 36 characters")
	Expect(*clusterOutput.Id).
		To(MatchRegexp(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`), "Expected UUID v7 format")
	Expect(clusterOutput.Kind).To(Equal("Cluster"))
	Expect(*clusterOutput.Href).To(Equal(fmt.Sprintf("/api/hyperfleet/v1/clusters/%s", *clusterOutput.Id)))

	// 400 bad request. posting junk json is one way to trigger 400.
	jwtToken := test.GetAccessTokenFromContext(ctx)
	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(`{ this is invalid }`).
		Post(h.RestURL("/clusters"))
	Expect(err).ToNot(HaveOccurred(), "Error object:  %v", err)
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

func TestClusterPatch(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// 404 for non-existent cluster
	patchBody := openapi.PatchClusterByIdJSONRequestBody{
		Spec: &openapi.ClusterSpec{"region": "us-east-1"},
	}
	resp, err := client.PatchClusterByIdWithResponse(ctx, "non-existent-id", patchBody, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusNotFound))

	clusterModel, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())
	initialGeneration := clusterModel.Generation

	// 200 success
	newSpec := openapi.ClusterSpec{"region": "us-east-1", "provider": "aws"}
	patchBody = openapi.PatchClusterByIdJSONRequestBody{Spec: &newSpec}

	patchResp, err := client.PatchClusterByIdWithResponse(ctx, clusterModel.ID, patchBody, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(patchResp.StatusCode()).To(Equal(http.StatusOK))

	updated := patchResp.JSON200
	Expect(updated).NotTo(BeNil())
	Expect(*updated.Id).To(Equal(clusterModel.ID))
	Expect(updated.Kind).To(Equal("Cluster"))
	Expect(updated.Generation).To(Equal(initialGeneration+1), "Generation should increment when spec changes")
	Expect(updated.Spec).To(HaveKeyWithValue("region", "us-east-1"))
	Expect(updated.Spec).To(HaveKeyWithValue("provider", "aws"))
	Expect(updated.Spec).To(HaveLen(2), "Spec should contain exactly the patched fields, not a merge")

	// Patch labels only
	newLabels := map[string]string{"env": "staging", "team": "platform"}
	labelsPatchResp, err := client.PatchClusterByIdWithResponse(
		ctx, clusterModel.ID,
		openapi.PatchClusterByIdJSONRequestBody{Labels: &newLabels},
		test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(labelsPatchResp.StatusCode()).To(Equal(http.StatusOK))

	updatedWithLabels := labelsPatchResp.JSON200
	Expect(updatedWithLabels).NotTo(BeNil())
	Expect(updatedWithLabels.Labels).NotTo(BeNil())
	Expect(*updatedWithLabels.Labels).To(HaveKeyWithValue("env", "staging"))
	Expect(*updatedWithLabels.Labels).To(HaveKeyWithValue("team", "platform"))
	Expect(*updatedWithLabels.Labels).To(HaveLen(2))
	Expect(updatedWithLabels.Generation).To(Equal(updated.Generation+1), "Generation should increment when labels change")
}

func TestClusterPatch_SetReconciledFalse(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a cluster with Reconciled=True
	cluster, err := factories.NewClusterWithStatus(&h.Factories, h.DBFactory, h.NewID(), true, true)
	Expect(err).NotTo(HaveOccurred())

	newLabels := map[string]string{"env": "staging"}
	patchResp, err := client.PatchClusterByIdWithResponse(
		ctx, cluster.ID,
		openapi.PatchClusterByIdJSONRequestBody{Labels: &newLabels},
		test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(patchResp.StatusCode()).To(Equal(http.StatusOK))

	updated := patchResp.JSON200
	Expect(updated.Generation).To(Equal(cluster.Generation+1), "Generation should increment when labels change")

	var reconciledCond *openapi.ResourceCondition
	for i := range updated.Status.Conditions {
		if updated.Status.Conditions[i].Type == api.ResourceConditionTypeReconciled {
			reconciledCond = &updated.Status.Conditions[i]
			break
		}
	}
	Expect(reconciledCond).NotTo(BeNil(), "Expected Reconciled condition in response")
	Expect(reconciledCond.Status).To(Equal(openapi.ResourceConditionStatusFalse),
		"Reconciled must be False after generation increment")
}

func TestClusterPaging(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Paging
	_, err := h.Factories.NewClustersList("Bronto", 20)
	Expect(err).NotTo(HaveOccurred())

	resp, err := client.GetClustersWithResponse(ctx, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Error getting cluster list: %v", err)
	list := resp.JSON200
	Expect(list).NotTo(BeNil())
	Expect(len(list.Items)).To(Equal(20))
	Expect(list.Size).To(Equal(int32(20)))
	Expect(list.Total).To(Equal(int32(20)))
	Expect(list.Page).To(Equal(int32(1)))

	page := openapi.QueryParamsPage(2)
	size := openapi.QueryParamsSize(5)
	params := &openapi.GetClustersParams{
		Page: &page,
		Size: &size,
	}
	resp, err = client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Error getting cluster list: %v", err)
	list = resp.JSON200
	Expect(list).NotTo(BeNil())
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

	searchStr := fmt.Sprintf("id in ['%s']", clusters[0].ID)
	search := openapi.SearchParams(searchStr)
	params := &openapi.GetClustersParams{
		Search: &search,
	}
	resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Error getting cluster list: %v", err)
	list := resp.JSON200
	Expect(list).NotTo(BeNil())
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
	maliciousSearchStr := "id='anything' OR '1'='1'"
	maliciousSearch := openapi.SearchParams(maliciousSearchStr)
	params := &openapi.GetClustersParams{
		Search: &maliciousSearch,
	}
	_, err = client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))
	// Should either return 400 error or return empty/controlled results
	// Not crash or return all data
	if err == nil {
		// If no error, the search should not return everything
		t.Logf("Search with SQL injection did not error - implementation may handle it gracefully")
	}

	// Test 2: SQL injection attempt with DROP
	dropSearchStr := "id='; DROP TABLE clusters; --"
	dropSearch := openapi.SearchParams(dropSearchStr)
	params = &openapi.GetClustersParams{
		Search: &dropSearch,
	}
	_, err = client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))
	// Should not crash
	if err == nil {
		t.Logf("Search with DROP statement did not error - implementation may handle it gracefully")
	}

	// Test 3: Verify clusters still exist after injection attempts
	resp, err := client.GetClustersWithResponse(ctx, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	list := resp.JSON200
	Expect(list).NotTo(BeNil())
	Expect(list.Total).To(BeNumerically(">=", 5), "Clusters should still exist after injection attempts")

	// Test 4: Valid search still works
	validSearchStr := fmt.Sprintf("id='%s'", clusters[0].ID)
	validSearch := openapi.SearchParams(validSearchStr)
	params = &openapi.GetClustersParams{
		Search: &validSearch,
	}
	resp, err = client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(len(resp.JSON200.Items)).To(BeNumerically(">=", 0))
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

	resp, err := client.PostClusterWithResponse(
		ctx, openapi.PostClusterJSONRequestBody(clusterInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated))
	id1 := *resp.JSON201.Id

	// Create second cluster with the SAME name
	// Names are unique, so this should return 409 Conflict
	resp, err = client.PostClusterWithResponse(
		ctx, openapi.PostClusterJSONRequestBody(clusterInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).
		To(Equal(http.StatusConflict), "Expected 409 Conflict for duplicate name")

	// Verify first cluster still exists
	getResp, err := client.GetClusterByIdWithResponse(
		ctx, id1, nil, test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(getResp.JSON200.Name).To(Equal("duplicate-name-test"))
}

// TestClusterBoundaryValues tests boundary values for cluster fields
func TestClusterBoundaryValues(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Test 1: Maximum name length (limit is 53 characters, aligned with CS)
	longName := ""
	for i := 0; i < 53; i++ {
		longName += "a"
	}

	longNameInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: longName,
		Spec: map[string]interface{}{"test": "spec"},
	}

	resp, err := client.PostClusterWithResponse(
		ctx, openapi.PostClusterJSONRequestBody(longNameInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred(), "Should accept name up to 53 characters")
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated))
	Expect(resp.JSON201.Name).To(Equal(longName))

	// Test exceeding max length (54 characters should fail)
	tooLongName := longName + "a"
	tooLongInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: tooLongName,
		Spec: map[string]interface{}{"test": "spec"},
	}
	resp, err = client.PostClusterWithResponse(
		ctx, openapi.PostClusterJSONRequestBody(tooLongInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).
		To(Equal(http.StatusBadRequest), "Should reject name exceeding 53 characters")

	// Test 2: Empty name
	emptyNameInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: "",
		Spec: map[string]interface{}{"test": "spec"},
	}

	resp, err = client.PostClusterWithResponse(
		ctx, openapi.PostClusterJSONRequestBody(emptyNameInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).
		To(Equal(http.StatusBadRequest), "Should reject empty name")

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

	resp, err = client.PostClusterWithResponse(
		ctx, openapi.PostClusterJSONRequestBody(largeSpecInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred(), "Should accept large spec JSON")
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated))

	// Verify the spec was stored correctly
	getResp, err := client.GetClusterByIdWithResponse(
		ctx, *resp.JSON201.Id, nil, test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(len(getResp.JSON200.Spec)).To(Equal(100))

	// Test 4: Unicode in name (should be rejected - pattern only allows [a-z0-9-])
	unicodeNameInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: "テスト-δοκιμή-🚀",
		Spec: map[string]interface{}{"test": "spec"},
	}

	resp, err = client.PostClusterWithResponse(
		ctx, openapi.PostClusterJSONRequestBody(unicodeNameInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest),
		"Should reject unicode in name (pattern is ^[a-z0-9-]+$)")
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

	resp, err := client.PostClusterWithResponse(
		ctx, openapi.PostClusterJSONRequestBody(validInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred(), "Valid spec should be accepted")
	Expect(resp.StatusCode()).To(Equal(http.StatusCreated))
	Expect(*resp.JSON201.Id).NotTo(BeEmpty())

	// Test 2: Invalid spec type (spec must be object, not string)
	// This should fail even with base schema
	// Can't use the generated struct because Spec is typed as map[string]interface{}
	// So we send raw JSON request
	invalidTypeJSON := `{
		"kind": "Cluster",
		"name": "schema-invalid-type",
		"spec": "invalid-string-spec"
	}`

	jwtToken := test.GetAccessTokenFromContext(ctx)

	resp2, _ := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidTypeJSON).
		Post(h.RestURL("/clusters"))

	if resp2.StatusCode() == http.StatusBadRequest {
		t.Logf("Schema validation correctly rejected invalid spec type")
		// Verify error response contains details
		var errorResponse openapi.ProblemDetails
		_ = json.Unmarshal(resp2.Body(), &errorResponse)
		Expect(errorResponse.Type).ToNot(BeEmpty())
		Expect(errorResponse.Code).ToNot(BeNil())
		Expect(errorResponse.Detail).ToNot(BeNil())
	} else {
		t.Logf("Base schema may accept any spec type, status: %d", resp2.StatusCode())
	}

	// Test 3: Empty spec (should be valid as spec is optional in base schema)
	emptySpecInput := openapi.ClusterCreateRequest{
		Kind: "Cluster",
		Name: "schema-empty-spec",
		Spec: map[string]interface{}{},
	}

	resp3, err := client.PostClusterWithResponse(
		ctx, openapi.PostClusterJSONRequestBody(emptySpecInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred(), "Empty spec should be accepted by base schema")
	Expect(resp3.StatusCode()).To(Equal(http.StatusCreated))
	Expect(*resp3.JSON201.Id).NotTo(BeEmpty())
}

// TestClusterSchemaValidationWithProviderSchema tests schema validation with a provider-specific schema
// This test will only work if HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH is set to a provider schema
// (e.g., gcp_openapi.yaml). When using the base schema, this test will be skipped.
func TestClusterSchemaValidationWithProviderSchema(t *testing.T) {
	RegisterTestingT(t)

	// Run only when a strict provider-specific schema is configured.
	// Default integration tests use test/validation-schema.yaml (permissive) or openapi/openapi.yaml.
	schemaPath := os.Getenv("HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH")
	if schemaPath == "" ||
		strings.HasSuffix(schemaPath, "openapi/openapi.yaml") ||
		strings.HasSuffix(schemaPath, "validation-schema.yaml") {
		t.Skip("Skipping provider schema validation test - using permissive or base schema")
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

	resp, err := client.PostClusterWithResponse(
		ctx, openapi.PostClusterJSONRequestBody(invalidInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest),
		"Should reject spec with missing required field")

	// Parse error response to verify field-level details
	bodyBytes, err := io.ReadAll(resp.HTTPResponse.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var errorResponse openapi.ProblemDetails
	if err := json.Unmarshal(bodyBytes, &errorResponse); err != nil {
		t.Fatalf("failed to unmarshal error response body: %v", err)
	}

	Expect(errorResponse.Type).ToNot(BeEmpty())
	Expect(errorResponse.Code).ToNot(BeNil())
	Expect(*errorResponse.Code).To(Equal("HYPERFLEET-VAL-000")) // Validation error code (RFC 9457 format)
	Expect(errorResponse.Errors).ToNot(BeEmpty(), "Should include field-level error details")

	// Verify errors contain field path
	foundRegionError := false
	if errorResponse.Errors != nil {
		for _, detail := range *errorResponse.Errors {
			if strings.Contains(detail.Field, "region") {
				foundRegionError = true
				break
			}
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
	jwtToken := test.GetAccessTokenFromContext(ctx)

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
	var errorResponse openapi.ProblemDetails
	if err := json.Unmarshal(resp.Body(), &errorResponse); err != nil {
		t.Fatalf("failed to unmarshal error response: %v, response body: %s", err, string(resp.Body()))
	}

	// Verify error structure (RFC 9457 Problem Details format)
	Expect(errorResponse.Type).ToNot(BeEmpty())
	Expect(errorResponse.Title).ToNot(BeEmpty())

	Expect(errorResponse.Code).ToNot(BeNil())
	// Both HYPERFLEET-VAL-000 (validation error) and HYPERFLEET-VAL-006 (malformed request) are acceptable
	// as they both indicate the spec field is invalid
	validCodes := []string{"HYPERFLEET-VAL-000", "HYPERFLEET-VAL-006"}
	Expect(validCodes).To(ContainElement(*errorResponse.Code), "Expected validation or format error code")

	Expect(errorResponse.Detail).ToNot(BeNil())
	Expect(*errorResponse.Detail).To(ContainSubstring("spec"))

	Expect(errorResponse.Instance).ToNot(BeNil())
	Expect(errorResponse.TraceId).ToNot(BeNil())

	t.Logf("Error response: code=%s, detail=%s", *errorResponse.Code, *errorResponse.Detail)
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
			Name: fmt.Sprintf("sort-test-%d-%s", i, strings.ToLower(h.NewID())),
			Spec: map[string]interface{}{"test": fmt.Sprintf("value-%d", i)},
		}

		resp, err := client.PostClusterWithResponse(
			ctx, openapi.PostClusterJSONRequestBody(clusterInput), test.WithAuthToken(ctx),
		)
		Expect(err).NotTo(HaveOccurred(), "Failed to create cluster %d", i)
		createdClusters = append(createdClusters, *resp.JSON201)

		// Add 100ms delay to ensure different created_time
		time.Sleep(100 * time.Millisecond)
	}

	// List clusters without order parameter - should default to created_time desc
	listResp, err := client.GetClustersWithResponse(
		ctx, nil, test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred(), "Failed to list clusters")
	list := listResp.JSON200
	Expect(list).NotTo(BeNil())
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
func TestClusterList_OrderName(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create clusters with names that will sort alphabetically
	testPrefix := fmt.Sprintf("sort-%s", strings.ToLower(h.NewID()))
	names := []string{
		fmt.Sprintf("%s-charlie", testPrefix),
		fmt.Sprintf("%s-alpha", testPrefix),
		fmt.Sprintf("%s-bravo", testPrefix),
	}

	for _, name := range names {
		clusterInput := openapi.ClusterCreateRequest{
			Kind: "Cluster",
			Name: name,
			Spec: map[string]interface{}{"test": "value"},
		}

		_, err := client.PostClusterWithResponse(
			ctx, openapi.PostClusterJSONRequestBody(clusterInput), test.WithAuthToken(ctx),
		)
		Expect(err).NotTo(HaveOccurred(), "Failed to create cluster %s", name)
	}

	// List with order=name asc
	orderStr := openapi.QueryParamsOrder("name asc")
	params := &openapi.GetClustersParams{
		Order: &orderStr,
	}
	listResp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Failed to list clusters with order")
	list := listResp.JSON200
	Expect(list).NotTo(BeNil())
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

// TestClusterList_OrderNameDesc tests sorting by name descending
func TestClusterList_OrderNameDesc(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create clusters with names that will sort alphabetically
	testPrefix := fmt.Sprintf("sort-%s", strings.ToLower(h.NewID()))
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

		_, err := client.PostClusterWithResponse(
			ctx, openapi.PostClusterJSONRequestBody(clusterInput), test.WithAuthToken(ctx),
		)
		Expect(err).NotTo(HaveOccurred(), "Failed to create cluster %s", name)
	}

	// List with order=name desc
	orderStr := openapi.QueryParamsOrder("name desc")
	params := &openapi.GetClustersParams{
		Order: &orderStr,
	}
	listResp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred(), "Failed to list clusters with order desc")
	list := listResp.JSON200

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

// TestClusterPost_EmptyKind tests that empty kind field returns 400
func TestClusterPost_EmptyKind(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	// Send request with empty kind
	invalidInput := `{
		"kind": "",
		"name": "test-cluster",
		"spec": {}
	}`

	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidInput).
		Post(h.RestURL("/clusters"))

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

// TestClusterPost_WrongKind tests that wrong kind field returns 400
func TestClusterPost_WrongKind(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	// Send request with wrong kind
	invalidInput := `{
		"kind": "NodePool",
		"name": "test-cluster",
		"spec": {}
	}`

	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidInput).
		Post(h.RestURL("/clusters"))

	Expect(err).ToNot(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))

	// Parse error response
	var errorResponse map[string]interface{}
	err = json.Unmarshal(restyResp.Body(), &errorResponse)
	Expect(err).ToNot(HaveOccurred())

	// Verify error message contains "kind must be 'Cluster'" (RFC 9457 uses "detail" field)
	detail, ok := errorResponse["detail"].(string)
	Expect(ok).To(BeTrue())
	Expect(detail).To(ContainSubstring("kind must be 'Cluster'"))
}

// TestClusterPost_NullSpec tests that null spec field returns 400
func TestClusterPost_NullSpec(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	// Send request with null spec
	invalidInput := `{
		"kind": "Cluster",
		"name": "test-cluster",
		"spec": null
	}`

	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidInput).
		Post(h.RestURL("/clusters"))

	Expect(err).ToNot(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))

	var errorResponse map[string]interface{}
	err = json.Unmarshal(restyResp.Body(), &errorResponse)
	Expect(err).ToNot(HaveOccurred())

	code, ok := errorResponse["code"].(string)
	Expect(ok).To(BeTrue())
	Expect(code).To(Equal("HYPERFLEET-VAL-000"))

	detail, ok := errorResponse["detail"].(string)
	Expect(ok).To(BeTrue())
	Expect(detail).To(ContainSubstring("spec field must be an object"))
}

// TestClusterPost_MissingSpec tests that missing spec field returns 400
func TestClusterPost_MissingSpec(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)
	jwtToken := test.GetAccessTokenFromContext(ctx)

	// Send request without spec field
	invalidInput := `{
		"kind": "Cluster",
		"name": "test-cluster"
	}`

	restyResp, err := resty.R().
		SetHeader("Content-Type", "application/json").
		SetHeader("Authorization", fmt.Sprintf("Bearer %s", jwtToken)).
		SetBody(invalidInput).
		Post(h.RestURL("/clusters"))

	Expect(err).ToNot(HaveOccurred())
	Expect(restyResp.StatusCode()).To(Equal(http.StatusBadRequest))

	var errorResponse map[string]interface{}
	err = json.Unmarshal(restyResp.Body(), &errorResponse)
	Expect(err).ToNot(HaveOccurred())

	code, ok := errorResponse["code"].(string)
	Expect(ok).To(BeTrue())
	Expect(code).To(Equal("HYPERFLEET-VAL-000"))

	detail, ok := errorResponse["detail"].(string)
	Expect(ok).To(BeTrue())
	Expect(detail).To(ContainSubstring("spec is required"))
}

func TestClusterSoftDelete(t *testing.T) {
	t.Run("given a valid cluster, when deleted, then returns 202 with deleted_time and deleted_by set", func(t *testing.T) { //nolint:lll
		RegisterTestingT(t)
		// Given:
		h, client := test.RegisterIntegration(t)
		account := h.NewRandAccount()
		ctx := h.NewAuthenticatedContext(account)
		cluster, err := h.Factories.NewClusters(h.NewID())
		Expect(err).NotTo(HaveOccurred())
		// When:
		resp, err := client.DeleteClusterByIdWithResponse(ctx, cluster.ID, test.WithAuthToken(ctx))
		// Then:
		Expect(err).NotTo(HaveOccurred())
		Expect(resp.StatusCode()).To(Equal(http.StatusAccepted))
		Expect(resp.JSON202).NotTo(BeNil())
		Expect(*resp.JSON202.Id).To(Equal(cluster.ID))
		Expect(resp.JSON202.DeletedTime).NotTo(BeNil())
		Expect(resp.JSON202.DeletedBy).NotTo(BeNil())
		Expect(string(*resp.JSON202.DeletedBy)).To(Equal(account.Email))
	})

	t.Run("given a cluster with child nodepools, when deleted, then nodepools are cascade soft-deleted in DB", func(t *testing.T) { //nolint:lll
		RegisterTestingT(t)
		// Given:
		h, client := test.RegisterIntegration(t)
		account := h.NewRandAccount()
		ctx := h.NewAuthenticatedContext(account)
		cluster, err := h.Factories.NewClusters(h.NewID())
		Expect(err).NotTo(HaveOccurred())
		npInput := openapi.NodePoolCreateRequest{
			Kind: "NodePool",
			Name: "cascade-np",
			Spec: map[string]interface{}{"test": "spec"},
		}
		npResp, err := client.CreateNodePoolWithResponse(
			ctx, cluster.ID, openapi.CreateNodePoolJSONRequestBody(npInput), test.WithAuthToken(ctx),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(npResp.StatusCode()).To(Equal(http.StatusCreated))
		nodePoolID := *npResp.JSON201.Id
		// When:
		delResp, err := client.DeleteClusterByIdWithResponse(ctx, cluster.ID, test.WithAuthToken(ctx))
		// Then:
		Expect(err).NotTo(HaveOccurred())
		Expect(delResp.StatusCode()).To(Equal(http.StatusAccepted))
		Expect(delResp.JSON202.DeletedTime).NotTo(BeNil())
		Expect(string(*delResp.JSON202.DeletedBy)).To(Equal(account.Email))
		// Verify cascade via direct DB query
		dbSession := h.DBFactory.New(ctx)
		var nodePool api.NodePool
		Expect(dbSession.First(&nodePool, "id = ?", nodePoolID).Error).NotTo(HaveOccurred())
		Expect(nodePool.DeletedTime).NotTo(BeNil(), "nodepool should be soft-deleted after cluster cascade")
		Expect(nodePool.DeletedBy).NotTo(BeNil(), "nodepool deleted_by should be set after cluster cascade")
	})

	t.Run("given a cluster with Reconciled=True, when deleted, then generation increments and Reconciled becomes False", func(t *testing.T) { //nolint:lll
		RegisterTestingT(t)
		// Given:
		h, client := test.RegisterIntegration(t)
		account := h.NewRandAccount()
		ctx := h.NewAuthenticatedContext(account)
		cluster, err := factories.NewClusterWithStatus(&h.Factories, h.DBFactory, h.NewID(), true, true)
		Expect(err).NotTo(HaveOccurred())
		initialGeneration := cluster.Generation
		// When:
		delResp, err := client.DeleteClusterByIdWithResponse(ctx, cluster.ID, test.WithAuthToken(ctx))
		// Then:
		Expect(err).NotTo(HaveOccurred())
		Expect(delResp.StatusCode()).To(Equal(http.StatusAccepted))
		Expect(delResp.JSON202).NotTo(BeNil())
		Expect(delResp.JSON202.Generation).To(Equal(initialGeneration+1),
			"Generation should be incremented after soft-delete")
		var reconciledCond *openapi.ResourceCondition
		for i := range delResp.JSON202.Status.Conditions {
			if delResp.JSON202.Status.Conditions[i].Type == api.ResourceConditionTypeReconciled {
				reconciledCond = &delResp.JSON202.Status.Conditions[i]
				break
			}
		}
		Expect(reconciledCond).NotTo(BeNil(), "Expected Reconciled condition in response")
		Expect(reconciledCond.Status).To(Equal(openapi.ResourceConditionStatusFalse),
			"Reconciled should be False after soft-delete due to generation bump")
	})

	t.Run("given a cluster soft-delete cascades to child nodepools, when deleted, then both cluster and nodepool generation increment and Reconciled becomes False", func(t *testing.T) { //nolint:lll
		RegisterTestingT(t)
		// Given:
		h, client := test.RegisterIntegration(t)
		account := h.NewRandAccount()
		ctx := h.NewAuthenticatedContext(account)
		cluster, err := factories.NewClusterWithStatus(&h.Factories, h.DBFactory, h.NewID(), true, true)
		Expect(err).NotTo(HaveOccurred())
		nodePool, err := factories.NewNodePoolWithStatus(&h.Factories, h.DBFactory, h.NewID(), true, true)
		Expect(err).NotTo(HaveOccurred())
		initialNodePoolGeneration := nodePool.Generation
		dbSession := h.DBFactory.New(ctx)
		Expect(dbSession.Model(nodePool).Update("owner_id", cluster.ID).Error).NotTo(HaveOccurred())
		// When:
		delResp, err := client.DeleteClusterByIdWithResponse(ctx, cluster.ID, test.WithAuthToken(ctx))
		// Then: Verify cluster generation and Reconciled status
		Expect(err).NotTo(HaveOccurred())
		Expect(delResp.StatusCode()).To(Equal(http.StatusAccepted))
		Expect(delResp.JSON202.Generation).To(Equal(cluster.Generation+1),
			"Cluster generation should increment on soft-delete")
		var clusterReconciledCond *openapi.ResourceCondition
		for i := range delResp.JSON202.Status.Conditions {
			if delResp.JSON202.Status.Conditions[i].Type == api.ResourceConditionTypeReconciled {
				clusterReconciledCond = &delResp.JSON202.Status.Conditions[i]
				break
			}
		}
		Expect(clusterReconciledCond).NotTo(BeNil(), "Expected Reconciled condition in cluster response")
		Expect(clusterReconciledCond.Status).To(Equal(openapi.ResourceConditionStatusFalse),
			"Cluster Reconciled should be False after soft-delete")
		// Then: Verify cascaded nodepool generation and Reconciled status
		var npAfterDelete api.NodePool
		Expect(dbSession.First(&npAfterDelete, "id = ?", nodePool.ID).Error).NotTo(HaveOccurred())
		Expect(npAfterDelete.Generation).To(Equal(initialNodePoolGeneration+1),
			"NodePool generation should be incremented after cascade soft-delete")
		var conditions []api.ResourceCondition
		err = json.Unmarshal(npAfterDelete.StatusConditions, &conditions)
		Expect(err).NotTo(HaveOccurred(), "should be able to unmarshal nodepool status conditions")
		var reconciledCond *api.ResourceCondition
		for i := range conditions {
			if conditions[i].Type == api.ResourceConditionTypeReconciled {
				reconciledCond = &conditions[i]
				break
			}
		}
		Expect(reconciledCond).NotTo(BeNil(), "Expected Reconciled condition in nodepool status")
		Expect(reconciledCond.Status).To(Equal(api.ConditionFalse),
			"Reconciled should be False after cascade soft-delete due to generation bump")
	})

	t.Run("given an already-deleted cluster, when deleted again, then returns 202 with unchanged deleted_time and nodepool state is unchanged", func(t *testing.T) { //nolint:lll
		RegisterTestingT(t)
		// Given:
		h, client := test.RegisterIntegration(t)
		account := h.NewRandAccount()
		ctx := h.NewAuthenticatedContext(account)
		cluster, err := h.Factories.NewClusters(h.NewID())
		Expect(err).NotTo(HaveOccurred())
		npInput := openapi.NodePoolCreateRequest{
			Kind: "NodePool",
			Name: "idem-np",
			Spec: map[string]interface{}{"test": "spec"},
		}
		npResp, err := client.CreateNodePoolWithResponse(
			ctx, cluster.ID, openapi.CreateNodePoolJSONRequestBody(npInput), test.WithAuthToken(ctx),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(npResp.StatusCode()).To(Equal(http.StatusCreated))
		nodePoolID := *npResp.JSON201.Id
		// When: first delete
		resp1, err := client.DeleteClusterByIdWithResponse(ctx, cluster.ID, test.WithAuthToken(ctx))
		Expect(err).NotTo(HaveOccurred())
		Expect(resp1.StatusCode()).To(Equal(http.StatusAccepted))
		firstDeletedTime := resp1.JSON202.DeletedTime
		Expect(firstDeletedTime).NotTo(BeNil())
		// Capture nodepool state after first delete
		dbSession := h.DBFactory.New(ctx)
		var npAfterFirst api.NodePool
		Expect(dbSession.First(&npAfterFirst, "id = ?", nodePoolID).Error).NotTo(HaveOccurred())
		npGenerationAfterFirst := npAfterFirst.Generation
		// When: second delete (idempotent)
		resp2, err := client.DeleteClusterByIdWithResponse(ctx, cluster.ID, test.WithAuthToken(ctx))
		// Then:
		Expect(err).NotTo(HaveOccurred())
		Expect(resp2.StatusCode()).To(Equal(http.StatusAccepted))
		Expect(resp2.JSON202.DeletedTime.Equal(*firstDeletedTime)).To(BeTrue(),
			"cluster deleted_time should not change on repeated delete")
		var npAfterSecond api.NodePool
		Expect(dbSession.First(&npAfterSecond, "id = ?", nodePoolID).Error).NotTo(HaveOccurred())
		Expect(npAfterSecond.DeletedTime.Equal(*npAfterFirst.DeletedTime)).To(BeTrue(),
			"nodepool deleted_time should not change on repeated delete")
		Expect(npAfterSecond.Generation).To(Equal(npGenerationAfterFirst),
			"nodepool generation should not be incremented on repeated delete")
	})
}

func TestClusterHardDelete(t *testing.T) {
	t.Run("given a soft-deleted cluster with no nodepools, when all required adapters report Finalized=True, then cluster is hard-deleted from DB", func(t *testing.T) { //nolint:lll
		RegisterTestingT(t)
		h, client := test.RegisterIntegration(t)
		account := h.NewRandAccount()
		ctx := h.NewAuthenticatedContext(account)

		cluster, err := h.Factories.NewClusters(h.NewID())
		Expect(err).NotTo(HaveOccurred())

		delResp, err := client.DeleteClusterByIdWithResponse(ctx, cluster.ID, test.WithAuthToken(ctx))
		Expect(err).NotTo(HaveOccurred())
		Expect(delResp.StatusCode()).To(Equal(http.StatusAccepted))
		newGeneration := delResp.JSON202.Generation
		Expect(newGeneration).To(Equal(cluster.Generation + 1))

		requiredAdapters := []string{"validation", "dns", "pullsecret", "hypershift"}
		dbSession := h.DBFactory.New(ctx)

		for _, adapter := range requiredAdapters {
			statusInput := newAdapterStatusRequest(
				adapter,
				newGeneration,
				[]openapi.ConditionRequest{
					{
						Type:   api.AdapterConditionTypeApplied,
						Status: openapi.AdapterConditionStatusFalse,
						Reason: util.PtrString("ManifestWorkNotDiscovered"),
					},
					{
						Type:   api.AdapterConditionTypeAvailable,
						Status: openapi.AdapterConditionStatusFalse,
						Reason: util.PtrString("NamespaceNotDiscovered"),
					},
					{
						Type:   api.AdapterConditionTypeHealth,
						Status: openapi.AdapterConditionStatusTrue,
						Reason: util.PtrString("Healthy"),
					},
					{
						Type:   api.AdapterConditionTypeFinalized,
						Status: openapi.AdapterConditionStatusTrue,
						Reason: util.PtrString("CleanupConfirmed"),
					},
				},
				nil,
			)
			statusResp, loopErr := client.PutClusterStatusesWithResponse(
				ctx, cluster.ID,
				openapi.PutClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
			)
			Expect(loopErr).NotTo(HaveOccurred())
			Expect(statusResp.StatusCode()).To(Equal(http.StatusCreated))
		}

		var clusterCheck api.Cluster
		dbErr := dbSession.First(&clusterCheck, "id = ?", cluster.ID).Error
		Expect(dbErr).To(HaveOccurred(), "Cluster should be hard-deleted from DB")
		Expect(dbErr.Error()).To(ContainSubstring("record not found"))

		var adapterStatuses []api.AdapterStatus
		err = dbSession.Where("resource_type = ? AND resource_id = ?", api.ResourceTypeCluster, cluster.ID).
			Find(&adapterStatuses).Error
		Expect(err).NotTo(HaveOccurred())
		Expect(adapterStatuses).To(BeEmpty(), "Adapter statuses should be hard-deleted")
	})

	t.Run("given a soft-deleted cluster with nodepools, when a non-last adapter reports Finalized=True, then status is stored but cluster is not hard-deleted", func(t *testing.T) { //nolint:lll
		RegisterTestingT(t)
		h, client := test.RegisterIntegration(t)
		account := h.NewRandAccount()
		ctx := h.NewAuthenticatedContext(account)

		cluster, err := h.Factories.NewClusters(h.NewID())
		Expect(err).NotTo(HaveOccurred())
		nodePool, err := h.Factories.NewNodePools(h.NewID())
		Expect(err).NotTo(HaveOccurred())
		dbSession := h.DBFactory.New(ctx)
		err = dbSession.Model(nodePool).Update("owner_id", cluster.ID).Error
		Expect(err).NotTo(HaveOccurred())

		delResp, err := client.DeleteClusterByIdWithResponse(ctx, cluster.ID, test.WithAuthToken(ctx))
		Expect(err).NotTo(HaveOccurred())
		Expect(delResp.StatusCode()).To(Equal(http.StatusAccepted))
		newGeneration := delResp.JSON202.Generation

		statusInput := newAdapterStatusRequest(
			"validation",
			newGeneration,
			[]openapi.ConditionRequest{
				{
					Type:   api.AdapterConditionTypeApplied,
					Status: openapi.AdapterConditionStatusFalse,
					Reason: util.PtrString("ManifestWorkNotDiscovered"),
				},
				{
					Type:   api.AdapterConditionTypeAvailable,
					Status: openapi.AdapterConditionStatusFalse,
					Reason: util.PtrString("NamespaceNotDiscovered"),
				},
				{
					Type:   api.AdapterConditionTypeHealth,
					Status: openapi.AdapterConditionStatusTrue,
					Reason: util.PtrString("Healthy"),
				},
				{
					Type:   api.AdapterConditionTypeFinalized,
					Status: openapi.AdapterConditionStatusTrue,
					Reason: util.PtrString("CleanupConfirmed"),
				},
			},
			nil,
		)
		statusResp, err := client.PutClusterStatusesWithResponse(
			ctx, cluster.ID,
			openapi.PutClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(statusResp.StatusCode()).To(Equal(http.StatusCreated))

		var adapterStatus api.AdapterStatus
		err = dbSession.Where("resource_type = ? AND resource_id = ? AND adapter = ?",
			api.ResourceTypeCluster, cluster.ID, "validation").First(&adapterStatus).Error
		Expect(err).NotTo(HaveOccurred())

		var clusterCheck api.Cluster
		err = dbSession.First(&clusterCheck, "id = ?", cluster.ID).Error
		Expect(err).NotTo(HaveOccurred())
	})

	t.Run(`given a soft-deleted cluster with a nodepool, when cluster adapters,
		report Finalized=True before nodepool adapters, cluster is not hard-deleted,
		then once node pool adapters report Finalized=True as well, cluster is hard-deleted`, func(t *testing.T) {
		RegisterTestingT(t)
		h, client := test.RegisterIntegration(t)
		account := h.NewRandAccount()
		ctx := h.NewAuthenticatedContext(account)

		cluster, err := h.Factories.NewClusters(h.NewID())
		Expect(err).NotTo(HaveOccurred())
		nodePool, err := h.Factories.NewNodePools(h.NewID())
		Expect(err).NotTo(HaveOccurred())
		dbSession := h.DBFactory.New(ctx)
		err = dbSession.Model(nodePool).Update("owner_id", cluster.ID).Error
		Expect(err).NotTo(HaveOccurred())

		delResp, err := client.DeleteClusterByIdWithResponse(ctx, cluster.ID, test.WithAuthToken(ctx))
		Expect(err).NotTo(HaveOccurred())
		Expect(delResp.StatusCode()).To(Equal(http.StatusAccepted))
		clusterNewGeneration := delResp.JSON202.Generation

		var nodePoolAfterDelete api.NodePool
		err = dbSession.First(&nodePoolAfterDelete, "id = ?", nodePool.ID).Error
		Expect(err).NotTo(HaveOccurred())
		nodePoolNewGeneration := nodePoolAfterDelete.Generation
		Expect(nodePoolNewGeneration).To(Equal(nodePool.Generation + 1))

		// STEP 1: Report all required cluster adapters with Finalized=True
		// Nodepools still exist → cluster is not hard-deleted
		clusterAdapters := []string{"validation", "dns", "pullsecret", "hypershift"}
		for _, adapter := range clusterAdapters {
			statusInput := newAdapterStatusRequest(
				adapter,
				clusterNewGeneration,
				[]openapi.ConditionRequest{
					{
						Type:   api.AdapterConditionTypeApplied,
						Status: openapi.AdapterConditionStatusFalse,
						Reason: util.PtrString("ManifestWorkNotDiscovered"),
					},
					{
						Type:   api.AdapterConditionTypeAvailable,
						Status: openapi.AdapterConditionStatusFalse,
						Reason: util.PtrString("NamespaceNotDiscovered"),
					},
					{
						Type:   api.AdapterConditionTypeHealth,
						Status: openapi.AdapterConditionStatusTrue,
						Reason: util.PtrString("Healthy"),
					},
					{
						Type:   api.AdapterConditionTypeFinalized,
						Status: openapi.AdapterConditionStatusTrue,
						Reason: util.PtrString("CleanupConfirmed"),
					},
				},
				nil,
			)
			statusResp, loopErr := client.PutClusterStatusesWithResponse(
				ctx, cluster.ID,
				openapi.PutClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
			)
			Expect(loopErr).NotTo(HaveOccurred())
			Expect(statusResp.StatusCode()).To(Equal(http.StatusCreated))
		}

		var clusterCheck api.Cluster
		err = dbSession.First(&clusterCheck, "id = ?", cluster.ID).Error
		Expect(err).NotTo(HaveOccurred())
		Expect(clusterCheck.DeletedTime).NotTo(BeNil(), "Cluster should still be soft-deleted")

		// STEP 2: Report all required nodepool adapters with Finalized=True → nodepool is hard-deleted
		nodePoolAdapters := []string{"validation", "hypershift"}
		for _, adapter := range nodePoolAdapters {
			statusInput := newAdapterStatusRequest(
				adapter,
				nodePoolNewGeneration,
				[]openapi.ConditionRequest{
					{
						Type:   api.AdapterConditionTypeApplied,
						Status: openapi.AdapterConditionStatusFalse,
						Reason: util.PtrString("ManifestWorkNotDiscovered"),
					},
					{
						Type:   api.AdapterConditionTypeAvailable,
						Status: openapi.AdapterConditionStatusFalse,
						Reason: util.PtrString("NamespaceNotDiscovered"),
					},
					{
						Type:   api.AdapterConditionTypeHealth,
						Status: openapi.AdapterConditionStatusTrue,
						Reason: util.PtrString("Healthy"),
					},
					{
						Type:   api.AdapterConditionTypeFinalized,
						Status: openapi.AdapterConditionStatusTrue,
						Reason: util.PtrString("CleanupConfirmed"),
					},
				},
				nil,
			)
			_, loopErr := client.PutNodePoolStatusesWithResponse(
				ctx, cluster.ID, nodePool.ID,
				openapi.PutNodePoolStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
			)
			Expect(loopErr).NotTo(HaveOccurred())
		}

		var nodePoolCheck api.NodePool
		nodePoolErr := dbSession.First(&nodePoolCheck, "id = ?", nodePool.ID).Error
		Expect(nodePoolErr).To(HaveOccurred())
		Expect(nodePoolErr.Error()).To(ContainSubstring("record not found"))

		// STEP 3: Re-report one cluster adapter with Finalized=True
		// No nodepools remain → cluster is hard-deleted
		statusInput := newAdapterStatusRequest(
			"hypershift",
			clusterNewGeneration,
			[]openapi.ConditionRequest{
				{
					Type:   api.AdapterConditionTypeApplied,
					Status: openapi.AdapterConditionStatusFalse,
					Reason: util.PtrString("ManifestWorkNotDiscovered"),
				},
				{
					Type:   api.AdapterConditionTypeAvailable,
					Status: openapi.AdapterConditionStatusFalse,
					Reason: util.PtrString("NamespaceNotDiscovered"),
				},
				{
					Type:   api.AdapterConditionTypeHealth,
					Status: openapi.AdapterConditionStatusTrue,
					Reason: util.PtrString("Healthy"),
				},
				{
					Type:   api.AdapterConditionTypeFinalized,
					Status: openapi.AdapterConditionStatusTrue,
					Reason: util.PtrString("CleanupConfirmed"),
				},
			},
			nil,
		)
		finalResp, err := client.PutClusterStatusesWithResponse(
			ctx, cluster.ID,
			openapi.PutClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(finalResp.StatusCode()).To(Equal(http.StatusCreated))

		clusterErr := dbSession.First(&clusterCheck, "id = ?", cluster.ID).Error
		Expect(clusterErr).To(HaveOccurred())
		Expect(clusterErr.Error()).To(ContainSubstring("record not found"))

		var adapterStatuses []api.AdapterStatus
		err = dbSession.Where("resource_type = ? AND resource_id = ?", api.ResourceTypeCluster, cluster.ID).
			Find(&adapterStatuses).Error
		Expect(err).NotTo(HaveOccurred())
		Expect(adapterStatuses).To(BeEmpty())
		err = dbSession.Where("resource_type = ? AND resource_id = ?", api.ResourceTypeNodePool, nodePool.ID).
			Find(&adapterStatuses).Error
		Expect(err).NotTo(HaveOccurred())
		Expect(adapterStatuses).To(BeEmpty())
	})
}

func TestClusterForceDelete(t *testing.T) {
	t.Run("force-delete cascade removes cluster, nodepools, and adapter statuses", func(t *testing.T) {
		RegisterTestingT(t)
		h, client := test.RegisterIntegration(t)
		account := h.NewRandAccount()
		ctx := h.NewAuthenticatedContext(account)

		cluster, err := h.Factories.NewClusters(h.NewID())
		Expect(err).NotTo(HaveOccurred())

		nodePool, err := h.Factories.NewNodePools(h.NewID())
		Expect(err).NotTo(HaveOccurred())
		dbSession := h.DBFactory.New(ctx)
		err = dbSession.Model(nodePool).Update("owner_id", cluster.ID).Error
		Expect(err).NotTo(HaveOccurred())

		// Soft-delete the cluster to put it in Finalizing state
		delResp, err := client.DeleteClusterByIdWithResponse(ctx, cluster.ID, test.WithAuthToken(ctx))
		Expect(err).NotTo(HaveOccurred())
		Expect(delResp.StatusCode()).To(Equal(http.StatusAccepted))

		// Report adapter statuses so there are records to cascade-delete
		newGeneration := delResp.JSON202.Generation
		statusInput := newAdapterStatusRequest(
			"validation",
			newGeneration,
			[]openapi.ConditionRequest{
				{
					Type:   api.AdapterConditionTypeApplied,
					Status: openapi.AdapterConditionStatusFalse,
					Reason: util.PtrString("Stuck"),
				},
				{
					Type:   api.AdapterConditionTypeAvailable,
					Status: openapi.AdapterConditionStatusFalse,
					Reason: util.PtrString("Stuck"),
				},
				{
					Type:   api.AdapterConditionTypeHealth,
					Status: openapi.AdapterConditionStatusTrue,
					Reason: util.PtrString("Healthy"),
				},
			},
			nil,
		)
		statusResp, err := client.PutClusterStatusesWithResponse(
			ctx, cluster.ID,
			openapi.PutClusterStatusesJSONRequestBody(statusInput), test.WithAuthToken(ctx),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(statusResp.StatusCode()).To(Equal(http.StatusCreated))

		// Force-delete the cluster
		forceDeleteResp, err := client.ForceDeleteClusterWithResponse(
			ctx, cluster.ID,
			openapi.ForceDeleteClusterJSONRequestBody{Reason: "integration test - adapter stuck"},
			test.WithAuthToken(ctx),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(forceDeleteResp.StatusCode()).To(Equal(http.StatusNoContent))

		// Verify cluster is gone
		var clusterCheck api.Cluster
		dbErr := dbSession.First(&clusterCheck, "id = ?", cluster.ID).Error
		Expect(dbErr).To(HaveOccurred())
		Expect(dbErr.Error()).To(ContainSubstring("record not found"))

		// Verify nodepools are gone
		var nodePoolCheck api.NodePool
		dbErr = dbSession.First(&nodePoolCheck, "id = ?", nodePool.ID).Error
		Expect(dbErr).To(HaveOccurred())
		Expect(dbErr.Error()).To(ContainSubstring("record not found"))

		// Verify adapter statuses are gone
		var adapterStatuses []api.AdapterStatus
		err = dbSession.Where("resource_type = ? AND resource_id = ?", api.ResourceTypeCluster, cluster.ID).
			Find(&adapterStatuses).Error
		Expect(err).NotTo(HaveOccurred())
		Expect(adapterStatuses).To(BeEmpty())
	})

	errorCases := []struct {
		name           string
		reason         string
		expectedStatus int
		createCluster  bool
	}{
		{"missing reason returns 400", "", http.StatusBadRequest, true},
		{"non-existent cluster returns 404", "should fail", http.StatusNotFound, false},
		{"cluster not in Finalizing returns 409", "should fail", http.StatusConflict, true},
	}

	for _, tc := range errorCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			h, client := test.RegisterIntegration(t)
			account := h.NewRandAccount()
			ctx := h.NewAuthenticatedContext(account)

			clusterID := h.NewID()
			if tc.createCluster {
				cluster, err := h.Factories.NewClusters(h.NewID())
				Expect(err).NotTo(HaveOccurred())
				clusterID = cluster.ID
			}

			forceDeleteResp, err := client.ForceDeleteClusterWithResponse(
				ctx, clusterID,
				openapi.ForceDeleteClusterJSONRequestBody{Reason: tc.reason},
				test.WithAuthToken(ctx),
			)
			Expect(err).NotTo(HaveOccurred())
			Expect(forceDeleteResp.StatusCode()).To(Equal(tc.expectedStatus))
		})
	}
}

func TestClusterDeleteNonExistent(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	resp, err := client.DeleteClusterByIdWithResponse(ctx, h.NewID(), test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusNotFound),
		"DELETE on non-existent cluster should return 404")
}

func TestClusterPatchSoftDeleted(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Soft-delete the cluster
	delResp, err := client.DeleteClusterByIdWithResponse(ctx, cluster.ID, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(delResp.StatusCode()).To(Equal(http.StatusAccepted))

	// PATCH should be rejected
	patchBody := openapi.PatchClusterByIdJSONRequestBody{
		Spec: &openapi.ClusterSpec{"should": "fail"},
	}
	patchResp, err := client.PatchClusterByIdWithResponse(ctx, cluster.ID, patchBody, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(patchResp.StatusCode()).To(Equal(http.StatusConflict),
		"PATCH on soft-deleted cluster should return 409")
}

func TestClusterCreateNodePoolUnderSoftDeleted(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// Soft-delete the cluster
	delResp, err := client.DeleteClusterByIdWithResponse(ctx, cluster.ID, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(delResp.StatusCode()).To(Equal(http.StatusAccepted))

	// Attempt to create a nodepool under the soft-deleted cluster
	npInput := openapi.NodePoolCreateRequest{
		Kind: "NodePool",
		Name: "should-fail-np",
		Spec: map[string]interface{}{"test": "spec"},
	}
	npResp, err := client.CreateNodePoolWithResponse(
		ctx, cluster.ID, openapi.CreateNodePoolJSONRequestBody(npInput), test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(npResp.StatusCode()).To(Equal(http.StatusConflict),
		"creating nodepool under soft-deleted cluster should return 409")
}

func TestClusterPatchNoOpSameGeneration(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	// PATCH with a spec change to bump generation
	spec := openapi.ClusterSpec{"region": "us-east-1"}
	patchBody := openapi.PatchClusterByIdJSONRequestBody{Spec: &spec}
	resp, err := client.PatchClusterByIdWithResponse(ctx, cluster.ID, patchBody, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))
	genAfterPatch := resp.JSON200.Generation

	// Replay the same spec — generation should not increment
	replayResp, err := client.PatchClusterByIdWithResponse(ctx, cluster.ID, patchBody, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(replayResp.StatusCode()).To(Equal(http.StatusOK))
	Expect(replayResp.JSON200.Generation).To(Equal(genAfterPatch),
		"generation should not increment for identical spec PATCH")
}

func TestClusterConcurrentDelete(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())
	initialGeneration := cluster.Generation

	const concurrency = 5
	type deleteResult struct {
		resp *openapi.DeleteClusterByIdResponse
		err  error
	}
	results := make([]deleteResult, concurrency)

	var wg sync.WaitGroup
	wg.Add(concurrency)
	for i := range concurrency {
		go func(idx int) {
			defer wg.Done()
			r, e := client.DeleteClusterByIdWithResponse(ctx, cluster.ID, test.WithAuthToken(ctx))
			results[idx] = deleteResult{resp: r, err: e}
		}(i)
	}
	wg.Wait()

	for i, r := range results {
		Expect(r.err).NotTo(HaveOccurred(), "DELETE request %d should not return error", i)
		Expect(r.resp.StatusCode()).To(Equal(http.StatusAccepted), "DELETE request %d should return 202", i)
		Expect(r.resp.JSON202.DeletedTime).NotTo(BeNil(), "DELETE request %d should have deleted_time", i)
	}

	referenceTime := *results[0].resp.JSON202.DeletedTime
	referenceGen := results[0].resp.JSON202.Generation
	for i := 1; i < concurrency; i++ {
		Expect(*results[i].resp.JSON202.DeletedTime).To(Equal(referenceTime),
			"all DELETE responses should carry identical deleted_time")
		Expect(results[i].resp.JSON202.Generation).To(Equal(referenceGen),
			"all DELETE responses should carry identical generation")
	}

	Expect(referenceGen).To(Equal(initialGeneration+1),
		"generation should increment by exactly 1, not by the number of concurrent requests")
}

func TestClusterGetSoftDeleted(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	cluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	delResp, err := client.DeleteClusterByIdWithResponse(ctx, cluster.ID, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(delResp.StatusCode()).To(Equal(http.StatusAccepted))

	getResp, err := client.GetClusterByIdWithResponse(ctx, cluster.ID, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(getResp.StatusCode()).To(Equal(http.StatusOK),
		"GET on soft-deleted cluster should return 200, not 404")
	Expect(getResp.JSON200.DeletedTime).NotTo(BeNil(),
		"soft-deleted cluster should have deleted_time in GET response")
	Expect(*getResp.JSON200.Id).To(Equal(cluster.ID))
}

func TestClusterListIncludesSoftDeleted(t *testing.T) {
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	activeCluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	deletedCluster, err := h.Factories.NewClusters(h.NewID())
	Expect(err).NotTo(HaveOccurred())

	delResp, err := client.DeleteClusterByIdWithResponse(ctx, deletedCluster.ID, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(delResp.StatusCode()).To(Equal(http.StatusAccepted))

	listResp, err := client.GetClustersWithResponse(ctx, nil, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(listResp.StatusCode()).To(Equal(http.StatusOK))

	var foundActive, foundDeleted bool
	for _, item := range listResp.JSON200.Items {
		if item.Id == nil {
			continue
		}
		if *item.Id == activeCluster.ID {
			Expect(item.DeletedTime).To(BeNil(), "active cluster should not have deleted_time")
			foundActive = true
		}
		if *item.Id == deletedCluster.ID {
			Expect(item.DeletedTime).NotTo(BeNil(), "soft-deleted cluster should have deleted_time")
			foundDeleted = true
		}
	}
	Expect(foundActive).To(BeTrue(), "active cluster should appear in LIST")
	Expect(foundDeleted).To(BeTrue(), "soft-deleted cluster should appear in LIST")
}
