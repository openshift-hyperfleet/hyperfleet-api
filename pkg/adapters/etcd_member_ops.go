package adapters

import (
	"context"
	"fmt"
)

// AddLearner adds a non-voting member unless a member already advertises the
// same peer URL. Repeating the operation after success is a no-op.
func AddLearner(
	ctx context.Context, client MemberClient, name, peerURL string,
) (bool, error) {
	members, err := client.List(ctx)
	if err != nil {
		return false, fmt.Errorf("list etcd members: %w", err)
	}
	for _, member := range members {
		if member.PeerURL == peerURL {
			return false, nil
		}
	}
	if err := client.AddAsLearner(ctx, name, peerURL); err != nil {
		return false, fmt.Errorf("add etcd learner %q: %w", name, err)
	}
	return true, nil
}

// PromoteMember promotes a learner to voting. A member that is already voting
// is left alone.
func PromoteMember(
	ctx context.Context, client MemberClient, ref string,
) (bool, error) {
	member, err := findMember(ctx, client, ref)
	if err != nil {
		return false, err
	}
	if member == nil {
		return false, fmt.Errorf("etcd member %q not found", ref)
	}
	if !member.Learner {
		return false, nil
	}
	if err := client.Promote(ctx, member.ID); err != nil {
		return false, fmt.Errorf("promote etcd member %q: %w", ref, err)
	}
	return true, nil
}

// RemoveMember removes a member by ID or name. An absent member is a no-op.
// Voting removal is refused when it would leave fewer than n/2+1 voters.
func RemoveMember(
	ctx context.Context, client MemberClient, ref string,
) (bool, error) {
	members, err := client.List(ctx)
	if err != nil {
		return false, fmt.Errorf("list etcd members: %w", err)
	}
	member := memberFromRef(members, ref)
	if member == nil {
		return false, nil
	}
	if !member.Learner && !safeToRemoveVoter(members) {
		return false, fmt.Errorf("refusing to remove etcd voter %q: quorum would be lost", ref)
	}
	if err := client.Remove(ctx, member.ID); err != nil {
		return false, fmt.Errorf("remove etcd member %q: %w", ref, err)
	}
	return true, nil
}

// ReleaseMembership removes a deleted cluster's members from etcd. Voters the
// quorum fence protects are left for workload deletion to take down. An
// unreachable etcd has no membership left to release, so it is not an error.
func ReleaseMembership(
	ctx context.Context, client MemberClient, membership Membership,
) error {
	members, err := client.List(ctx)
	if err != nil {
		return nil
	}
	for _, slot := range membership.Slots {
		member := memberFromRef(members, slot.MemberName)
		if member == nil {
			continue
		}
		if !member.Learner && !safeToRemoveVoter(members) {
			continue
		}
		if err := client.Remove(ctx, member.ID); err != nil {
			return fmt.Errorf("release etcd member %q: %w", slot.MemberName, err)
		}
		members = withoutMember(members, member.ID)
	}
	return nil
}

func withoutMember(members []EtcdMember, id string) []EtcdMember {
	remaining := make([]EtcdMember, 0, len(members))
	for _, member := range members {
		if member.ID != id {
			remaining = append(remaining, member)
		}
	}
	return remaining
}

func findMember(
	ctx context.Context, client MemberClient, ref string,
) (*EtcdMember, error) {
	members, err := client.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list etcd members: %w", err)
	}
	return memberFromRef(members, ref), nil
}

func memberFromRef(members []EtcdMember, ref string) *EtcdMember {
	for index := range members {
		if members[index].ID == ref || members[index].Name == ref {
			return &members[index]
		}
	}
	return nil
}

func safeToRemoveVoter(members []EtcdMember) bool {
	voters := 0
	for _, member := range members {
		if !member.Learner {
			voters++
		}
	}
	return voters-1 >= voters/2+1
}
