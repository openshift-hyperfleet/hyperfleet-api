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

// TestSearchStatusPhaseMapping verifies that status.phase user-friendly syntax
// correctly maps to status_phase database field
func TestSearchStatusPhaseMapping(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create NotReady cluster using new factory method
	notReadyCluster, err := factories.NewClusterWithStatus(&h.Factories, h.DBFactory, h.NewID(), "NotReady", nil)
	Expect(err).NotTo(HaveOccurred())

	// Create Ready cluster
	readyCluster, err := factories.NewClusterWithStatus(&h.Factories, h.DBFactory, h.NewID(), "Ready", nil)
	Expect(err).NotTo(HaveOccurred())

	// Query NotReady clusters using user-friendly syntax
	search := "status.phase='NotReady'"
	list, resp, err := client.DefaultAPI.GetClusters(ctx).Search(search).Execute()

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(list.Total).To(BeNumerically(">=", 1))

	// Verify all returned clusters are NotReady
	foundNotReady := false
	for _, item := range list.Items {
		if *item.Id == notReadyCluster.ID {
			foundNotReady = true
			// Status field structure depends on openapi.yaml
			// Assuming status.phase exists
			Expect(item.Status.Phase).To(Equal(openapi.NOT_READY))
		}
		// Should not contain readyCluster
		Expect(*item.Id).NotTo(Equal(readyCluster.ID))
	}
	Expect(foundNotReady).To(BeTrue(), "Expected to find the NotReady cluster")
}

// TestSearchStatusLastUpdatedTimeMapping verifies that status.last_updated_time
// user-friendly syntax correctly maps to status_last_updated_time database field
// and time comparison works correctly
func TestSearchStatusLastUpdatedTimeMapping(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	now := time.Now()
	oldTime := now.Add(-2 * time.Hour)
	recentTime := now.Add(-30 * time.Minute)

	// Create old cluster (2 hours ago)
	oldCluster, err := factories.NewClusterWithStatus(&h.Factories, h.DBFactory, h.NewID(), "Ready", &oldTime)
	Expect(err).NotTo(HaveOccurred())

	// Create recent cluster (30 minutes ago)
	recentCluster, err := factories.NewClusterWithStatus(&h.Factories, h.DBFactory, h.NewID(), "Ready", &recentTime)
	Expect(err).NotTo(HaveOccurred())

	// Query clusters updated before 1 hour ago
	threshold := now.Add(-1 * time.Hour)
	search := fmt.Sprintf("status.last_updated_time < '%s'", threshold.Format(time.RFC3339))
	list, resp, err := client.DefaultAPI.GetClusters(ctx).Search(search).Execute()

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))

	// Should return at least oldCluster
	Expect(list.Total).To(BeNumerically(">=", 1))

	// Verify oldCluster is in results but recentCluster is not
	foundOld := false
	for _, item := range list.Items {
		if *item.Id == oldCluster.ID {
			foundOld = true
		}
		// Should not contain recentCluster (updated 30 mins ago)
		Expect(*item.Id).NotTo(Equal(recentCluster.ID))
	}
	Expect(foundOld).To(BeTrue(), "Expected to find the old cluster")
}

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
	search := "labels.environment='production'"
	list, resp, err := client.DefaultAPI.GetClusters(ctx).Search(search).Execute()

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
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
	search := "spec = '{}'"
	_, resp, err := client.DefaultAPI.GetClusters(ctx).Search(search).Execute()

	// Should return error
	Expect(err).To(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusBadRequest))
}

