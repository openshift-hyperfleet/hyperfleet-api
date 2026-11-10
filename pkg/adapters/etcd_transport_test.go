package adapters

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newFakeDynamicClient(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	listKinds := map[schema.GroupVersionResource]string{
		etcdKindResources[kindPod]:                      "PodList",
		etcdKindResources[kindService]:                  "ServiceList",
		etcdKindResources[kindSecret]:                   "SecretList",
		etcdKindResources[kindStatefulSet]:              "StatefulSetList",
		corev1.SchemeGroupVersion.WithResource("nodes"): "NodeList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(), listKinds, objects...,
	)
}

func testTransport(t *testing.T, client *dynamicfake.FakeDynamicClient) *KubeconfigTransport {
	t.Helper()
	transport := NewKubeconfigTransport(map[string]string{"mc-a": "/outside/repo/mc-a.kubeconfig"})
	transport.newClient = func(path string) (dynamic.Interface, error) {
		Expect(path).To(Equal("/outside/repo/mc-a.kubeconfig"))
		return client, nil
	}
	return transport
}

func testService() *corev1.Service {
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: kindService},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-cluster-etcd-m0",
			Namespace: "etcd-my-cluster",
			Labels:    map[string]string{"app": "etcd"},
		},
		Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Port: etcdClientPort}}},
	}
}

func TestKubeconfigTransportAppliesAndUpdates(t *testing.T) {
	RegisterTestingT(t)
	client := newFakeDynamicClient()
	transport := testTransport(t, client)
	ctx := context.Background()

	Expect(transport.Apply(ctx, "mc-a", []runtime.Object{testService()})).To(Succeed())

	service := testService()
	service.Labels = map[string]string{"app": "etcd", "updated": "true"}
	Expect(transport.Apply(ctx, "mc-a", []runtime.Object{service})).To(Succeed())

	states, err := transport.List(ctx, "mc-a", kindService, "etcd-my-cluster", "app=etcd")
	Expect(err).ToNot(HaveOccurred())
	Expect(states).To(HaveLen(1))
	Expect(states[0].Labels).To(HaveKeyWithValue("updated", "true"))

	verbs := make([]string, 0, len(client.Actions()))
	for _, action := range client.Actions() {
		verbs = append(verbs, action.GetVerb())
	}
	Expect(verbs).To(Equal([]string{"get", "create", "get", "update", "list"}))
}

func TestKubeconfigTransportLeavesExistingPVCsAlone(t *testing.T) {
	RegisterTestingT(t)
	client := newFakeDynamicClient()
	transport := testTransport(t, client)
	claim := &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: kindPersistentVolumeClaim},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hcp-state",
			Namespace: "hcp-my-cluster",
		},
	}
	ctx := context.Background()

	Expect(transport.Apply(ctx, "mc-a", []runtime.Object{claim})).To(Succeed())
	storageClass := "standard"
	claim.Spec.StorageClassName = &storageClass
	Expect(transport.Apply(ctx, "mc-a", []runtime.Object{claim})).To(Succeed())

	verbs := make([]string, 0, len(client.Actions()))
	for _, action := range client.Actions() {
		verbs = append(verbs, action.GetVerb())
	}
	Expect(verbs).To(Equal([]string{"get", "create", "get"}))
}

func TestKubeconfigTransportListsPodHealth(t *testing.T) {
	RegisterTestingT(t)
	pod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       kindPod,
		"metadata": map[string]interface{}{
			"name":      "my-cluster-etcd-m0",
			"namespace": "etcd-my-cluster",
			"labels": map[string]interface{}{
				"app":           "etcd",
				EtcdMemberLabel: "m0",
			},
		},
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Ready", "status": "True"},
			},
		},
	}}
	notReady := pod.DeepCopy()
	notReady.SetName("my-cluster-etcd-m1")
	notReady.SetLabels(map[string]string{"app": "etcd", EtcdMemberLabel: "m1"})
	notReady.Object["status"] = map[string]interface{}{
		"conditions": []interface{}{
			map[string]interface{}{"type": "Ready", "status": "False"},
		},
	}
	client := newFakeDynamicClient(pod, notReady)
	transport := testTransport(t, client)

	states, err := transport.List(
		context.Background(), "mc-a", kindPod, "etcd-my-cluster", "app=etcd",
	)
	Expect(err).ToNot(HaveOccurred())
	Expect(states).To(HaveLen(2))
	Expect(states[0].Name).To(Equal("my-cluster-etcd-m0"))
	Expect(states[0].Ready).To(BeTrue())
	Expect(states[1].Ready).To(BeFalse())

	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})
	healthy := membership.ObservedHealth(states)
	Expect(healthy).To(HaveKeyWithValue("my-cluster-etcd-m0", true))
	Expect(healthy).ToNot(HaveKey("my-cluster-etcd-m1"))
}

