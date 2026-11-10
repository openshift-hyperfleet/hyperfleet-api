package adapters

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/fleet"
)

//nolint:govet // fieldalignment: test fake reads naturally in this order
type teardownTransport struct {
	objects     map[string]bool
	deleted     []string
	deleteErr   error
	deferDelete bool
}

func newTeardownTransport() *teardownTransport {
	return &teardownTransport{objects: map[string]bool{}}
}

func (t *teardownTransport) Apply(context.Context, string, []runtime.Object) error {
	return nil
}

func (t *teardownTransport) Delete(
	_ context.Context, mc, kind, namespace, name string,
) error {
	if t.deleteErr != nil {
		return t.deleteErr
	}
	if !t.deferDelete {
		delete(t.objects, kind+"/"+namespace+"/"+name)
	}
	t.deleted = append(t.deleted, mc+"/"+kind+"/"+namespace+"/"+name)
	return nil
}

func (t *teardownTransport) List(
	_ context.Context, _, kind, namespace, _ string,
) ([]ManifestState, error) {
	var states []ManifestState
	for key := range t.objects {
		parts := strings.SplitN(key, "/", 3)
		if len(parts) != 3 || parts[0] != kind || parts[1] != namespace {
			continue
		}
		states = append(states, ManifestState{Kind: kind, Name: parts[2], Namespace: namespace})
	}
	return states, nil
}

func teardownMembership() Membership {
	return NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})
}

func teardownState(t *testing.T, membership Membership) (string, *EtcdPKI) {
	t.Helper()
	dir := t.TempDir()
	pki, err := MintEtcdPKI("my-cluster", membership.Slots, EtcdPKIOptions{})
	Expect(err).ToNot(HaveOccurred())
	Expect(saveEtcdState(dir, stateFrom(membership, pki))).To(Succeed())
	return dir, pki
}

func seedTeardownObjects(transport *teardownTransport, membership Membership, pki *EtcdPKI) {
	for _, slot := range membership.Slots {
		for _, object := range memberTeardownObjects(slot, pki) {
			transport.objects[object.kind+"/"+slot.Namespace+"/"+object.name] = true
		}
	}
}

func teardownClient() *fakeMemberClient {
	return &fakeMemberClient{members: map[string]EtcdMember{
		"id-1": learner("id-1", "my-cluster-etcd-m0"),
		"id-2": learner("id-2", "my-cluster-etcd-m1"),
		"id-3": learner("id-3", "my-cluster-etcd-m2"),
	}}
}

func teardownController(
	transport Transport, client MemberClient, dir string,
) *EtcdController {
	return NewEtcdController(
		fakeFleet{mcs: []fleet.MC{{Name: "mc-a"}, {Name: "mc-b"}, {Name: "mc-c"}}}.list,
		transport, EtcdOptions{StateDir: dir},
	).WithMembership(client)
}

func deletingCluster() ClusterView {
	return ClusterView{
		Name: "my-cluster", Generation: 2, Deleting: true, Spec: managedEtcdSpec(),
	}
}

func stateFileExists(dir, cluster string) bool {
	_, err := os.Stat(filepath.Join(dir, cluster+".json"))
	return err == nil
}

func TestEtcdControllerFinalizesWhenNothingWasMinted(t *testing.T) {
	RegisterTestingT(t)
	transport := newTeardownTransport()
	controller := NewEtcdController(fakeFleet{}.list, transport, EtcdOptions{StateDir: t.TempDir()})

	report, err := controller.Reconcile(context.Background(), deletingCluster())

	Expect(err).ToNot(HaveOccurred())
	Expect(conditionStatus(report, api.AdapterConditionTypeFinalized)).
		To(Equal(api.AdapterConditionTrue))
	Expect(transport.deleted).To(BeEmpty())
}

func TestEtcdControllerFinalizesNotManagedTeardown(t *testing.T) {
	RegisterTestingT(t)
	transport := newTeardownTransport()
	controller := NewEtcdController(fakeFleet{}.list, transport, EtcdOptions{StateDir: t.TempDir()})
	cluster := deletingCluster()
	cluster.Spec = map[string]interface{}{}

	report, err := controller.Reconcile(context.Background(), cluster)

	Expect(err).ToNot(HaveOccurred())
	Expect(conditionStatus(report, api.AdapterConditionTypeFinalized)).
		To(Equal(api.AdapterConditionTrue))
	Expect(transport.deleted).To(BeEmpty())
}

