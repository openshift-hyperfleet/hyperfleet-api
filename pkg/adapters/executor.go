package adapters

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

// Precondition gates an adapter's turn in the chain. The PoC does not set one;
// ordering is the only rule.
type Precondition func(ctx context.Context, cluster ClusterView, adapter string) bool

// Executor runs an ordered adapter chain against one cluster.
type Executor struct {
	registry     *Registry
	reporter     StatusReporter
	precondition Precondition
}

// NewExecutor returns an executor that reports through reporter.
func NewExecutor(registry *Registry, reporter StatusReporter) *Executor {
	return &Executor{
		registry:     registry,
		reporter:     reporter,
		precondition: func(context.Context, ClusterView, string) bool { return true },
	}
}

// Run executes the chain in order. A failed adapter stops the chain after its
// failure report is persisted.
func (e *Executor) Run(ctx context.Context, cluster ClusterView, chain []string) error {
	controllers, err := e.registry.Resolve(chain)
	if err != nil {
		return err
	}

	for _, controller := range controllers {
		if !e.precondition(ctx, cluster, controller.Name()) {
			continue
		}

		report, reconcileErr := controller.Reconcile(ctx, cluster)
		if reconcileErr != nil {
			report = failureReport(report, cluster, reconcileErr)
		}
		if reportErr := e.reporter.Report(ctx, cluster, controller.Name(), report); reportErr != nil {
			return reportErr
		}
		if reconcileErr != nil {
			return reconcileErr
		}
	}
	return nil
}

// failureReport makes an adapter failure visible in aggregated status while
// preserving any conditions the adapter already reported.
func failureReport(report StatusReport, cluster ClusterView, err error) StatusReport {
	if report.ObservedGeneration == 0 {
		report.ObservedGeneration = cluster.Generation
	}
	reason := "ReconcileFailed"
	message := err.Error()
	failed := api.AdapterCondition{
		Type:    api.AdapterConditionTypeAvailable,
		Status:  api.AdapterConditionFalse,
		Reason:  &reason,
		Message: &message,
	}
	for i := range report.Conditions {
		if report.Conditions[i].Type == api.AdapterConditionTypeAvailable {
			report.Conditions[i] = failed
			return report
		}
	}
	report.Conditions = append(report.Conditions, failed)
	return report
}
