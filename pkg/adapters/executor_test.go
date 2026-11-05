package adapters

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

type fakeController struct {
	name   string
	calls  *[]string
	err    error
	report StatusReport
}

func (f fakeController) Name() string { return f.name }

func (f fakeController) Reconcile(_ context.Context, cluster ClusterView) (StatusReport, error) {
	*f.calls = append(*f.calls, f.name)
	report := f.report
	if report.ObservedGeneration == 0 {
		report.ObservedGeneration = cluster.Generation
	}
	return report, f.err
}

type recordedReport struct {
	adapter string
	report  StatusReport
}

type fakeReporter struct {
	err     error
	reports []recordedReport
}

func (r *fakeReporter) Report(
	_ context.Context, _ ClusterView, adapter string, report StatusReport,
) error {
	if r.err != nil {
		return r.err
	}
	r.reports = append(r.reports, recordedReport{adapter: adapter, report: report})
	return nil
}

func healthyReport() StatusReport {
	reason := "Fake"
	message := "ok"
	return StatusReport{
		Conditions: []api.AdapterCondition{{
			Type:    api.AdapterConditionTypeAvailable,
			Status:  api.AdapterConditionTrue,
			Reason:  &reason,
			Message: &message,
		}},
	}
}

func conditionByType(report StatusReport, conditionType string) *api.AdapterCondition {
	for i := range report.Conditions {
		if report.Conditions[i].Type == conditionType {
			return &report.Conditions[i]
		}
	}
	return nil
}

func TestExecutorRunsChainInOrderAndReportsEachAdapter(t *testing.T) {
	RegisterTestingT(t)
	calls := []string{}
	var reports fakeReporter
	registry := NewRegistry(map[string]Factory{
		"first": func() Controller {
			return fakeController{name: "first", calls: &calls, report: healthyReport()}
		},
		"second": func() Controller {
			return fakeController{name: "second", calls: &calls, report: healthyReport()}
		},
	})
	executor := NewExecutor(registry, &reports)
	cluster := ClusterView{ID: "cluster-1", Generation: 2}

	err := executor.Run(context.Background(), cluster, []string{"first", "second"})

	Expect(err).ToNot(HaveOccurred())
	Expect(calls).To(Equal([]string{"first", "second"}))
	Expect(reports.reports).To(HaveLen(2))
	Expect(reports.reports[0].adapter).To(Equal("first"))
	Expect(reports.reports[1].adapter).To(Equal("second"))
	for _, reported := range reports.reports {
		Expect(reported.report.ObservedGeneration).To(Equal(cluster.Generation))
		condition := conditionByType(reported.report, api.AdapterConditionTypeAvailable)
		Expect(condition).ToNot(BeNil())
		Expect(condition.Status).To(Equal(api.AdapterConditionTrue))
	}
}

func TestExecutorStopsChainOnAdapterFailure(t *testing.T) {
	RegisterTestingT(t)
	calls := []string{}
	var reports fakeReporter
	reconcileErr := errors.New("boom")
	registry := NewRegistry(map[string]Factory{
		"first": func() Controller {
			return fakeController{name: "first", calls: &calls, err: reconcileErr}
		},
		"second": func() Controller {
			return fakeController{name: "second", calls: &calls, report: healthyReport()}
		},
	})
	executor := NewExecutor(registry, &reports)
	cluster := ClusterView{ID: "cluster-1", Generation: 4}

	err := executor.Run(context.Background(), cluster, []string{"first", "second"})

	Expect(err).To(MatchError(reconcileErr))
	Expect(calls).To(Equal([]string{"first"}))
	Expect(reports.reports).To(HaveLen(1))
	Expect(reports.reports[0].adapter).To(Equal("first"))
	condition := conditionByType(reports.reports[0].report, api.AdapterConditionTypeAvailable)
	Expect(condition).ToNot(BeNil())
	Expect(condition.Status).To(Equal(api.AdapterConditionFalse))
	Expect(condition.Reason).ToNot(BeNil())
	Expect(*condition.Reason).To(Equal("ReconcileFailed"))
	Expect(reports.reports[0].report.ObservedGeneration).To(Equal(cluster.Generation))
}

func TestExecutorIsRetrySafe(t *testing.T) {
	RegisterTestingT(t)
	calls := []string{}
	var reports fakeReporter
	registry := NewRegistry(map[string]Factory{
		"first": func() Controller {
			return fakeController{name: "first", calls: &calls, report: healthyReport()}
		},
		"second": func() Controller {
			return fakeController{name: "second", calls: &calls, report: healthyReport()}
		},
	})
	executor := NewExecutor(registry, &reports)
	cluster := ClusterView{ID: "cluster-1", Generation: 2}
	chain := []string{"first", "second"}

	Expect(executor.Run(context.Background(), cluster, chain)).To(Succeed())
	firstPass := append([]recordedReport(nil), reports.reports...)
	Expect(executor.Run(context.Background(), cluster, chain)).To(Succeed())

	Expect(calls).To(Equal([]string{"first", "second", "first", "second"}))
	Expect(reports.reports).To(HaveLen(4))
	Expect(reports.reports[2:]).To(Equal(firstPass))
}
