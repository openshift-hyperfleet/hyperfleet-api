package adapters

import (
	"context"
	"fmt"
)

// EtcdMember is the membership subset the adapter reads from etcd.
type EtcdMember struct {
	ID      string
	Name    string
	PeerURL string
	Learner bool
}

// MemberClient is the etcd membership API used by the reset-member and
// replacement operations.
type MemberClient interface {
	List(ctx context.Context) ([]EtcdMember, error)
	Remove(ctx context.Context, id string) error
	Add(ctx context.Context, name, peerURL string) error
	AddAsLearner(ctx context.Context, name, peerURL string) error
	Promote(ctx context.Context, id string) error
}

// Rejoin removes any stale member with this name, then re-adds it. It does
// nothing when the member has data or the cluster is not reachable. The
// reset-member init container mirrors this logic in shell.
func Rejoin(
	ctx context.Context, client MemberClient, name, peerURL string, hasData bool,
) (bool, error) {
	if hasData {
		return false, nil
	}
	members, err := client.List(ctx)
	if err != nil {
		return false, nil
	}
	for _, member := range members {
		if member.Name != name {
			continue
		}
		if err := client.Remove(ctx, member.ID); err != nil {
			return false, fmt.Errorf("remove stale etcd member %q: %w", name, err)
		}
	}
	if err := client.Add(ctx, name, peerURL); err != nil {
		return false, fmt.Errorf("add etcd member %q: %w", name, err)
	}
	return true, nil
}
