package adapters

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/fleet"
)

func TestPlaceSpreadsOneMemberPerMC(t *testing.T) {
	RegisterTestingT(t)

	placed, err := Place([]string{"mc-c", "mc-a", "mc-b"}, 3, "mc-b")

	Expect(err).ToNot(HaveOccurred())
	Expect(placed).To(Equal([]string{"mc-b", "mc-a", "mc-c"}))
}

func TestPlaceRejectsInsufficientMCs(t *testing.T) {
	RegisterTestingT(t)

	_, err := Place([]string{"mc-a", "mc-b"}, 3, "mc-a")

	Expect(err).To(MatchError(ContainSubstring("need 3, found 2")))
}

func TestPlaceRejectsMissingPreferredMC(t *testing.T) {
	RegisterTestingT(t)

	_, err := Place([]string{"mc-a", "mc-b", "mc-c"}, 3, "mc-z")

	Expect(err).To(MatchError(ContainSubstring("preferred management cluster")))
}

//nolint:govet // fieldalignment: test fake reads naturally in this order
type fakeFleet struct {
	mcs []fleet.MC
	err error
}

func (f fakeFleet) list() ([]fleet.MC, error) {
	return f.mcs, f.err
}

//nolint:govet // fieldalignment: test fake reads naturally in this order
type recordingTransport struct {
	applied map[string][]runtime.Object
	listed  map[string][]ManifestState
	deleted []string
	err     error
}

func (t *recordingTransport) Apply(_ context.Context, mc string, objects []runtime.Object) error {
	if t.err != nil {
		return t.err
	}
	t.applied[mc] = append(t.applied[mc], objects...)
	return nil
}

func (t *recordingTransport) Delete(
	_ context.Context, mc, kind, namespace, name string,
) error {
	if t.err != nil {
		return t.err
	}
	t.deleted = append(t.deleted, mc+"/"+kind+"/"+namespace+"/"+name)
	return nil
}

func (t *recordingTransport) List(
	_ context.Context, mc, kind, _, selector string,
) ([]ManifestState, error) {
	if t.err != nil {
		return nil, t.err
	}
	return t.listed[mc+"/"+kind+"/"+selector], nil
}

func managedEtcdSpec() map[string]interface{} {
	return map[string]interface{}{"etcd": map[string]interface{}{"managed": "clm"}}
}

func TestEtcdControllerPlacesMemberZeroOnPreferredMC(t *testing.T) {
	RegisterTestingT(t)
	mcs := []fleet.MC{{Name: "mc-b"}, {Name: "mc-a"}, {Name: "mc-c"}}
	transport := &recordingTransport{applied: map[string][]runtime.Object{}}
	controller := NewEtcdController(
		fakeFleet{mcs: mcs}.list, transport, EtcdOptions{PreferredMC: "mc-b"},
	)
	cluster := ClusterView{Name: "my-cluster", Generation: 3, Spec: managedEtcdSpec()}

	report, err := controller.Reconcile(context.Background(), cluster)

	Expect(err).ToNot(HaveOccurred())
	Expect(transport.applied).To(HaveLen(3))
	Expect(findObject(transport.applied["mc-b"], kindStatefulSet, "my-cluster-etcd-m0")).NotTo(BeNil())
	Expect(findObject(transport.applied["mc-a"], kindStatefulSet, "my-cluster-etcd-m1")).NotTo(BeNil())
	Expect(findObject(transport.applied["mc-c"], kindStatefulSet, "my-cluster-etcd-m2")).NotTo(BeNil())

	Expect(report.Data["quorumState"]).To(Equal("Pending"))
	Expect(report.Data["endpoints"]).To(HaveLen(3))
	Expect(report.Data["members"]).To(HaveLen(3))
}

func TestEtcdControllerMintsNothingWithTooFewMCs(t *testing.T) {
	RegisterTestingT(t)
	mcs := []fleet.MC{{Name: "mc-a"}, {Name: "mc-b"}}
	transport := &recordingTransport{applied: map[string][]runtime.Object{}}
	controller := NewEtcdController(
		fakeFleet{mcs: mcs}.list, transport, EtcdOptions{PreferredMC: "mc-a"},
	)
	cluster := ClusterView{Name: "my-cluster", Generation: 1, Spec: managedEtcdSpec()}

	report, err := controller.Reconcile(context.Background(), cluster)

	Expect(err).To(MatchError(ContainSubstring("need 3, found 2")))
	Expect(transport.applied).To(BeEmpty())
	Expect(report.Conditions).To(HaveLen(3))
	Expect(*report.Conditions[0].Message).To(ContainSubstring("need 3, found 2"))
}

func TestEtcdControllerDoesNothingWithoutManagedKnob(t *testing.T) {
	RegisterTestingT(t)
	mcs := []fleet.MC{{Name: "mc-a"}, {Name: "mc-b"}, {Name: "mc-c"}}
	transport := &recordingTransport{applied: map[string][]runtime.Object{}}
	controller := NewEtcdController(fakeFleet{mcs: mcs}.list, transport, EtcdOptions{})
	cluster := ClusterView{Name: "my-cluster", Generation: 1, Spec: map[string]interface{}{}}

	report, err := controller.Reconcile(context.Background(), cluster)

	Expect(err).ToNot(HaveOccurred())
	Expect(transport.applied).To(BeEmpty())
	Expect(report.Data).To(BeEmpty())
}
