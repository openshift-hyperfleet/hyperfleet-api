package integration

import (
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/metrics"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
	"github.com/openshift-hyperfleet/hyperfleet-api/test/factories"
)

const (
	resourceCluster = "cluster"
	isDeleteFalse   = "false"
	isDeleteTrue    = "true"
)

func TestReconciliationCollector_Integration(t *testing.T) {
	t.Run("pending reconciliation resources are reported with correct labels", func(t *testing.T) {
		RegisterTestingT(t)
		h, _ := test.RegisterIntegration(t)

		pastTime := time.Now().UTC().Add(-5 * time.Minute)
		_, err := factories.NewClusterWithStatusAtTime(&h.Factories, h.DBFactory, h.NewID(), false, false, pastTime)
		Expect(err).NotTo(HaveOccurred())

		rawDB := h.DBFactory.DirectDB()
		collector := metrics.NewReconciliationCollector(rawDB, 30*time.Minute)

		collected := collectReconciliationMetrics(t, collector)

		pending := collected["pending"]
		Expect(pending).NotTo(BeEmpty(), "should report pending reconciliation metrics")

		var found bool
		for _, m := range pending {
			if m.labels["resource_type"] == resourceCluster && m.labels["is_delete"] == isDeleteFalse {
				found = true
				Expect(m.value).To(BeNumerically(">=", 1))
			}
		}
		Expect(found).To(BeTrue(), "should report pending cluster with is_delete=false")
	})

	t.Run("stuck resources beyond threshold are reported", func(t *testing.T) {
		RegisterTestingT(t)
		h, _ := test.RegisterIntegration(t)

		pastTime := time.Now().UTC().Add(-1 * time.Hour)
		_, err := factories.NewClusterWithStatusAtTime(&h.Factories, h.DBFactory, h.NewID(), false, false, pastTime)
		Expect(err).NotTo(HaveOccurred())

		rawDB := h.DBFactory.DirectDB()
		collector := metrics.NewReconciliationCollector(rawDB, 30*time.Minute)

		collected := collectReconciliationMetrics(t, collector)

		stuck := collected["stuck"]
		Expect(stuck).NotTo(BeEmpty(), "should report stuck reconciliation metrics")

		var found bool
		for _, m := range stuck {
			if m.labels["resource_type"] == resourceCluster && m.labels["is_delete"] == isDeleteFalse {
				found = true
				Expect(m.value).To(BeNumerically(">=", 1))
			}
		}
		Expect(found).To(BeTrue(), "should report stuck cluster")
	})

	t.Run("resources within threshold are pending but not stuck", func(t *testing.T) {
		RegisterTestingT(t)
		h, _ := test.RegisterIntegration(t)

		recentTime := time.Now().UTC().Add(-2 * time.Minute)
		_, err := factories.NewClusterWithStatusAtTime(&h.Factories, h.DBFactory, h.NewID(), false, false, recentTime)
		Expect(err).NotTo(HaveOccurred())

		rawDB := h.DBFactory.DirectDB()
		collector := metrics.NewReconciliationCollector(rawDB, 30*time.Minute)

		collected := collectReconciliationMetrics(t, collector)

		pending := collected["pending"]
		var pendingCount float64
		for _, m := range pending {
			if m.labels["resource_type"] == resourceCluster && m.labels["is_delete"] == isDeleteFalse {
				pendingCount = m.value
			}
		}
		Expect(pendingCount).To(BeNumerically(">=", 1), "resource should be pending")

		stuck := collected["stuck"]
		var stuckCount float64
		for _, m := range stuck {
			if m.labels["resource_type"] == resourceCluster && m.labels["is_delete"] == isDeleteFalse {
				stuckCount = m.value
			}
		}
		Expect(stuckCount).To(Equal(0.0), "recent resource should not be stuck")
	})

	t.Run("max stuck duration is reported for stuck resources", func(t *testing.T) {
		RegisterTestingT(t)
		h, _ := test.RegisterIntegration(t)

		pastTime := time.Now().UTC().Add(-1 * time.Hour)
		_, err := factories.NewClusterWithStatusAtTime(&h.Factories, h.DBFactory, h.NewID(), false, false, pastTime)
		Expect(err).NotTo(HaveOccurred())

		rawDB := h.DBFactory.DirectDB()
		collector := metrics.NewReconciliationCollector(rawDB, 30*time.Minute)

		collected := collectReconciliationMetrics(t, collector)

		duration := collected["duration"]
		Expect(duration).NotTo(BeEmpty(), "should report stuck duration metrics")

		var found bool
		for _, m := range duration {
			if m.labels["resource_type"] == resourceCluster && m.labels["is_delete"] == isDeleteFalse {
				found = true
				Expect(m.value).To(BeNumerically(">=", 3500), "stuck duration should be ~3600 seconds")
			}
		}
		Expect(found).To(BeTrue(), "should report stuck duration for cluster")
	})

	t.Run("soft-deleted resources are reported with is_delete=true", func(t *testing.T) {
		RegisterTestingT(t)
		h, _ := test.RegisterIntegration(t)

		pastTime := time.Now().UTC().Add(-1 * time.Hour)
		cluster, err := factories.NewClusterWithStatusAtTime(&h.Factories, h.DBFactory, h.NewID(), false, false, pastTime)
		Expect(err).NotTo(HaveOccurred())

		ctx := h.NewAuthenticatedContext(h.NewRandAccount())
		db := h.DBFactory.New(ctx)
		deletedTime := time.Now().UTC().Add(-1 * time.Hour)
		err = db.Exec(
			"UPDATE resources SET deleted_time = ? WHERE id = ?", deletedTime, cluster.ID,
		).Error
		Expect(err).NotTo(HaveOccurred())

		rawDB := h.DBFactory.DirectDB()
		collector := metrics.NewReconciliationCollector(rawDB, 30*time.Minute)

		collected := collectReconciliationMetrics(t, collector)

		pending := collected["pending"]
		var found bool
		for _, m := range pending {
			if m.labels["resource_type"] == resourceCluster && m.labels["is_delete"] == isDeleteTrue {
				found = true
				Expect(m.value).To(BeNumerically(">=", 1))
			}
		}
		Expect(found).To(BeTrue(), "should report soft-deleted cluster with is_delete=true")
	})
}

type collectedMetric struct {
	labels map[string]string
	value  float64
}

func collectReconciliationMetrics(
	t *testing.T, collector *metrics.ReconciliationCollector,
) map[string][]collectedMetric {
	t.Helper()
	RegisterTestingT(t)

	ch := make(chan prometheus.Metric, 20)
	collector.Collect(ch)
	close(ch)

	result := map[string][]collectedMetric{
		"pending":  {},
		"stuck":    {},
		"duration": {},
	}

	for m := range ch {
		pb := &dto.Metric{}
		Expect(m.Write(pb)).To(Succeed())

		desc := m.Desc().String()
		labels := make(map[string]string)
		for _, lp := range pb.GetLabel() {
			labels[lp.GetName()] = lp.GetValue()
		}

		cm := collectedMetric{labels: labels, value: pb.GetGauge().GetValue()}

		switch {
		case strings.Contains(desc, "stuck_duration_seconds"):
			result["duration"] = append(result["duration"], cm)
		case strings.Contains(desc, "reconciliation_stuck"):
			result["stuck"] = append(result["stuck"], cm)
		case strings.Contains(desc, "pending_reconciliation"):
			result["pending"] = append(result["pending"], cm)
		}
	}
	return result
}
