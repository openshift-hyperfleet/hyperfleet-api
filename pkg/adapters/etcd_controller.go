package adapters

import (
	"context"
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/fleet"
)

// FleetLister returns the management clusters visible to the adapter.
type FleetLister func() ([]fleet.MC, error)

// EtcdController places and applies the etcd member workloads.
type EtcdController struct {
	fleet     FleetLister
	transport Transport
	opts      EtcdOptions
}

var _ Controller = (*EtcdController)(nil)

// NewEtcdController returns an etcd controller backed by the fleet view and transport.
func NewEtcdController(fleetView FleetLister, transport Transport, opts EtcdOptions) *EtcdController {
	return &EtcdController{fleet: fleetView, transport: transport, opts: opts}
}

func (c *EtcdController) Name() string { return EtcdAdapterName }

func (c *EtcdController) Reconcile(
	ctx context.Context, cluster ClusterView,
) (StatusReport, error) {
	if !etcdManaged(cluster.Spec) {
		return notManagedReport(cluster), nil
	}

	mcs, err := c.fleet()
	if err != nil {
		err = fmt.Errorf("list management fleet: %w", err)
		return etcdErrorReport(cluster, err), err
	}
	names := make([]string, len(mcs))
	for index, mc := range mcs {
		names[index] = mc.Name
	}

	targets, err := Place(names, c.opts.Slots, c.opts.PreferredMC)
	if err != nil {
		return etcdErrorReport(cluster, err), err
	}

	membership := NewMembership(cluster.Name, targets, c.opts)
	pki, err := MintEtcdPKI(cluster.Name, membership.Slots, EtcdPKIOptions{})
	if err != nil {
		err = fmt.Errorf("mint etcd PKI: %w", err)
		return etcdErrorReport(cluster, err), err
	}
	byMC, err := RenderEtcdManifestsByMC(membership, pki, c.opts)
	if err != nil {
		return etcdErrorReport(cluster, err), err
	}

	applied := make(map[string]bool, len(byMC))
	for _, slot := range membership.Slots {
		if applied[slot.TargetMC] {
			continue
		}
		if applyErr := c.transport.Apply(ctx, slot.TargetMC, byMC[slot.TargetMC]); applyErr != nil {
			err = fmt.Errorf("apply etcd manifests to %s: %w", slot.TargetMC, applyErr)
			return etcdErrorReport(cluster, err), err
		}
		applied[slot.TargetMC] = true
	}

	return membership.PendingReport(cluster), nil
}

func etcdManaged(spec map[string]interface{}) bool {
	etcd, ok := spec["etcd"].(map[string]interface{})
	if !ok {
		return false
	}
	managed, ok := etcd["managed"].(string)
	return ok && managed == "clm"
}

func etcdErrorReport(cluster ClusterView, err error) StatusReport {
	const reason = "Error"
	message := err.Error()
	return StatusReport{
		ObservedGeneration: cluster.Generation,
		Conditions: []api.AdapterCondition{
			condition(api.AdapterConditionTypeAvailable, api.AdapterConditionFalse, reason, message),
			condition(api.AdapterConditionTypeApplied, api.AdapterConditionFalse, reason, message),
			condition(api.AdapterConditionTypeHealth, api.AdapterConditionFalse, reason, message),
		},
	}
}

func notManagedReport(cluster ClusterView) StatusReport {
	const reason = "NotManaged"
	const message = "Etcd is not managed by CLM"
	return StatusReport{
		ObservedGeneration: cluster.Generation,
		Conditions: []api.AdapterCondition{
			condition(api.AdapterConditionTypeAvailable, api.AdapterConditionTrue, reason, message),
			condition(api.AdapterConditionTypeApplied, api.AdapterConditionTrue, reason, message),
			condition(api.AdapterConditionTypeHealth, api.AdapterConditionTrue, reason, message),
		},
		Data: map[string]interface{}{},
	}
}
