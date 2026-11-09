package adapters

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"
)

func threeVoters() *fakeMemberClient {
	return &fakeMemberClient{members: map[string]EtcdMember{
		"id-1": voter("id-1", "my-cluster-etcd-m0"),
		"id-2": voter("id-2", "my-cluster-etcd-m1"),
		"id-3": voter("id-3", "my-cluster-etcd-m2"),
	}}
}

func TestReplaceMemberDeletesDataThenReAddsLearner(t *testing.T) {
	RegisterTestingT(t)
	client := threeVoters()
	transport := &recordingTransport{applied: map[string][]runtime.Object{}}
	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})
	slot := membership.Slots[1]
	ctx := context.Background()

	Expect(ReplaceMember(ctx, transport, client, slot)).To(Succeed())

	Expect(transport.deleted).To(Equal([]string{
		"mc-b/Pod/etcd-my-cluster/my-cluster-etcd-m1",
		"mc-b/PersistentVolumeClaim/etcd-my-cluster/data-my-cluster-etcd-m1",
	}))
	Expect(client.calls).To(Equal([]string{
		"remove:id-2",
		"learner:my-cluster-etcd-m1:https://my-cluster-etcd-m1.etcd-my-cluster.svc:2380",
	}))
	for _, member := range client.members {
		if member.Name == slot.MemberName {
			Expect(member.Learner).To(BeTrue())
		}
	}
}

func TestReplaceMemberIsIdempotentAfterLearnerExists(t *testing.T) {
	RegisterTestingT(t)
	client := threeVoters()
	transport := &recordingTransport{applied: map[string][]runtime.Object{}}
	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})
	slot := membership.Slots[1]
	ctx := context.Background()
	Expect(ReplaceMember(ctx, transport, client, slot)).To(Succeed())

	calls, deletes := len(client.calls), len(transport.deleted)
	Expect(ReplaceMember(ctx, transport, client, slot)).To(Succeed())

	Expect(client.calls).To(HaveLen(calls))
	Expect(transport.deleted).To(HaveLen(deletes))
}

func TestRepairMCLossRemovesDeadMemberAndRetargetsSlot(t *testing.T) {
	RegisterTestingT(t)
	client := threeVoters()
	transport := &recordingTransport{applied: map[string][]runtime.Object{}}
	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})
	pki, err := MintEtcdPKI("my-cluster", membership.Slots, EtcdPKIOptions{})
	Expect(err).ToNot(HaveOccurred())

	updated, err := RepairMCLoss(
		context.Background(), transport, client, pki, membership,
		"my-cluster-etcd-m2", []string{"mc-a", "mc-b", "mc-d"}, EtcdOptions{},
	)

	Expect(err).ToNot(HaveOccurred())
	Expect(membership.Slots[2].TargetMC).To(Equal("mc-c"))
	Expect(updated.Slots[2].TargetMC).To(Equal("mc-d"))
	Expect(client.calls).To(Equal([]string{"remove:id-3"}))
	Expect(client.members).To(HaveLen(2))
	Expect(transport.applied["mc-d"]).To(HaveLen(etcdManifestsPerMember))
	Expect(findObject(transport.applied["mc-d"], kindStatefulSet, "my-cluster-etcd-m2")).
		ToNot(BeNil())
	for _, call := range client.calls {
		Expect(call).ToNot(HavePrefix("promote:"))
	}
}

func TestRepairMCLossRefusesWithoutASpareMC(t *testing.T) {
	RegisterTestingT(t)
	client := threeVoters()
	transport := &recordingTransport{applied: map[string][]runtime.Object{}}
	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})
	pki, err := MintEtcdPKI("my-cluster", membership.Slots, EtcdPKIOptions{})
	Expect(err).ToNot(HaveOccurred())

	_, err = RepairMCLoss(
		context.Background(), transport, client, pki, membership,
		"my-cluster-etcd-m2", []string{"mc-a", "mc-b"}, EtcdOptions{},
	)

	Expect(err).To(MatchError(ContainSubstring("no healthy management cluster")))
	Expect(client.calls).To(BeEmpty())
	Expect(transport.applied).To(BeEmpty())
}
