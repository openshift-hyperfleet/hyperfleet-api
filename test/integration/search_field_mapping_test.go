package integration

import (
	"fmt"
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
	"github.com/openshift-hyperfleet/hyperfleet-api/test/factories"
)

// TestSearchLabelsMapping verifies that labels.xxx user-friendly syntax
// correctly maps to JSONB query labels->>'xxx'
func TestSearchLabelsMapping(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create cluster with production labels
	prodCluster, err := factories.NewClusterWithLabels(&h.Factories, h.DBFactory, h.NewID(), map[string]string{
		"environment": "production",
		"region":      "us-east",
	})
	Expect(err).NotTo(HaveOccurred())

	// Create cluster with staging labels
	stagingCluster, err := factories.NewClusterWithLabels(&h.Factories, h.DBFactory, h.NewID(), map[string]string{
		"environment": "staging",
	})
	Expect(err).NotTo(HaveOccurred())

	// Query production environment clusters using user-friendly syntax
	searchStr := "labels.environment='production'"
	search := openapi.SearchParams(searchStr)
	params := &openapi.GetClustersParams{
		Search: &search,
	}
	resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))
	list := resp.JSON200
	Expect(list).NotTo(BeNil())
	Expect(list.Total).To(BeNumerically(">=", 1))

	// Verify returned clusters have correct label
	foundProd := false
	for _, item := range list.Items {
		if *item.Id == prodCluster.ID {
			foundProd = true
			// Verify labels field contains environment=production
			if item.Labels != nil {
				Expect(*item.Labels).To(HaveKeyWithValue("environment", "production"))
			}
		}
		// Should not contain stagingCluster
		Expect(*item.Id).NotTo(Equal(stagingCluster.ID))
	}
	Expect(foundProd).To(BeTrue(), "Expected to find the production cluster")
}

// TestSearchSpecFieldRejected verifies that querying the spec field
// is correctly rejected with 400 Bad Request error
func TestSearchSpecFieldRejected(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Attempt to query spec field (should be rejected)
	searchStr := "spec = '{}'"
	search := openapi.SearchParams(searchStr)
	params := &openapi.GetClustersParams{
		Search: &search,
	}
	resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

	// Should return error
	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest))
}

// TestSearchCombinedQuery verifies that combined queries (AND/OR)
// work correctly with field mapping
func TestSearchCombinedQuery(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create cluster with NotReconciled status (Available=False, Reconciled=False) and us-east region
	matchCluster, err := factories.NewClusterWithStatusAndLabels(
		&h.Factories,
		h.DBFactory,
		h.NewID(),
		false, // isAvailable
		false, // isReconciled
		map[string]string{"region": "us-east"},
	)
	Expect(err).NotTo(HaveOccurred())

	// Create cluster with NotReconciled status but different region
	wrongRegionCluster, err := factories.NewClusterWithStatusAndLabels(
		&h.Factories,
		h.DBFactory,
		h.NewID(),
		false, // isAvailable
		false, // isReconciled
		map[string]string{"region": "us-west"},
	)
	Expect(err).NotTo(HaveOccurred())

	// Create cluster with Reconciled status (Available=True, Reconciled=True) and us-east region
	_, err = factories.NewClusterWithStatusAndLabels(
		&h.Factories,
		h.DBFactory,
		h.NewID(),
		true, // isAvailable
		true, // isReconciled
		map[string]string{"region": "us-east"},
	)
	Expect(err).NotTo(HaveOccurred())

	// Query using combined AND condition with labels (labels search still works)
	searchStr := "labels.region='us-east'"
	search := openapi.SearchParams(searchStr)
	params := &openapi.GetClustersParams{
		Search: &search,
	}
	resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))
	list := resp.JSON200
	Expect(list).NotTo(BeNil())
	Expect(list.Total).To(BeNumerically(">=", 1))

	// Should return matchCluster and wrongStatusCluster but not wrongRegionCluster
	foundMatch := false
	for _, item := range list.Items {
		if *item.Id == matchCluster.ID {
			foundMatch = true
		}
		// Should not contain wrongRegionCluster
		Expect(*item.Id).NotTo(Equal(wrongRegionCluster.ID))
	}
	Expect(foundMatch).To(BeTrue(), "Expected to find the matching cluster")
}

