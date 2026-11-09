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
	members   MemberClient
	opts      EtcdOptions
}

var _ Controller = (*EtcdController)(nil)

// NewEtcdController returns an etcd controller backed by the fleet view and transport.
func NewEtcdController(fleetView FleetLister, transport Transport, opts EtcdOptions) *EtcdController {
	return &EtcdController{fleet: fleetView, transport: transport, opts: opts}
}

// WithMembership enables live membership decisions against a quorum client.
func (c *EtcdController) WithMembership(client MemberClient) *EtcdController {
	c.members = client
	return c
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
	// The PoC uses the cluster name as the HostedCluster namespace.
	seam, err := RenderEtcdSeam(membership.Status(nil), pki, cluster.Name)
	if err != nil {
		err = fmt.Errorf("render etcd seam: %w", err)
		return etcdErrorReport(cluster, err), err
	}
	if c.members != nil {
		report, handled, recoverErr := c.reconcileMembership(
			ctx, cluster, membership, pki, names, &seam.Payload,
		)
		if handled {
			return report, recoverErr
		}
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

	return membership.PendingReport(cluster, &seam.Payload), nil
}

func (c *EtcdController) reconcileMembership(
	ctx context.Context,
	cluster ClusterView,
	membership Membership,
	pki *EtcdPKI,
	fleetMCs []string,
	seam *EtcdSeamPayload,
) (StatusReport, bool, error) {
	state, err := ObserveRecoveryState(ctx, c.transport, c.members, membership, fleetMCs)
	if err != nil {
		return etcdErrorReport(cluster, err), true, err
	}
	if len(state.Members) == 0 {
		return StatusReport{}, false, nil
	}

	decision := DecideRecovery(state)
	switch decision.Action {
	case RecoveryNone, RecoveryWait, RecoveryBlocked:
	case RecoveryPromote:
		_, err = PromoteMember(ctx, c.members, decision.Target)
	case RecoveryReplace:
		slot, found := membership.SlotByMemberName(decision.Target)
		if !found {
			err = fmt.Errorf("etcd slot for member %q not found", decision.Target)
			break
		}
		err = ReplaceMember(ctx, c.transport, c.members, slot)
	case RecoveryRepairMCLoss:
		membership, err = RepairMCLoss(
			ctx, c.transport, c.members, pki, membership,
			decision.Target, fleetMCs, c.opts,
		)
	}
	if err != nil {
		return etcdErrorReport(cluster, err), true, err
	}
	report := RecoveryReport(cluster, decision)
	status := membership.Status(healthyMembers(state))
	status.Seam = seam
	report.Data = status.Data()
	return report, true, nil
}

func healthyMembers(state RecoveryState) map[string]bool {
	healthy := make(map[string]bool, len(state.Members))
	for _, member := range state.Members {
		if member.Healthy {
			healthy[member.Name] = true
		}
	}
	return healthy
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
