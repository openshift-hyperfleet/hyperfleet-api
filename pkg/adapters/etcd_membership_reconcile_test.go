package adapters

import (
	"context"
	"strconv"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/fleet"
)

func memberStates(healthy map[string]bool) map[string][]ManifestState {
	states := make(map[string][]ManifestState, len(healthy)*2)
	for index, name := range []string{
		"my-cluster-etcd-m0", "my-cluster-etcd-m1", "my-cluster-etcd-m2",
	} {
		selector := EtcdMemberLabel + "=m" + strconv.Itoa(index)
		for _, mc := range []string{"mc-a", "mc-b", "mc-c"} {
			states[mc+"/"+kindPod+"/"+selector] = []ManifestState{{
				Kind: kindPod, Name: name, Ready: healthy[name],
			}}
			states[mc+"/"+kindStatefulSet+"/"+selector] = []ManifestState{{
				Kind: kindStatefulSet, Name: name, Ready: true,
			}}
		}
	}
	return states
}

func membershipController(
	t *testing.T, client MemberClient, states map[string][]ManifestState,
) (*EtcdController, *recordingTransport) {
	t.Helper()
	transport := &recordingTransport{
		applied: map[string][]runtime.Object{},
		listed:  states,
	}
	controller := NewEtcdController(
		fakeFleet{mcs: []fleet.MC{{Name: "mc-a"}, {Name: "mc-b"}, {Name: "mc-c"}}}.list,
		transport, EtcdOptions{},
	).WithMembership(client)
	return controller, transport
}

func TestEtcdControllerReportsRefusalInsteadOfActing(t *testing.T) {
	RegisterTestingT(t)
	client := threeVoters()
	controller, transport := membershipController(t, client, memberStates(map[string]bool{
		"my-cluster-etcd-m0": true,
	}))

	report, err := controller.Reconcile(context.Background(), ClusterView{
		Name: "my-cluster", Generation: 3, Spec: managedEtcdSpec(),
	})

	Expect(err).ToNot(HaveOccurred())
	Expect(transport.applied).To(BeEmpty())
	Expect(transport.deleted).To(BeEmpty())
	Expect(client.calls).To(BeEmpty())
	Expect(conditionStatus(report, api.AdapterConditionTypeAvailable)).
		To(Equal(api.AdapterConditionFalse))
	Expect(*report.Conditions[0].Reason).To(Equal("NoQuorum"))
}

func TestEtcdControllerReplacesTheSingleFailingMember(t *testing.T) {
	RegisterTestingT(t)
	client := threeVoters()
	controller, transport := membershipController(t, client, memberStates(map[string]bool{
		"my-cluster-etcd-m0": true,
		"my-cluster-etcd-m2": true,
	}))

	report, err := controller.Reconcile(context.Background(), ClusterView{
		Name: "my-cluster", Generation: 3, Spec: managedEtcdSpec(),
	})

	Expect(err).ToNot(HaveOccurred())
	Expect(transport.applied).To(BeEmpty())
	Expect(transport.deleted).To(HaveLen(2))
	Expect(client.calls).To(Equal([]string{
		"remove:id-2",
		"learner:my-cluster-etcd-m1:https://my-cluster-etcd-m1.etcd-my-cluster.svc:2380",
	}))
	Expect(*report.Conditions[0].Reason).To(Equal("ReplaceMember"))
}