// TestSearchCombinedQuery verifies that combined queries (AND/OR)
// work correctly with field mapping
func TestSearchCombinedQuery(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create cluster with NotReady status and us-east region
	matchCluster, err := factories.NewClusterWithStatusAndLabels(
		&h.Factories,
		h.DBFactory,
		h.NewID(),
		"NotReady",
		nil,
		map[string]string{"region": "us-east"},
	)
	Expect(err).NotTo(HaveOccurred())

	// Create cluster with NotReady status but different region
	wrongRegionCluster, err := factories.NewClusterWithStatusAndLabels(
		&h.Factories,
		h.DBFactory,
		h.NewID(),
		"NotReady",
		nil,
		map[string]string{"region": "us-west"},
	)
	Expect(err).NotTo(HaveOccurred())

	// Create cluster with Ready status and us-east region
	wrongStatusCluster, err := factories.NewClusterWithStatusAndLabels(
		&h.Factories,
		h.DBFactory,
		h.NewID(),
		"Ready",
		nil,
		map[string]string{"region": "us-east"},
	)
	Expect(err).NotTo(HaveOccurred())

	// Query using combined AND condition
	search := "status.phase='NotReady' and labels.region='us-east'"
	list, resp, err := client.DefaultAPI.GetClusters(ctx).Search(search).Execute()

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(list.Total).To(BeNumerically(">=", 1))

	// Should only return matchCluster
	foundMatch := false
	for _, item := range list.Items {
		if *item.Id == matchCluster.ID {
			foundMatch = true
			Expect(item.Status.Phase).To(Equal(openapi.NOT_READY))
		}
		// Should not contain wrongRegionCluster or wrongStatusCluster
		Expect(*item.Id).NotTo(Equal(wrongRegionCluster.ID))
		Expect(*item.Id).NotTo(Equal(wrongStatusCluster.ID))
	}
	Expect(foundMatch).To(BeTrue(), "Expected to find the matching cluster")
}

// TestSearchNodePoolFieldMapping verifies that NodePool also supports
// the same field mapping as Cluster
func TestSearchNodePoolFieldMapping(t *testing.T) {
	RegisterTestingT(t)
	h, client := test.RegisterIntegration(t)

	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	// Create NotReady NodePool
	notReadyNP, err := factories.NewNodePoolWithStatus(&h.Factories, h.DBFactory, h.NewID(), "NotReady", nil)
	Expect(err).NotTo(HaveOccurred())

	// Create Ready NodePool
	readyNP, err := factories.NewNodePoolWithStatus(&h.Factories, h.DBFactory, h.NewID(), "Ready", nil)
	Expect(err).NotTo(HaveOccurred())

	// Query NotReady NodePools using user-friendly syntax
	search := "status.phase='NotReady'"
	list, resp, err := client.DefaultAPI.GetNodePools(ctx).Search(search).Execute()

	Expect(err).NotTo(HaveOccurred())
	Expect(resp.StatusCode).To(Equal(http.StatusOK))
	Expect(list.Total).To(BeNumerically(">=", 1))

	// Verify NotReady NodePool is in results
	foundNotReady := false
	for _, item := range list.Items {
		if *item.Id == notReadyNP.ID {
			foundNotReady = true
			Expect(item.Status.Phase).To(Equal(openapi.NOT_READY))
		}
		// Should not contain readyNP
		Expect(*item.Id).NotTo(Equal(readyNP.ID))
	}
	Expect(foundNotReady).To(BeTrue(), "Expected to find the NotReady node pool")

	// Also test labels mapping for NodePools
	npWithLabels, err := factories.NewNodePoolWithLabels(&h.Factories, h.DBFactory, h.NewID(), map[string]string{
		"environment": "test",
	})
	Expect(err).NotTo(HaveOccurred())

	searchLabels := "labels.environment='test'"
	labelsList, labelsResp, labelsErr := client.DefaultAPI.GetNodePools(ctx).Search(searchLabels).Execute()

	Expect(labelsErr).NotTo(HaveOccurred())
	Expect(labelsResp.StatusCode).To(Equal(http.StatusOK))

	foundLabeled := false
	for _, item := range labelsList.Items {
		if *item.Id == npWithLabels.ID {
			foundLabeled = true
		}
	}
	Expect(foundLabeled).To(BeTrue(), "Expected to find the labeled node pool")
}
