package adapters

import (
	"context"
	"fmt"
	"sort"

	"k8s.io/apimachinery/pkg/runtime"
)

// ReplaceMember deletes the failing member's pod and data, removes its stale
// membership entry, then re-adds it as a learner. The member's reset container
// re-joins it when the pod starts again.
func ReplaceMember(
	ctx context.Context, transport Transport, client MemberClient, slot MemberSlot,
) error {
	members, err := client.List(ctx)
	if err != nil {
		return fmt.Errorf("list etcd members: %w", err)
	}
	if member := memberFromRef(members, slot.MemberName); member != nil &&
		member.Learner && member.PeerURL == slot.PeerURL {
		return nil
	}

	podErr := transport.Delete(ctx, slot.TargetMC, kindPod, slot.Namespace, slot.MemberName)
	if podErr != nil {
		return fmt.Errorf("delete etcd pod %q: %w", slot.MemberName, podErr)
	}
	pvc := etcdDataVolume + "-" + slot.MemberName
	pvcErr := transport.Delete(ctx, slot.TargetMC, kindPersistentVolumeClaim, slot.Namespace, pvc)
	if pvcErr != nil {
		return fmt.Errorf("delete etcd PVC %q: %w", pvc, pvcErr)
	}
	if _, err := RemoveMember(ctx, client, slot.MemberName); err != nil {
		return err
	}
	if _, err := AddLearner(ctx, client, slot.MemberName, slot.PeerURL); err != nil {
		return err
	}
	return nil
}

// RepairMCLoss removes the dead member, updates its slot to a healthy MC with
// no surviving member, and re-renders that member's manifests. The dead member
// is removed before the replacement reaches the transport; promotion is left to
// a later fenced pass.
func RepairMCLoss(
	ctx context.Context,
	transport Transport,
	client MemberClient,
	pki *EtcdPKI,
	membership Membership,
	deadMember string,
	fleetMCs []string,
	opts EtcdOptions,
) (Membership, error) {
	opts = opts.withDefaults()
	index := slotIndex(membership, deadMember)
	if index < 0 {
		return membership, fmt.Errorf("etcd slot for member %q not found", deadMember)
	}
	replacement, ok := replacementMC(membership, index, fleetMCs)
	if !ok {
		return membership, fmt.Errorf("no healthy management cluster for etcd member %q", deadMember)
	}

	if _, err := RemoveMember(ctx, client, deadMember); err != nil {
		return membership, err
	}

	updated := membership
	updated.Slots = append([]MemberSlot(nil), membership.Slots...)
	updated.Slots[index].TargetMC = replacement
	if ip := opts.NodeIPs[replacement]; ip != "" {
		applyNodeAddress(&updated.Slots[index], ip)
	}
	if err := pki.RemintMember(updated.Slots[index]); err != nil {
		return membership, fmt.Errorf("re-mint etcd certs for %q: %w", deadMember, err)
	}
	objects, err := renderMemberManifests(updated, updated.Slots[index], pki, opts)
	if err != nil {
		return membership, err
	}
	objects = append([]runtime.Object{memberNamespace(updated)}, objects...)
	if err := transport.Apply(ctx, replacement, objects); err != nil {
		return membership, fmt.Errorf("apply etcd replacement to %s: %w", replacement, err)
	}
	// Every member's initial-cluster must reflect the new slot address, or a
	// later restart of a survivor fails etcd's peer URL validation.
	for i, slot := range updated.Slots {
		if i == index {
			continue
		}
		memberObjects, err := renderMemberManifests(updated, slot, pki, opts)
		if err != nil {
			return membership, err
		}
		memberObjects = append([]runtime.Object{memberNamespace(updated)}, memberObjects...)
		if err := transport.Apply(ctx, slot.TargetMC, memberObjects); err != nil {
			return membership, fmt.Errorf("apply updated etcd member to %s: %w", slot.TargetMC, err)
		}
	}
	return updated, nil
}

func slotIndex(membership Membership, memberName string) int {
	for index, slot := range membership.Slots {
		if slot.MemberName == memberName {
			return index
		}
	}
	return -1
}

func replacementMC(
	membership Membership, deadIndex int, fleetMCs []string,
) (string, bool) {
	used := make(map[string]bool, len(membership.Slots))
	for index, slot := range membership.Slots {
		if index != deadIndex {
			used[slot.TargetMC] = true
		}
	}
	deadMC := membership.Slots[deadIndex].TargetMC
	candidates := make([]string, 0, len(fleetMCs))
	for _, mc := range fleetMCs {
		if mc != deadMC && !used[mc] {
			candidates = append(candidates, mc)
		}
	}
	if len(candidates) == 0 {
		return "", false
	}
	sort.Strings(candidates)
	return candidates[0], true
}

// ObserveRecoveryState reads live membership, pod health, and rollout state.
// An unreachable MC is reported as lost rather than failing the whole read.
func ObserveRecoveryState(
	ctx context.Context,
	transport Transport,
	client MemberClient,
	membership Membership,
	fleetMCs []string,
) (RecoveryState, error) {
	live, err := client.List(ctx)
	if err != nil {
		return RecoveryState{}, fmt.Errorf("list etcd members: %w", err)
	}
	if len(live) == 0 {
		return RecoveryState{}, nil
	}

	byName := make(map[string]EtcdMember, len(live))
	for _, member := range live {
		byName[member.Name] = member
	}
	fleet := fleetSet(fleetMCs)
	state := RecoveryState{FleetMCs: fleetMCs}
	for _, slot := range membership.Slots {
		if fleet != nil {
			if _, ok := fleet[slot.TargetMC]; !ok {
				state.Members = append(state.Members, MemberObservation{
					Name: slot.MemberName,
					MC:   slot.TargetMC,
				})
				continue
			}
		}
		member, err := observeMember(ctx, transport, slot, byName[slot.MemberName])
		if err != nil {
			return RecoveryState{}, err
		}
		if member.Rollout {
			state.RolloutInProgress = true
		}
		state.Members = append(state.Members, member.Observation)
	}
	return state, nil
}

type memberObservation struct {
	Observation MemberObservation
	Rollout     bool
}

func observeMember(
	ctx context.Context,
	transport Transport,
	slot MemberSlot,
	member EtcdMember,
) (memberObservation, error) {
	selector := EtcdMemberLabel + "=" + slot.Name
	pods, err := transport.List(ctx, slot.TargetMC, kindPod, slot.Namespace, selector)
	if err != nil {
		return memberObservation{}, fmt.Errorf(
			"list etcd pods on %s: %w", slot.TargetMC, err,
		)
	}
	healthy := false
	for _, pod := range pods {
		if pod.Ready {
			healthy = true
			break
		}
	}
	sets, err := transport.List(ctx, slot.TargetMC, kindStatefulSet, slot.Namespace, selector)
	if err != nil {
		return memberObservation{}, fmt.Errorf(
			"list etcd statefulsets on %s: %w", slot.TargetMC, err,
		)
	}
	rollout := len(sets) == 0 || !sets[0].Ready
	return memberObservation{
		Observation: MemberObservation{
			Name:     slot.MemberName,
			MC:       slot.TargetMC,
			Healthy:  healthy,
			Learner:  member.Learner,
			CaughtUp: member.Learner && healthy,
		},
		Rollout: rollout,
	}, nil
}

func fleetSet(fleetMCs []string) map[string]bool {
	if fleetMCs == nil {
		return nil
	}
	fleet := make(map[string]bool, len(fleetMCs))
	for _, mc := range fleetMCs {
		fleet[mc] = true
	}
	return fleet
}
