package adapters

import (
	"context"
	"encoding/json"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

type serviceReporter struct {
	resources services.ResourceService
	now       func() time.Time
}

var _ StatusReporter = (*serviceReporter)(nil)

// NewServiceReporter returns a StatusReporter that writes through the same
// ProcessAdapterStatus path used by the REST status handler.
func NewServiceReporter(resources services.ResourceService) StatusReporter {
	return &serviceReporter{resources: resources, now: time.Now}
}

func (r *serviceReporter) Report(
	ctx context.Context, cluster ClusterView, adapter string, report StatusReport,
) error {
	now := r.now()

	conditions := make([]api.AdapterCondition, len(report.Conditions))
	copy(conditions, report.Conditions)
	for i := range conditions {
		if conditions[i].LastTransitionTime.IsZero() {
			conditions[i].LastTransitionTime = now
		}
	}
	conditionsJSON, err := json.Marshal(conditions)
	if err != nil {
		return errors.GeneralError("failed to marshal adapter conditions: %s", err)
	}

	data := report.Data
	if data == nil {
		data = map[string]interface{}{}
	}
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return errors.GeneralError("failed to marshal adapter data: %s", err)
	}

	status := &api.AdapterStatus{
		Adapter:            adapter,
		ObservedGeneration: report.ObservedGeneration,
		Conditions:         conditionsJSON,
		Data:               dataJSON,
		CreatedTime:        now,
		LastReportTime:     now,
	}

	if _, svcErr := r.resources.ProcessAdapterStatus(
		ctx, api.ResourceTypeCluster, cluster.ID, status,
	); svcErr != nil {
		return svcErr
	}
	return nil
}
