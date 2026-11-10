package adapters

import (
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func testMembership(t *testing.T) Membership {
	t.Helper()
	return NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})
}

func testPKI(t *testing.T, membership Membership) *EtcdPKI {
	t.Helper()
	pki, err := MintEtcdPKI("my-cluster", membership.Slots, EtcdPKIOptions{})
	Expect(err).ToNot(HaveOccurred())
	return pki
}

func findObject(objects []runtime.Object, kind, name string) runtime.Object {
	for _, object := range objects {
		switch typed := object.(type) {
		case *corev1.Secret:
			if kind == kindSecret && typed.Name == name {
				return typed
			}
		case *corev1.Service:
			if kind == kindService && typed.Name == name {
				return typed
			}
		case *appsv1.StatefulSet:
			if kind == kindStatefulSet && typed.Name == name {
				return typed
			}
		}
	}
	return nil
}

func TestRenderEtcdManifestsCreatesStableMemberResources(t *testing.T) {
	RegisterTestingT(t)
	membership := testMembership(t)
	pki := testPKI(t, membership)

	objects, err := RenderEtcdManifests(membership, pki, EtcdOptions{})
	Expect(err).ToNot(HaveOccurred())
	Expect(objects).To(HaveLen(9))

	secret, ok := findObject(objects, kindSecret, "my-cluster-etcd-m0-tls").(*corev1.Secret)
	Expect(ok).To(BeTrue())
	Expect(secret.Data).To(HaveLen(7))

	service, ok := findObject(objects, kindService, "my-cluster-etcd-m0").(*corev1.Service)
	Expect(ok).To(BeTrue())
	Expect(service.Spec.Selector).To(HaveKeyWithValue(EtcdMemberLabel, "m0"))
	Expect(service.Spec.Ports).To(HaveLen(2))

	sts, ok := findObject(objects, kindStatefulSet, "my-cluster-etcd-m0").(*appsv1.StatefulSet)
	Expect(ok).To(BeTrue())
	Expect(*sts.Spec.Replicas).To(Equal(int32(1)))
	Expect(sts.Spec.ServiceName).To(Equal("my-cluster-etcd-m0"))
	Expect(sts.Spec.Selector.MatchLabels).To(HaveKeyWithValue(EtcdMemberLabel, "m0"))
	Expect(sts.Spec.Template.Spec.Volumes).To(ContainElement(HaveField("Name", etcdTLSVolume)))

	Expect(sts.Spec.VolumeClaimTemplates).To(HaveLen(1))
	pvc := sts.Spec.VolumeClaimTemplates[0]
	Expect(pvc.Name).To(Equal(etcdDataVolume))
	storage := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	Expect(storage.String()).To(Equal("8Gi"))

	Expect(sts.Spec.Template.Spec.InitContainers).To(HaveLen(1))
	init := sts.Spec.Template.Spec.InitContainers[0]
	Expect(init.Name).To(Equal("reset-member"))
	script := init.Args[0]
	Expect(script).To(ContainSubstring("member remove"))
	Expect(script).To(ContainSubstring("member add"))
	Expect(script).To(ContainSubstring("/var/lib/data/member/snap/db"))
	Expect(script).To(ContainSubstring(membership.Slots[2].ClientURL))

	Expect(sts.Spec.Template.Spec.Containers).To(HaveLen(1))
	etcd := sts.Spec.Template.Spec.Containers[0]
	Expect(etcd.Image).To(Equal("quay.io/openshift/origin-etcd:4.20.0"))
	command := etcd.Args[0]
	Expect(command).To(ContainSubstring("--client-cert-auth"))
	Expect(command).To(ContainSubstring("--peer-client-cert-auth"))
	Expect(command).To(ContainSubstring("--cert-file=/etc/etcd/tls/server.crt"))
	Expect(command).To(ContainSubstring("--key-file=/etc/etcd/tls/server.key"))
	Expect(command).To(ContainSubstring("--peer-cert-file=/etc/etcd/tls/peer.crt"))
	Expect(command).To(ContainSubstring("--initial-cluster=" + initialCluster(membership)))
}

func TestRenderEtcdManifestsUsesConfiguredStorageAndImage(t *testing.T) {
	RegisterTestingT(t)
	membership := testMembership(t)
	pki := testPKI(t, membership)

	objects, err := RenderEtcdManifests(membership, pki, EtcdOptions{
		Image:        "example/etcd:test",
		StorageClass: "fast",
		StorageSize:  "2Gi",
	})
	Expect(err).ToNot(HaveOccurred())

	sts, ok := findObject(objects, kindStatefulSet, "my-cluster-etcd-m0").(*appsv1.StatefulSet)
	Expect(ok).To(BeTrue())
	Expect(sts.Spec.Template.Spec.Containers[0].Image).To(Equal("example/etcd:test"))
	pvc := sts.Spec.VolumeClaimTemplates[0]
	Expect(*pvc.Spec.StorageClassName).To(Equal("fast"))
	storage := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	Expect(storage.String()).To(Equal("2Gi"))
}

func TestRenderEtcdManifestsRejectsMissingCertMaterial(t *testing.T) {
	RegisterTestingT(t)
	membership := testMembership(t)

	_, err := RenderEtcdManifests(membership, &EtcdPKI{}, EtcdOptions{})

	Expect(err).To(MatchError(ContainSubstring("missing cert material")))
}

func TestRenderEtcdManifestsUsesNodePortServicesOnTheFabric(t *testing.T) {
	RegisterTestingT(t)
	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{
		NodeIPs: map[string]string{"mc-a": "10.89.0.10", "mc-b": "10.89.0.11", "mc-c": "10.89.0.12"},
	})
	pki := testPKI(t, membership)

	objects, err := RenderEtcdManifests(membership, pki, EtcdOptions{})
	Expect(err).ToNot(HaveOccurred())

	service, ok := findObject(objects, kindService, "my-cluster-etcd-m0").(*corev1.Service)
	Expect(ok).To(BeTrue())
	Expect(service.Spec.Type).To(Equal(corev1.ServiceTypeNodePort))
	Expect(service.Spec.Ports[0].NodePort).To(Equal(int32(30379)))
	Expect(service.Spec.Ports[1].NodePort).To(Equal(int32(30380)))

	sts, ok := findObject(objects, kindStatefulSet, "my-cluster-etcd-m0").(*appsv1.StatefulSet)
	Expect(ok).To(BeTrue())
	Expect(sts.Spec.Template.Spec.Containers[0].Args[0]).
		To(ContainSubstring("--advertise-client-urls=https://10.89.0.10:30379"))
	Expect(sts.Spec.Template.Spec.Containers[0].Args[0]).
		To(ContainSubstring("--initial-cluster=my-cluster-etcd-m0=https://10.89.0.10:30380"))
}
