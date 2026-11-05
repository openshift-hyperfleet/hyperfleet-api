package adapters

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

const NoopAdapterName = "noop"

// NoopController proves the SPI without touching any infrastructure.
// AdapterName lets it stand in for a configured adapter until the real one lands.
type NoopController struct {
	AdapterName string
}

var _ Controller = NoopController{}

func (c NoopController) Name() string {
	if c.AdapterName != "" {
		return c.AdapterName
	}
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
