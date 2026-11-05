package adapters

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

const NoopAdapterName = "noop"

// NoopController proves the SPI without touching any infrastructure.
type NoopController struct{}

var _ Controller = NoopController{}

func (NoopController) Name() string {
	return NoopAdapterName
}

func (NoopController) Reconcile(_ context.Context, cluster ClusterView) (StatusReport, error) {
	return StatusReport{
		ObservedGeneration: cluster.Generation,
		Conditions: []api.AdapterCondition{
			noopCondition(api.AdapterConditionTypeAvailable),
			noopCondition(api.AdapterConditionTypeApplied),
			noopCondition(api.AdapterConditionTypeHealth),
			noopCondition(api.AdapterConditionTypeFinalized),
		},
		Data: map[string]interface{}{},
	}, nil
}

func noopCondition(conditionType string) api.AdapterCondition {
	reason := "NoOp"
	message := "No action required"
	return api.AdapterCondition{
		Type:    conditionType,
		Status:  api.AdapterConditionTrue,
		Reason:  &reason,
		Message: &message,
	}
}
