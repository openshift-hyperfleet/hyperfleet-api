// Package orchestrator contains the in-server drift detection and
// reconciliation loop.
package orchestrator

import (
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

// NeedsReconcile ports sentinel's default drift rules: brand-new resources,
// generation mismatch, debounced failures, and stale periodic health checks.
// Age comes from last_updated_time, which advances on every adapter report.
func NeedsReconcile(resource *api.Resource, now time.Time, debounce, maxAge time.Duration) bool {
	if resource == nil {
		return false
	}

	reconciled, found := reconciledCondition(resource.Conditions)

	if resource.Generation == 1 && !found {
		return true
	}
	if resource.Generation > reconciled.ObservedGeneration {
		return true
	}
	if !found {
		return false
	}

	age := now.Sub(reconciled.LastUpdatedTime)
	if reconciled.Status == api.ConditionFalse && age > debounce {
		return true
	}
	return reconciled.Status == api.ConditionTrue && age > maxAge
}

func reconciledCondition(conditions []api.ResourceCondition) (api.ResourceCondition, bool) {
	for _, condition := range conditions {
		if condition.Type == api.ResourceConditionTypeReconciled {
			return condition, true
		}
	}
	return api.ResourceCondition{}, false
}