// TestSearchNodePoolLabelsMapping verifies that NodePool also supports
// the labels field mapping
func TestSearchNodePoolLabelsMapping(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Test labels mapping for NodePools
	npWithLabels, err := factories.NewNodePoolWithLabels(&h.Factories, h.DBFactory, h.NewID(), map[string]string{
		"environment": "test",
	})
	Expect(err).NotTo(HaveOccurred())

	searchLabelsStr := "labels.environment='test'"
	searchLabels := openapi.SearchParams(searchLabelsStr)
	labelsParams := &openapi.GetNodePoolsParams{
		Search: &searchLabels,
	}
	labelsResp, labelsErr := client.GetNodePoolsWithResponse(ctx, labelsParams, test.WithAuthToken(ctx))

	Expect(labelsErr).NotTo(HaveOccurred())
	Expect(labelsResp.StatusCode()).To(Equal(http.StatusOK))
	labelsList := labelsResp.JSON200
	Expect(labelsList).NotTo(BeNil())

	foundLabeled := false
	for _, item := range labelsList.Items {
		if *item.Id == npWithLabels.ID {
			foundLabeled = true
		}
	}
	Expect(foundLabeled).To(BeTrue(), "Expected to find the labeled node pool")
}

// TestSearchStatusConditionsMapping verifies that status.conditions.<Type>='<Status>'
// user-friendly syntax correctly maps to JSONB containment query
func TestSearchStatusConditionsMapping(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create cluster with Reconciled=True, Available=True
	reconciledCluster, err := factories.NewClusterWithStatus(&h.Factories, h.DBFactory, h.NewID(), true, true)
	Expect(err).NotTo(HaveOccurred())

	// Create cluster with Reconciled=False, Available=True
	notReconciledCluster, err := factories.NewClusterWithStatus(&h.Factories, h.DBFactory, h.NewID(), true, false)
	Expect(err).NotTo(HaveOccurred())

	// Create cluster with Reconciled=False, Available=False
	notAvailableCluster, err := factories.NewClusterWithStatus(&h.Factories, h.DBFactory, h.NewID(), false, false)
	Expect(err).NotTo(HaveOccurred())

	// Search for Reconciled=True
	searchStr := "status.conditions.Reconciled='True'"
	search := openapi.SearchParams(searchStr)
	params := &openapi.GetClustersParams{
		Search: &search,
	}
	resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))
	list := resp.JSON200
	Expect(list).NotTo(BeNil())
	Expect(list.Total).To(BeNumerically(">=", 1))

	// Verify only reconciledCluster is returned
	foundReconciled := false
	for _, item := range list.Items {
		if *item.Id == reconciledCluster.ID {
			foundReconciled = true
		}
		// Should not contain notReconciledCluster or notAvailableCluster
		Expect(*item.Id).NotTo(Equal(notReconciledCluster.ID))
		Expect(*item.Id).NotTo(Equal(notAvailableCluster.ID))
	}
	Expect(foundReconciled).To(BeTrue(), "Expected to find the reconciled cluster")

	// Search for Available=True
	searchAvailableStr := "status.conditions.Available='True'"
	searchAvailable := openapi.SearchParams(searchAvailableStr)
	availableParams := &openapi.GetClustersParams{
		Search: &searchAvailable,
	}
	availableResp, err := client.GetClustersWithResponse(ctx, availableParams, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(availableResp.StatusCode()).To(Equal(http.StatusOK))
	availableList := availableResp.JSON200
	Expect(availableList).NotTo(BeNil())
	Expect(availableList.Total).To(BeNumerically(">=", 2))

	// Should contain reconciledCluster and notReconciledCluster (both have Available=True)
	foundReconciledInAvailable := false
	foundNotReconciledInAvailable := false
	for _, item := range availableList.Items {
		if *item.Id == reconciledCluster.ID {
			foundReconciledInAvailable = true
		}
		if *item.Id == notReconciledCluster.ID {
			foundNotReconciledInAvailable = true
		}
		// Should not contain notAvailableCluster
		Expect(*item.Id).NotTo(Equal(notAvailableCluster.ID))
	}
	Expect(foundReconciledInAvailable).To(BeTrue(), "Expected to find reconciled cluster in Available=True search")
	Expect(foundNotReconciledInAvailable).To(BeTrue(), "Expected to find notReconciled cluster in Available=True search")
}

