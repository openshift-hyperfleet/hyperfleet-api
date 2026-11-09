package adapters

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
)

//nolint:govet // fieldalignment: test fake reads naturally in this order
type fakeMemberClient struct {
	members     map[string]EtcdMember
	calls       []string
	listErr     error
	failAddOnce bool
}

func (f *fakeMemberClient) List(context.Context) ([]EtcdMember, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	members := make([]EtcdMember, 0, len(f.members))
	for _, member := range f.members {
		members = append(members, member)
	}
	return members, nil
}

func (f *fakeMemberClient) Remove(_ context.Context, id string) error {
	delete(f.members, id)
	f.calls = append(f.calls, "remove:"+id)
	return nil
}

func (f *fakeMemberClient) Add(_ context.Context, name, peerURL string) error {
	if f.failAddOnce {
		f.failAddOnce = false
		f.calls = append(f.calls, "add-failed:"+name)
		return fmt.Errorf("add failed")
	}
	id := fmt.Sprintf("id-%d", len(f.members)+1)
	f.members[id] = EtcdMember{ID: id, Name: name}
	f.calls = append(f.calls, "add:"+name+":"+peerURL)
	return nil
}

func TestRejoinRemovesStaleMemberAndAddsItBack(t *testing.T) {
	RegisterTestingT(t)
	client := &fakeMemberClient{members: map[string]EtcdMember{
		"old": {ID: "old", Name: "my-cluster-etcd-m0"},
	}}

	acted, err := Rejoin(
		context.Background(), client, "my-cluster-etcd-m0", "https://peer:2380", false,
	)

	Expect(err).ToNot(HaveOccurred())
	Expect(acted).To(BeTrue())
	Expect(client.calls).To(Equal([]string{
		"remove:old",
		"add:my-cluster-etcd-m0:https://peer:2380",
	}))
	Expect(client.members).To(HaveLen(1))
	for _, member := range client.members {
		Expect(member.Name).To(Equal("my-cluster-etcd-m0"))
	}
}

func TestRejoinRetriesAfterAddFailure(t *testing.T) {
	RegisterTestingT(t)
	client := &fakeMemberClient{
		members: map[string]EtcdMember{
			"old": {ID: "old", Name: "my-cluster-etcd-m0"},
		},
		failAddOnce: true,
	}

	_, err := Rejoin(context.Background(), client, "my-cluster-etcd-m0", "https://peer:2380", false)
	Expect(err).To(HaveOccurred())

	acted, err := Rejoin(context.Background(), client, "my-cluster-etcd-m0", "https://peer:2380", false)
	Expect(err).ToNot(HaveOccurred())
	Expect(acted).To(BeTrue())
	Expect(client.calls).To(Equal([]string{
		"remove:old",
		"add-failed:my-cluster-etcd-m0",
		"add:my-cluster-etcd-m0:https://peer:2380",
	}))
	Expect(client.members).To(HaveLen(1))
}

func TestRejoinSkipsWhenMemberHasData(t *testing.T) {
	RegisterTestingT(t)
	client := &fakeMemberClient{members: map[string]EtcdMember{
		"old": {ID: "old", Name: "my-cluster-etcd-m0"},
	}}

	acted, err := Rejoin(context.Background(), client, "my-cluster-etcd-m0", "https://peer:2380", true)

	Expect(err).ToNot(HaveOccurred())
	Expect(acted).To(BeFalse())
	Expect(client.calls).To(BeEmpty())
}

func TestRejoinSkipsWhenClusterIsUnreachable(t *testing.T) {
	RegisterTestingT(t)
	client := &fakeMemberClient{members: map[string]EtcdMember{}, listErr: fmt.Errorf("down")}

	acted, err := Rejoin(context.Background(), client, "my-cluster-etcd-m0", "https://peer:2380", false)

	Expect(err).ToNot(HaveOccurred())
	Expect(acted).To(BeFalse())
	Expect(client.calls).To(BeEmpty())
}