func TestEtcdControllerTearsDownMembersThenFinalizes(t *testing.T) {
	RegisterTestingT(t)
	membership := teardownMembership()
	dir, pki := teardownState(t, membership)
	transport := newTeardownTransport()
	seedTeardownObjects(transport, membership, pki)
	client := teardownClient()
	controller := teardownController(transport, client, dir)

	report, err := controller.Reconcile(context.Background(), deletingCluster())

	Expect(err).ToNot(HaveOccurred())
	Expect(conditionStatus(report, api.AdapterConditionTypeFinalized)).
		To(Equal(api.AdapterConditionTrue))
	Expect(client.calls).To(Equal([]string{"remove:id-1", "remove:id-2", "remove:id-3"}))
	Expect(transport.deleted).To(HaveLen(3 * 4))
	Expect(transport.objects).To(BeEmpty())
	Expect(stateFileExists(dir, "my-cluster")).To(BeFalse())
}

func TestEtcdControllerWaitsForTeardownToFinish(t *testing.T) {
	RegisterTestingT(t)
	membership := teardownMembership()
	dir, pki := teardownState(t, membership)
	transport := newTeardownTransport()
	seedTeardownObjects(transport, membership, pki)
	transport.deferDelete = true
	controller := teardownController(transport, teardownClient(), dir)

	report, err := controller.Reconcile(context.Background(), deletingCluster())

	Expect(err).ToNot(HaveOccurred())
	Expect(conditionByType(report, api.AdapterConditionTypeFinalized)).To(BeNil())
	Expect(*report.Conditions[0].Reason).To(Equal("Deleting"))
	Expect(stateFileExists(dir, "my-cluster")).To(BeTrue())

	transport.deferDelete = false
	report, err = controller.Reconcile(context.Background(), deletingCluster())

	Expect(err).ToNot(HaveOccurred())
	Expect(conditionStatus(report, api.AdapterConditionTypeFinalized)).
		To(Equal(api.AdapterConditionTrue))
	Expect(stateFileExists(dir, "my-cluster")).To(BeFalse())
}

func TestEtcdControllerSurfacesTeardownErrorsAndRetries(t *testing.T) {
	RegisterTestingT(t)
	membership := teardownMembership()
	dir, pki := teardownState(t, membership)
	transport := newTeardownTransport()
	seedTeardownObjects(transport, membership, pki)
	transport.deleteErr = errors.New("transport down")
	controller := teardownController(transport, teardownClient(), dir)

	report, err := controller.Reconcile(context.Background(), deletingCluster())

	Expect(err).To(MatchError(ContainSubstring("transport down")))
	Expect(conditionByType(report, api.AdapterConditionTypeFinalized)).To(BeNil())
	Expect(*report.Conditions[1].Reason).To(Equal("TeardownFailed"))
	Expect(stateFileExists(dir, "my-cluster")).To(BeTrue())

	transport.deleteErr = nil
	report, err = controller.Reconcile(context.Background(), deletingCluster())

	Expect(err).ToNot(HaveOccurred())
	Expect(conditionStatus(report, api.AdapterConditionTypeFinalized)).
		To(Equal(api.AdapterConditionTrue))
	Expect(stateFileExists(dir, "my-cluster")).To(BeFalse())
}

func TestEtcdControllerFinalizesAfterAFailedMint(t *testing.T) {
	RegisterTestingT(t)
	membership := teardownMembership()
	dir, _ := teardownState(t, membership)
	transport := newTeardownTransport()
	client := &fakeMemberClient{listErr: errors.New("connection refused")}
	controller := teardownController(transport, client, dir)

	report, err := controller.Reconcile(context.Background(), deletingCluster())

	Expect(err).ToNot(HaveOccurred())
	Expect(conditionStatus(report, api.AdapterConditionTypeFinalized)).
		To(Equal(api.AdapterConditionTrue))
	Expect(stateFileExists(dir, "my-cluster")).To(BeFalse())
}

func TestEtcdControllerFinalizesRepeatedlyAfterTeardown(t *testing.T) {
	RegisterTestingT(t)
	membership := teardownMembership()
	dir, pki := teardownState(t, membership)
	transport := newTeardownTransport()
	seedTeardownObjects(transport, membership, pki)
	client := teardownClient()
	controller := teardownController(transport, client, dir)
	first, err := controller.Reconcile(context.Background(), deletingCluster())
	Expect(err).ToNot(HaveOccurred())
	Expect(conditionStatus(first, api.AdapterConditionTypeFinalized)).
		To(Equal(api.AdapterConditionTrue))
	deletes, removes := len(transport.deleted), len(client.calls)

	report, err := controller.Reconcile(context.Background(), deletingCluster())

	Expect(err).ToNot(HaveOccurred())
	Expect(conditionStatus(report, api.AdapterConditionTypeFinalized)).
		To(Equal(api.AdapterConditionTrue))
	Expect(transport.deleted).To(HaveLen(deletes))
	Expect(client.calls).To(HaveLen(removes))
}