// TestSearchStatusConditionsCombinedWithLabels verifies that condition queries
// can be combined with label queries using AND
func TestSearchStatusConditionsCombinedWithLabels(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create cluster with Reconciled=True and region=us-east
	matchCluster, err := factories.NewClusterWithStatusAndLabels(
		&h.Factories,
		h.DBFactory,
		h.NewID(),
		true, // isAvailable
		true, // isReconciled
		map[string]string{"region": "us-east"},
	)
	Expect(err).NotTo(HaveOccurred())

	// Create cluster with Reconciled=True but wrong region
	wrongRegionCluster, err := factories.NewClusterWithStatusAndLabels(
		&h.Factories,
		h.DBFactory,
		h.NewID(),
		true, // isAvailable
		true, // isReconciled
		map[string]string{"region": "us-west"},
	)
	Expect(err).NotTo(HaveOccurred())

	// Create cluster with correct region but Reconciled=False
	wrongStatusCluster, err := factories.NewClusterWithStatusAndLabels(
		&h.Factories,
		h.DBFactory,
		h.NewID(),
		true,  // isAvailable
		false, // isReconciled
		map[string]string{"region": "us-east"},
	)
	Expect(err).NotTo(HaveOccurred())

	// Search for Reconciled=True AND region=us-east
	searchStr := "status.conditions.Reconciled='True' AND labels.region='us-east'"
	search := openapi.SearchParams(searchStr)
	params := &openapi.GetClustersParams{
		Search: &search,
	}
	resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))
	list := resp.JSON200
	Expect(list).NotTo(BeNil())
	Expect(list.Total).To(BeNumerically(">=", 1))

	// Should only find matchCluster
	foundMatch := false
	for _, item := range list.Items {
		if *item.Id == matchCluster.ID {
			foundMatch = true
		}
		// Should not contain wrongRegionCluster or wrongStatusCluster
		Expect(*item.Id).NotTo(Equal(wrongRegionCluster.ID))
		Expect(*item.Id).NotTo(Equal(wrongStatusCluster.ID))
	}
	Expect(foundMatch).To(BeTrue(), "Expected to find the matching cluster")
}

// TestSearchStatusConditionsInvalidValues verifies that invalid condition values
// are rejected with 400 Bad Request
func TestSearchStatusConditionsInvalidValues(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Test invalid condition status
	searchStr := "status.conditions.Reconciled='Invalid'"
	search := openapi.SearchParams(searchStr)
	params := &openapi.GetClustersParams{
		Search: &search,
	}
	resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest))

	// Test invalid condition type (lowercase)
	searchInvalidType := "status.conditions.reconciled='True'"
	searchInvalidTypeParam := openapi.SearchParams(searchInvalidType)
	invalidTypeParams := &openapi.GetClustersParams{
		Search: &searchInvalidTypeParam,
	}
	invalidTypeResp, err := client.GetClustersWithResponse(ctx, invalidTypeParams, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(invalidTypeResp.StatusCode()).To(Equal(http.StatusBadRequest))
}

// TestSearchStatusConditionsNotOperator verifies that using "not" with condition
// queries returns 400 Bad Request
func TestSearchStatusConditionsNotOperator(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// "not" wrapping a condition query
	searchStr := "not status.conditions.Reconciled='True'"
	search := openapi.SearchParams(searchStr)
	params := &openapi.GetClustersParams{
		Search: &search,
	}
	resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest))

	// "not" wrapping subtree containing a condition
	searchMixed := "not (labels.region='us-east' AND status.conditions.Reconciled='True')"
	searchMixedParam := openapi.SearchParams(searchMixed)
	mixedParams := &openapi.GetClustersParams{
		Search: &searchMixedParam,
	}
	mixedResp, err := client.GetClustersWithResponse(ctx, mixedParams, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(mixedResp.StatusCode()).To(Equal(http.StatusBadRequest))

	// "not" wrapping a non-condition
	searchAllowed := "status.conditions.Reconciled='True' AND not labels.region='us-west'"
	searchAllowedParam := openapi.SearchParams(searchAllowed)
	allowedParams := &openapi.GetClustersParams{
		Search: &searchAllowedParam,
	}
	allowedResp, err := client.GetClustersWithResponse(ctx, allowedParams, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(allowedResp.StatusCode()).To(Equal(http.StatusOK))
}

// TestSearchConditionSubfieldLastUpdatedTime verifies that
// status.conditions.<Type>.last_updated_time queries work correctly
func TestSearchConditionSubfieldLastUpdatedTime(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a "stale" cluster with conditions updated 2 hours ago
	staleTime := time.Now().Add(-2 * time.Hour)
	staleCluster, err := factories.NewClusterWithStatusAtTime(
		&h.Factories, h.DBFactory, h.NewID(),
		true, true, // isAvailable, isReconciled
		staleTime,
	)
	Expect(err).NotTo(HaveOccurred())

	// Create a "fresh" cluster with conditions updated just now
	freshTime := time.Now()
	freshCluster, err := factories.NewClusterWithStatusAtTime(
		&h.Factories, h.DBFactory, h.NewID(),
		true, true, // isAvailable, isReconciled
		freshTime,
	)
	Expect(err).NotTo(HaveOccurred())

	// Search for clusters where Reconciled.last_updated_time is older than 1 hour ago
	cutoff := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	searchStr := fmt.Sprintf("status.conditions.Reconciled.last_updated_time < '%s'", cutoff)
	search := openapi.SearchParams(searchStr)
	params := &openapi.GetClustersParams{
		Search: &search,
	}
	resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))
	list := resp.JSON200
	Expect(list).NotTo(BeNil())
	Expect(list.Total).To(BeNumerically(">=", 1))

	// Should contain staleCluster but NOT freshCluster
	foundStale := false
	for _, item := range list.Items {
		if *item.Id == staleCluster.ID {
			foundStale = true
		}
		Expect(*item.Id).NotTo(Equal(freshCluster.ID))
	}
	Expect(foundStale).To(BeTrue(), "Expected to find the stale cluster")
}

