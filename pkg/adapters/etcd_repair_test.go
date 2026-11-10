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
	transport := &recordingTransport{
		applied: map[string][]runtime.Object{},
		listed: map[string][]ManifestState{
			"mc-a/StatefulSet/etcd.hyperfleet.io/member=m0": {{Ready: true}},
			"mc-b/StatefulSet/etcd.hyperfleet.io/member=m1": {{Ready: true}},
		},
	}
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
	Expect(transport.applied["mc-d"]).To(HaveLen(etcdManifestsPerMember + 1))
	Expect(findObject(transport.applied["mc-d"], kindStatefulSet, "my-cluster-etcd-m2")).
		ToNot(BeNil())
	Expect(transport.applied["mc-a"]).To(HaveLen(etcdManifestsPerMember + 1))
	Expect(transport.applied["mc-b"]).To(HaveLen(etcdManifestsPerMember + 1))
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

type repairRaceTransport struct {
	client   *fakeMemberClient
	events   []string
	ready    bool
	diverged bool
}

func (t *repairRaceTransport) Apply(_ context.Context, mc string, objects []runtime.Object) error {
	t.events = append(t.events, "apply:"+mc)
	if findObject(objects, kindStatefulSet, "my-cluster-etcd-m2") == nil {
		return nil
	}
	if !t.ready {
		t.diverged = true
	}
	// The replacement's reset container joins the existing cluster.
	t.client.add("my-cluster-etcd-m2", "https://my-cluster-etcd-m2:2380", false)
	return nil
}

func (t *repairRaceTransport) Delete(_ context.Context, _, _, _, _ string) error {
	return nil
}

func (t *repairRaceTransport) List(
	_ context.Context, mc, kind, _, _ string,
) ([]ManifestState, error) {
	if kind != kindStatefulSet {
		return nil, nil
	}
	t.events = append(t.events, "list:"+mc)
	if t.ready {
		return []ManifestState{{Ready: true}}, nil
	}
	t.ready = true
	return []ManifestState{{Ready: false}}, nil
}

func TestRepairMCLossWaitsForRestartingSurvivorsBeforeReplacement(t *testing.T) {
	RegisterTestingT(t)
	client := threeVoters()
	transport := &repairRaceTransport{client: client}
	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})
	pki, err := MintEtcdPKI("my-cluster", membership.Slots, EtcdPKIOptions{})
	Expect(err).ToNot(HaveOccurred())

	_, err = RepairMCLoss(
		context.Background(), transport, client, pki, membership,
		"my-cluster-etcd-m2", []string{"mc-a", "mc-b", "mc-d"}, EtcdOptions{},
	)

	Expect(err).ToNot(HaveOccurred())
	Expect(transport.diverged).To(BeFalse())
	Expect(client.members).To(HaveLen(3))
	Expect(transport.events[len(transport.events)-1]).To(Equal("apply:mc-d"))
	Expect(transport.events).To(ContainElement("list:mc-a"))
}
