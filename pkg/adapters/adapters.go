package adapters

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

// ClusterView is the read-only cluster state handed to a controller.
type ClusterView struct {
	Spec       map[string]interface{}
	ID         string
	Name       string
	Generation int32
	Deleting   bool
}

// StatusReport is the result of one idempotent reconcile pass.
type StatusReport struct {
	Data               map[string]interface{}
	Conditions         []api.AdapterCondition
	ObservedGeneration int32
}

// Controller is the adapter SPI. Reconcile must be idempotent and report
// status for the generation it observed.
type Controller interface {
	Name() string
	Reconcile(ctx context.Context, cluster ClusterView) (StatusReport, error)
}

// StatusReporter persists an adapter's report. The PoC implementation calls
// the service layer directly; an external implementation can use the REST
// status endpoint instead.
type StatusReporter interface {
	Report(ctx context.Context, cluster ClusterView, adapter string, report StatusReport) error
}