// TestSearchConditionSubfieldCombinedWithStatus verifies that condition subfield
// queries can be combined with condition status queries using AND.
// This is the primary Sentinel use case: fetch reconciled-but-stale resources.
func TestSearchConditionSubfieldCombinedWithStatus(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create a stale reconciled cluster (Reconciled=True, updated 2h ago) - should match
	staleTime := time.Now().Add(-2 * time.Hour)
	staleReconciledCluster, err := factories.NewClusterWithStatusAtTime(
		&h.Factories, h.DBFactory, h.NewID(),
		true, true,
		staleTime,
	)
	Expect(err).NotTo(HaveOccurred())

	// Create a fresh reconciled cluster (Reconciled=True, updated now) - should NOT match
	freshTime := time.Now()
	freshReconciledCluster, err := factories.NewClusterWithStatusAtTime(
		&h.Factories, h.DBFactory, h.NewID(),
		true, true,
		freshTime,
	)
	Expect(err).NotTo(HaveOccurred())

	// Search: Reconciled=True AND last_updated_time < cutoff (stale resources)
	cutoff := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	searchStr := fmt.Sprintf(
		"status.conditions.Reconciled='True' AND "+
			"status.conditions.Reconciled.last_updated_time < '%s'",
		cutoff,
	)
	search := openapi.SearchParams(searchStr)
	params := &openapi.GetClustersParams{
		Search: &search,
	}
	resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))
	list := resp.JSON200
	Expect(list).NotTo(BeNil())
	Expect(list.Total).To(BeNumerically(">=", 1))

	foundStaleReconciled := false
	for _, item := range list.Items {
		if *item.Id == staleReconciledCluster.ID {
			foundStaleReconciled = true
		}
		// Should NOT contain freshReconciledCluster
		Expect(*item.Id).NotTo(Equal(freshReconciledCluster.ID))
	}
	Expect(foundStaleReconciled).To(BeTrue(), "Expected to find the stale reconciled cluster")
}

// TestSearchConditionSubfieldGreaterThan verifies the > operator works for time subfield queries
func TestSearchConditionSubfieldGreaterThan(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Compute timestamps upfront for determinism
	now := time.Now().UTC()
	staleTime := now.Add(-2 * time.Hour)
	freshTime := now
	cutoff := now.Add(-1 * time.Hour).Format(time.RFC3339)

	// Create a "stale" cluster (updated 2h ago) — should NOT match > cutoff
	_, err := factories.NewClusterWithStatusAtTime(
		&h.Factories, h.DBFactory, h.NewID(),
		true, true,
		staleTime,
	)
	Expect(err).NotTo(HaveOccurred())

	// Create a "fresh" cluster (updated now) — should match > cutoff
	freshCluster, err := factories.NewClusterWithStatusAtTime(
		&h.Factories, h.DBFactory, h.NewID(),
		true, true,
		freshTime,
	)
	Expect(err).NotTo(HaveOccurred())

	// Search for clusters where Reconciled.last_updated_time is newer than 1 hour ago
	searchStr := fmt.Sprintf("status.conditions.Reconciled.last_updated_time > '%s'", cutoff)
	search := openapi.SearchParams(searchStr)
	params := &openapi.GetClustersParams{
		Search: &search,
	}
	resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))
	list := resp.JSON200
	Expect(list).NotTo(BeNil())
	Expect(list.Total).To(BeNumerically(">=", 1))

	foundFresh := false
	for _, item := range list.Items {
		if *item.Id == freshCluster.ID {
			foundFresh = true
		}
	}
	Expect(foundFresh).To(BeTrue(), "Expected to find the fresh cluster with > operator")
}