func TestKubeconfigTransportDeletesIdempotently(t *testing.T) {
	RegisterTestingT(t)
	transport := testTransport(t, newFakeDynamicClient())
	ctx := context.Background()
	Expect(transport.Apply(ctx, "mc-a", []runtime.Object{testService()})).To(Succeed())

	Expect(transport.Delete(ctx, "mc-a", kindService, "etcd-my-cluster", "my-cluster-etcd-m0")).
		To(Succeed())
	Expect(transport.Delete(ctx, "mc-a", kindService, "etcd-my-cluster", "my-cluster-etcd-m0")).
		To(Succeed())

	states, err := transport.List(ctx, "mc-a", kindService, "etcd-my-cluster", "app=etcd")
	Expect(err).ToNot(HaveOccurred())
	Expect(states).To(BeEmpty())
}

func TestKubeconfigTransportRejectsUnknownMC(t *testing.T) {
	RegisterTestingT(t)
	transport := NewKubeconfigTransport(nil)

	err := transport.Apply(context.Background(), "missing", nil)

	Expect(err).To(MatchError(ContainSubstring("no kubeconfig configured")))
}

func TestKubeconfigTransportRejectsUnknownKind(t *testing.T) {
	RegisterTestingT(t)
	transport := testTransport(t, newFakeDynamicClient())

	_, err := transport.List(context.Background(), "mc-a", "Unknown", "", "")

	Expect(err).To(MatchError(ContainSubstring("unsupported manifest kind")))
}

func TestKubeconfigTransportResolvesNodeInternalIP(t *testing.T) {
	RegisterTestingT(t)
	node := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata":   map[string]interface{}{"name": "control-plane"},
		"status": map[string]interface{}{
			"addresses": []interface{}{
				map[string]interface{}{"type": "Hostname", "address": "control-plane"},
				map[string]interface{}{"type": "InternalIP", "address": "10.89.0.42"},
			},
		},
	}}
	transport := testTransport(t, newFakeDynamicClient(node))

	address, err := transport.NodeAddress(context.Background(), "mc-a")

	Expect(err).ToNot(HaveOccurred())
	Expect(address).To(Equal("10.89.0.42"))
}

func TestKubeconfigTransportReportsStatefulSetRolloutReadiness(t *testing.T) {
	RegisterTestingT(t)
	statefulSet := func(name string, generation int64, status map[string]interface{}) *unstructured.Unstructured {
		return &unstructured.Unstructured{Object: map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       kindStatefulSet,
			"metadata": map[string]interface{}{
				"name":       name,
				"namespace":  "etcd-my-cluster",
				"generation": generation,
				"labels":     map[string]interface{}{"app": "etcd"},
			},
			"spec":   map[string]interface{}{"replicas": int64(1)},
			"status": status,
		}}
	}
	ready := statefulSet("my-cluster-etcd-m0", 2, map[string]interface{}{
		"observedGeneration": int64(2),
		"readyReplicas":      int64(1),
		"updatedReplicas":    int64(1),
	})
	restarting := statefulSet("my-cluster-etcd-m1", 3, map[string]interface{}{
		"observedGeneration": int64(2),
		"readyReplicas":      int64(1),
		"updatedReplicas":    int64(0),
	})
	transport := testTransport(t, newFakeDynamicClient(ready, restarting))

	states, err := transport.List(
		context.Background(), "mc-a", kindStatefulSet, "etcd-my-cluster", "app=etcd",
	)
	Expect(err).ToNot(HaveOccurred())
	Expect(states).To(HaveLen(2))
	Expect(states[0].Ready).To(BeTrue())
	Expect(states[1].Ready).To(BeFalse())
}
