package orchestrator

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

const (
	testDebounce = 10 * time.Second
	testMaxAge   = 30 * time.Minute
)

func TestNeedsReconcile(t *testing.T) {
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		resource *api.Resource
		name     string
		skip     string
		want     bool
	}{
		{
			name:     "Test 1: reconciled resource with recent check - skip",
			resource: withReconciled(2, api.ConditionTrue, now.Add(-5*time.Minute), 2),
		},
		{
			name:     "Test 2: not-reconciled resource with debounce elapsed - publish",
			resource: withReconciled(2, api.ConditionFalse, now.Add(-15*time.Second), 2),
			want:     true,
		},
		{
			name:     "Test 3: not-reconciled resource within debounce period - skip",
			resource: withReconciled(2, api.ConditionFalse, now.Add(-5*time.Second), 2),
		},
		{
			name:     "Test 4: reconciled resource with stale check - publish",
			resource: withReconciled(2, api.ConditionTrue, now.Add(-31*time.Minute), 2),
			want:     true,
		},
		{
			name:     "Test 5: brand-new resource with generation 1 - publish",
			resource: withoutConditions(1),
			want:     true,
		},
		{
			name:     "Test 6: not-reconciled resource due to generation mismatch - publish",
			resource: withReconciled(2, api.ConditionFalse, now.Add(-15*time.Second), 1),
			want:     true,
		},
		{
			name:     "Test 7: missing Reconciled condition on generation 1 - publish",
			resource: withoutConditions(1),
			want:     true,
		},
		{
			name: "Test 8: CEL expression compilation failure at startup",
			skip: "predicate has no CEL expressions to compile",
		},
		{
			name: "Test 9: circular param dependency at startup",
			skip: "predicate has no CEL params",
		},
		{
			name:     "Test 10: missing Reconciled condition with generation greater than 1 - publish",
			resource: withoutConditions(2),
			want:     true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			if tc.skip != "" {
				t.Skip(tc.skip)
			}
			Expect(NeedsReconcile(tc.resource, now, testDebounce, testMaxAge)).To(Equal(tc.want))
		})
	}
}

func withReconciled(
	generation int32, status api.ResourceConditionStatus, updated time.Time, observed int32,
) *api.Resource {
	return &api.Resource{
		Generation: generation,
		Conditions: []api.ResourceCondition{{
			Type:               api.ResourceConditionTypeReconciled,
			Status:             status,
			LastUpdatedTime:    updated,
			ObservedGeneration: observed,
		}},
	}
}

func withoutConditions(generation int32) *api.Resource {
	return &api.Resource{Generation: generation}
}