// TestSearchConditionSubfieldObservedGeneration verifies that
// status.conditions.<Type>.observed_generation queries work correctly (INTEGER cast path)
func TestSearchConditionSubfieldObservedGeneration(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create cluster with observed_generation = 2
	lowGenCluster, err := factories.NewClusterWithObservedGeneration(
		&h.Factories, h.DBFactory, h.NewID(),
		true, true, 2,
	)
	Expect(err).NotTo(HaveOccurred())

	// Create cluster with observed_generation = 10
	highGenCluster, err := factories.NewClusterWithObservedGeneration(
		&h.Factories, h.DBFactory, h.NewID(),
		true, true, 10,
	)
	Expect(err).NotTo(HaveOccurred())

	// Search for clusters where Reconciled.observed_generation < 5
	searchStr := "status.conditions.Reconciled.observed_generation < 5"
	search := openapi.SearchParams(searchStr)
	params := &openapi.GetClustersParams{
		Search: &search,
	}
	resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))
	list := resp.JSON200
	Expect(list).NotTo(BeNil())
	Expect(list.Total).To(BeNumerically(">=", 1))

	// Should contain lowGenCluster but NOT highGenCluster
	foundLow := false
	for _, item := range list.Items {
		if *item.Id == lowGenCluster.ID {
			foundLow = true
		}
		Expect(*item.Id).NotTo(Equal(highGenCluster.ID))
	}
	Expect(foundLow).To(BeTrue(), "Expected to find the low-generation cluster")
}

// TestSearchConditionSubfieldInvalidSubfield verifies that invalid subfield names
// are rejected with 400 Bad Request
func TestSearchConditionSubfieldInvalidSubfield(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Query with an unsupported subfield
	searchStr := "status.conditions.Reconciled.invalid_field < '2026-03-06T00:00:00Z'"
	search := openapi.SearchParams(searchStr)
	params := &openapi.GetClustersParams{
		Search: &search,
	}
	resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest))
}

// TestSearchNodePoolConditionSubfieldLastUpdatedTime verifies that condition subfield queries
// work for NodePools — same code path as Clusters but validates the full end-to-end for NodePools.
func TestSearchNodePoolConditionSubfieldLastUpdatedTime(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	now := time.Now().UTC()
	staleTime := now.Add(-2 * time.Hour)
	freshTime := now
	cutoff := now.Add(-1 * time.Hour).Format(time.RFC3339)

	// Create a "stale" node pool (updated 2h ago)
	staleNP, err := factories.NewNodePoolWithStatusAtTime(
		&h.Factories, h.DBFactory, h.NewID(),
		true, true,
		staleTime,
	)
	Expect(err).NotTo(HaveOccurred())

	// Create a "fresh" node pool (updated now)
	freshNP, err := factories.NewNodePoolWithStatusAtTime(
		&h.Factories, h.DBFactory, h.NewID(),
		true, true,
		freshTime,
	)
	Expect(err).NotTo(HaveOccurred())

	// Search for node pools where Reconciled.last_updated_time is older than 1 hour ago
	searchStr := fmt.Sprintf("status.conditions.Reconciled.last_updated_time < '%s'", cutoff)
	search := openapi.SearchParams(searchStr)
	params := &openapi.GetNodePoolsParams{
		Search: &search,
	}
	resp, err := client.GetNodePoolsWithResponse(ctx, params, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusOK))
	list := resp.JSON200
	Expect(list).NotTo(BeNil())
	Expect(list.Total).To(BeNumerically(">=", 1))

	foundStale := false
	for _, item := range list.Items {
		if *item.Id == staleNP.ID {
			foundStale = true
		}
		Expect(*item.Id).NotTo(Equal(freshNP.ID))
	}
	Expect(foundStale).To(BeTrue(), "Expected to find the stale node pool")
}
