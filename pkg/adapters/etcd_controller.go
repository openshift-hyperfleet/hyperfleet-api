package adapters

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/fleet"
)

// FleetLister returns the management clusters visible to the adapter.
type FleetLister func() ([]fleet.MC, error)

// EtcdController places and applies the etcd member workloads.
//
//nolint:govet // fieldalignment: grouping beats padding for readability
type EtcdController struct {
	fleet     FleetLister
	transport Transport
	members   MemberClient
	opts      EtcdOptions
	states    map[string]*etcdState
	live      bool
}

var _ Controller = (*EtcdController)(nil)

// NewEtcdController returns an etcd controller backed by the fleet view and transport.
func NewEtcdController(fleetView FleetLister, transport Transport, opts EtcdOptions) *EtcdController {
	return &EtcdController{
		fleet:     fleetView,
		transport: transport,
		opts:      opts,
		states:    map[string]*etcdState{},
	}
}

// WithMembership enables live membership decisions against a fixed client.
func (c *EtcdController) WithMembership(client MemberClient) *EtcdController {
	c.members = client
	return c
}

// WithLiveMembership builds the etcd client from the cluster's own certs.
func (c *EtcdController) WithLiveMembership() *EtcdController {
	c.live = true
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

	nodeIPs, err := c.nodeAddresses(ctx, names)
	if err != nil {
		return etcdErrorReport(cluster, err), err
	}
	opts := c.opts
	opts.NodeIPs = nodeIPs

	state, created, err := c.ensureState(cluster.Name, names, opts)
	if err != nil {
		return etcdErrorReport(cluster, err), err
	}
	membership := state.Membership
	pki := state.pki()

	// The PoC uses the cluster name as the HostedCluster namespace.
	seam, err := RenderEtcdSeam(membership.Status(nil), pki, cluster.Name)
	if err != nil {
		err = fmt.Errorf("render etcd seam: %w", err)
		return etcdErrorReport(cluster, err), err
	}

	client := c.members
	shouldReconcile := !created || !c.live
	if c.live && shouldReconcile {
		liveClient, clientErr := NewClientV3(membership.Endpoints(), pki)
		if clientErr != nil {
			err = fmt.Errorf("connect to etcd: %w", clientErr)
			return etcdErrorReport(cluster, err), err
		}
		defer func() { _ = liveClient.Close() }()
		client = liveClient
	}
	if client != nil && shouldReconcile {
		started, startErr := membershipStarted(ctx, c.transport, membership, names)
		if startErr != nil {
			return etcdErrorReport(cluster, startErr), startErr
		}
		if started {
			report, handled, recoverErr := c.reconcileMembership(
				ctx, cluster, &membership, pki, client, names, &seam.Payload, opts,
			)
			if handled {
				if recoverErr == nil {
					state.Membership = membership
					state.CA, state.Client = pki.CA, pki.Client
					state.Server, state.Peer = pki.ServerCerts, pki.PeerCerts
					if saveErr := saveEtcdState(c.opts.StateDir, *state); saveErr != nil {
						return etcdErrorReport(cluster, saveErr), saveErr
					}
				}
				return report, recoverErr
			}
		}
	}

	byMC, err := RenderEtcdManifestsByMC(membership, pki, opts)
	if err != nil {
		return etcdErrorReport(cluster, err), err
	}

	applied := make(map[string]bool, len(byMC))
	for _, slot := range membership.Slots {
		if applied[slot.TargetMC] {
			continue
		}
		objects := append([]runtime.Object{memberNamespace(membership)}, byMC[slot.TargetMC]...)
		if applyErr := c.transport.Apply(ctx, slot.TargetMC, objects); applyErr != nil {
			err = fmt.Errorf("apply etcd manifests to %s: %w", slot.TargetMC, applyErr)
			return etcdErrorReport(cluster, err), err
		}
		applied[slot.TargetMC] = true
	}

	return membership.PendingReport(cluster, &seam.Payload), nil
}

func (c *EtcdController) ensureState(
	cluster string, fleetMCs []string, opts EtcdOptions,
) (*etcdState, bool, error) {
	if state, ok := c.states[cluster]; ok {
		return state, false, nil
	}
	state, err := loadEtcdState(c.opts.StateDir, cluster)
	if err != nil {
		return nil, false, err
	}
	if state != nil {
		state.Membership = state.Membership.WithNodeAddresses(opts.NodeIPs)
		c.states[cluster] = state
		return state, false, nil
	}

	targets, err := Place(fleetMCs, opts.Slots, opts.PreferredMC)
	if err != nil {
		return nil, false, err
	}
	membership := NewMembership(cluster, targets, opts)
	pki, err := MintEtcdPKI(cluster, membership.Slots, EtcdPKIOptions{})
	if err != nil {
		return nil, false, fmt.Errorf("mint etcd PKI: %w", err)
	}
	created := stateFrom(membership, pki)
	if err := saveEtcdState(c.opts.StateDir, created); err != nil {
		return nil, false, err
	}
	c.states[cluster] = &created
	return &created, true, nil
}

func (c *EtcdController) nodeAddresses(
	ctx context.Context, fleetMCs []string,
) (map[string]string, error) {
	addresser, ok := c.transport.(NodeAddresser)
	if !ok {
		return nil, nil
	}
	addresses := make(map[string]string, len(fleetMCs))
	for _, mc := range fleetMCs {
		ip, err := addresser.NodeAddress(ctx, mc)
		if err != nil {
			return nil, fmt.Errorf("resolve node address for %s: %w", mc, err)
		}
		addresses[mc] = ip
	}
	return addresses, nil
}

func membershipStarted(
	ctx context.Context, transport Transport, membership Membership, fleetMCs []string,
) (bool, error) {
	inFleet := make(map[string]bool, len(fleetMCs))
	for _, mc := range fleetMCs {
		inFleet[mc] = true
	}
	for _, slot := range membership.Slots {
		if !inFleet[slot.TargetMC] {
			continue
		}
		selector := EtcdMemberLabel + "=" + slot.Name
		pods, err := transport.List(ctx, slot.TargetMC, kindPod, slot.Namespace, selector)
		if err != nil {
			return false, fmt.Errorf("list etcd pods on %s: %w", slot.TargetMC, err)
		}
		for _, pod := range pods {
			if pod.Ready {
				return true, nil
			}
		}
	}
	return false, nil
}

func (c *EtcdController) reconcileMembership(
	ctx context.Context,
	cluster ClusterView,
	membership *Membership,
	pki *EtcdPKI,
	client MemberClient,
	fleetMCs []string,
	seam *EtcdSeamPayload,
	opts EtcdOptions,
) (StatusReport, bool, error) {
	state, err := ObserveRecoveryState(ctx, c.transport, client, *membership, fleetMCs)
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
		_, err = PromoteMember(ctx, client, decision.Target)
	case RecoveryReplace:
		slot, found := membership.SlotByMemberName(decision.Target)
		if !found {
			err = fmt.Errorf("etcd slot for member %q not found", decision.Target)
			break
		}
		err = ReplaceMember(ctx, c.transport, client, slot)
	case RecoveryRepairMCLoss:
		*membership, err = RepairMCLoss(
			ctx, c.transport, client, pki, *membership,
			decision.Target, fleetMCs, opts,
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
