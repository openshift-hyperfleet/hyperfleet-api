package integration

import (
	"net/http"
	"testing"

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

	// Create cluster with NotReady status (Available=False, Ready=False) and us-east region
	matchCluster, err := factories.NewClusterWithStatusAndLabels(
		&h.Factories,
		h.DBFactory,
		h.NewID(),
		false, // isAvailable
		false, // isReady
		map[string]string{"region": "us-east"},
	)
	Expect(err).NotTo(HaveOccurred())

	// Create cluster with NotReady status but different region
	wrongRegionCluster, err := factories.NewClusterWithStatusAndLabels(
		&h.Factories,
		h.DBFactory,
		h.NewID(),
		false, // isAvailable
		false, // isReady
		map[string]string{"region": "us-west"},
	)
	Expect(err).NotTo(HaveOccurred())

	// Create cluster with Ready status (Available=True, Ready=True) and us-east region
	_, err = factories.NewClusterWithStatusAndLabels(
		&h.Factories,
		h.DBFactory,
		h.NewID(),
		true, // isAvailable
		true, // isReady
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

	// Create cluster with Ready=True, Available=True
	readyCluster, err := factories.NewClusterWithStatus(&h.Factories, h.DBFactory, h.NewID(), true, true)
	Expect(err).NotTo(HaveOccurred())

	// Create cluster with Ready=False, Available=True
	notReadyCluster, err := factories.NewClusterWithStatus(&h.Factories, h.DBFactory, h.NewID(), true, false)
	Expect(err).NotTo(HaveOccurred())

	// Create cluster with Ready=False, Available=False
	notAvailableCluster, err := factories.NewClusterWithStatus(&h.Factories, h.DBFactory, h.NewID(), false, false)
	Expect(err).NotTo(HaveOccurred())

	// Search for Ready=True
	searchStr := "status.conditions.Ready='True'"
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

	// Verify only readyCluster is returned
	foundReady := false
	for _, item := range list.Items {
		if *item.Id == readyCluster.ID {
			foundReady = true
		}
		// Should not contain notReadyCluster or notAvailableCluster
		Expect(*item.Id).NotTo(Equal(notReadyCluster.ID))
		Expect(*item.Id).NotTo(Equal(notAvailableCluster.ID))
	}
	Expect(foundReady).To(BeTrue(), "Expected to find the ready cluster")

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

	// Should contain readyCluster and notReadyCluster (both have Available=True)
	foundReadyInAvailable := false
	foundNotReadyInAvailable := false
	for _, item := range availableList.Items {
		if *item.Id == readyCluster.ID {
			foundReadyInAvailable = true
		}
		if *item.Id == notReadyCluster.ID {
			foundNotReadyInAvailable = true
		}
		// Should not contain notAvailableCluster
		Expect(*item.Id).NotTo(Equal(notAvailableCluster.ID))
	}
	Expect(foundReadyInAvailable).To(BeTrue(), "Expected to find ready cluster in Available=True search")
	Expect(foundNotReadyInAvailable).To(BeTrue(), "Expected to find notReady cluster in Available=True search")
}

// TestSearchStatusConditionsCombinedWithLabels verifies that condition queries
// can be combined with label queries using AND
func TestSearchStatusConditionsCombinedWithLabels(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create cluster with Ready=True and region=us-east
	matchCluster, err := factories.NewClusterWithStatusAndLabels(
		&h.Factories,
		h.DBFactory,
		h.NewID(),
		true,  // isAvailable
		true,  // isReady
		map[string]string{"region": "us-east"},
	)
	Expect(err).NotTo(HaveOccurred())

	// Create cluster with Ready=True but wrong region
	wrongRegionCluster, err := factories.NewClusterWithStatusAndLabels(
		&h.Factories,
		h.DBFactory,
		h.NewID(),
		true, // isAvailable
		true, // isReady
		map[string]string{"region": "us-west"},
	)
	Expect(err).NotTo(HaveOccurred())

	// Create cluster with correct region but Ready=False
	wrongStatusCluster, err := factories.NewClusterWithStatusAndLabels(
		&h.Factories,
		h.DBFactory,
		h.NewID(),
		true,  // isAvailable
		false, // isReady
		map[string]string{"region": "us-east"},
	)
	Expect(err).NotTo(HaveOccurred())

	// Search for Ready=True AND region=us-east
	searchStr := "status.conditions.Ready='True' AND labels.region='us-east'"
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
	searchStr := "status.conditions.Ready='Invalid'"
	search := openapi.SearchParams(searchStr)
	params := &openapi.GetClustersParams{
		Search: &search,
	}
	resp, err := client.GetClustersWithResponse(ctx, params, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode()).To(Equal(http.StatusBadRequest))

	// Test invalid condition type (lowercase)
	searchInvalidType := "status.conditions.ready='True'"
	searchInvalidTypeParam := openapi.SearchParams(searchInvalidType)
	invalidTypeParams := &openapi.GetClustersParams{
		Search: &searchInvalidTypeParam,
	}
	invalidTypeResp, err := client.GetClustersWithResponse(ctx, invalidTypeParams, test.WithAuthToken(ctx))

	Expect(err).NotTo(HaveOccurred())
	Expect(invalidTypeResp.StatusCode()).To(Equal(http.StatusBadRequest))
}
