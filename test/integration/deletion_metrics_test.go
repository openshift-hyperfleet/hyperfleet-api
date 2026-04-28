package integration

import (
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/metrics"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

func TestPendingDeletionCollector_Integration(t *testing.T) {
	t.Run("given soft-deleted resources older than threshold, collector reports them as stuck", func(t *testing.T) {
		RegisterTestingT(t)
		h, client := test.RegisterIntegration(t)
		account := h.NewRandAccount()
		ctx := h.NewAuthenticatedContext(account)

		cluster, err := h.Factories.NewClusters(h.NewID())
		Expect(err).NotTo(HaveOccurred())
		npResp, err := client.CreateNodePoolWithResponse(
			ctx, cluster.ID, openapi.CreateNodePoolJSONRequestBody(newNodePoolInput("stuck-np")),
			test.WithAuthToken(ctx),
		)
		Expect(err).NotTo(HaveOccurred())
		Expect(npResp.StatusCode()).To(Equal(http.StatusCreated))

		_, err = client.DeleteClusterByIdWithResponse(ctx, cluster.ID, test.WithAuthToken(ctx))
		Expect(err).NotTo(HaveOccurred())

		// Backdate deleted_time to 1 hour ago so resources exceed the 30m threshold
		db := h.DBFactory.New(ctx)
		pastTime := time.Now().UTC().Add(-1 * time.Hour)
		Expect(db.Exec("UPDATE clusters SET deleted_time = ? WHERE id = ?", pastTime, cluster.ID).Error).NotTo(HaveOccurred())
		err = db.Exec("UPDATE node_pools SET deleted_time = ? WHERE owner_id = ?", pastTime, cluster.ID).Error
		Expect(err).NotTo(HaveOccurred())

		rawDB := h.DBFactory.DirectDB()
		collector := metrics.NewPendingDeletionCollector(rawDB, 30*time.Minute)

		collected := collectStuckMetrics(t, collector)

		Expect(collected["cluster"]).To(BeNumerically(">=", 1), "should report at least 1 stuck cluster")
		Expect(collected["nodepool"]).To(BeNumerically(">=", 1), "should report at least 1 stuck nodepool")
	})

	t.Run("given soft-deleted resources within threshold, collector reports zero stuck", func(t *testing.T) {
		RegisterTestingT(t)
		h, client := test.RegisterIntegration(t)
		account := h.NewRandAccount()
		ctx := h.NewAuthenticatedContext(account)

		cluster, err := h.Factories.NewClusters(h.NewID())
		Expect(err).NotTo(HaveOccurred())

		_, err = client.DeleteClusterByIdWithResponse(ctx, cluster.ID, test.WithAuthToken(ctx))
		Expect(err).NotTo(HaveOccurred())

		rawDB := h.DBFactory.DirectDB()
		// Use a very long threshold so the just-deleted cluster is NOT stuck
		collector := metrics.NewPendingDeletionCollector(rawDB, 24*time.Hour)

		collected := collectStuckMetrics(t, collector)

		Expect(collected["cluster"]).To(Equal(0.0), "recently deleted cluster should not be stuck")
	})
}

func collectStuckMetrics(t *testing.T, collector *metrics.PendingDeletionCollector) map[string]float64 {
	t.Helper()
	RegisterTestingT(t)

	ch := make(chan prometheus.Metric, 10)
	collector.Collect(ch)
	close(ch)

	result := make(map[string]float64)
	for m := range ch {
		pb := &dto.Metric{}
		Expect(m.Write(pb)).To(Succeed())
		for _, lp := range pb.GetLabel() {
			if lp.GetName() == "resource_type" {
				result[lp.GetValue()] = pb.GetGauge().GetValue()
			}
		}
	}
	return result
}
