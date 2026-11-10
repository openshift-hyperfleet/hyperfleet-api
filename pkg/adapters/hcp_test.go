package adapters

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/fleet"
)

//nolint:govet // fieldalignment: test fake reads naturally in this order
type hcpFakeTransport struct {
	applied     map[string][]runtime.Object
	deleted     []string
	pods        map[string][]ManifestState
	namespaces  map[string]map[string]bool
	secrets     map[string]map[string][]byte
	kubeconfig  []byte
	execErr     error
	deferDelete bool
}

func newHCPFakeTransport() *hcpFakeTransport {
	return &hcpFakeTransport{
		applied:    map[string][]runtime.Object{},
		pods:       map[string][]ManifestState{},
		namespaces: map[string]map[string]bool{},
		secrets:    map[string]map[string][]byte{},
	}
}

func (t *hcpFakeTransport) Apply(
	_ context.Context, mc string, objects []runtime.Object,
) error {
	t.applied[mc] = append(t.applied[mc], objects...)
	for _, object := range objects {
		if namespace, ok := object.(*corev1.Namespace); ok {
			if t.namespaces[mc] == nil {
				t.namespaces[mc] = map[string]bool{}
			}
			t.namespaces[mc][namespace.Name] = true
		}
	}
	return nil
}

func (t *hcpFakeTransport) Delete(
	_ context.Context, mc, kind, namespace, name string,
) error {
	t.deleted = append(t.deleted, mc+"/"+kind+"/"+namespace+"/"+name)
	if kind == kindNamespace && !t.deferDelete {
		delete(t.namespaces[mc], name)
	}
	return nil
}

func (t *hcpFakeTransport) List(
	_ context.Context, mc, kind, namespace, _ string,
) ([]ManifestState, error) {
	switch kind {
	case kindPod:
		return t.pods[mc], nil
	case kindNamespace:
		states := make([]ManifestState, 0, len(t.namespaces[mc]))
		for name := range t.namespaces[mc] {
			if namespace != "" && namespace != name {
				continue
			}
			states = append(states, ManifestState{Kind: kind, Name: name})
		}
		sort.Slice(states, func(i, j int) bool { return states[i].Name < states[j].Name })
		return states, nil
	}
	return nil, nil
}

func (t *hcpFakeTransport) ReadSecret(
	_ context.Context, mc, namespace, name string,
) (map[string][]byte, error) {
	data, ok := t.secrets[mc+"/"+namespace+"/"+name]
	if !ok {
		return nil, fmt.Errorf("secret %q not found in %q", name, mc)
	}
	return data, nil
}

func (t *hcpFakeTransport) Exec(
	_ context.Context, _, _, _ string, _ []string,
) ([]byte, error) {
	return t.kubeconfig, t.execErr
}

type hcpFakeStatus map[string]map[string]interface{}

func (s hcpFakeStatus) read(
	_ context.Context, _ ClusterView, adapter string,
) (map[string]interface{}, error) {
	return s[adapter], nil
}

func hcpStatusData(members ...EtcdMemberStatus) map[string]interface{} {
	data := map[string]interface{}{
		"endpoints": []string{"https://10.0.0.1:30379"},
	}
	if len(members) > 0 {
		data["members"] = members
	}
	return data
}

func hcpSecretData(marker string) map[string][]byte {
	return map[string][]byte{
		EtcdCACertKey:     []byte("ca-" + marker),
		EtcdClientCertKey: []byte("cert-" + marker),
		EtcdClientKeyKey:  []byte("key-" + marker),
	}
}

func hcpTestController(
	t *testing.T, transport *hcpFakeTransport, statuses hcpFakeStatus,
	preferred string, mcs ...string,
) *HCPController {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pull-secret")
	Expect(os.WriteFile(path, []byte("PULL-SECRET-MARKER"), 0o600)).To(Succeed())
	return NewHCPController(
		func() ([]fleet.MC, error) {
			found := make([]fleet.MC, len(mcs))
			for index, name := range mcs {
				found[index] = fleet.MC{Name: name}
			}
			return found, nil
		},
		transport,
		statuses.read,
		HCPOptions{
			Image:          "localhost/coreshift:test",
			PullSecretFile: path,
			PreferredMC:    preferred,
		},
	)
}

func findHCPObject(objects []runtime.Object, kind, name string) runtime.Object {
	for _, object := range objects {
		switch typed := object.(type) {
		case *corev1.Namespace:
			if kind == kindNamespace && typed.Name == name {
				return typed
			}
		case *corev1.Secret:
			if kind == kindSecret && typed.Name == name {
				return typed
			}
		case *corev1.PersistentVolumeClaim:
			if kind == kindPersistentVolumeClaim && typed.Name == name {
				return typed
			}
		case *appsv1.Deployment:
			if kind == "Deployment" && typed.Name == name {
				return typed
			}
		}
	}
	return nil
}

func hcpTestCluster() ClusterView {
	return ClusterView{
		ID:         "cluster-id",
		Name:       "my-cluster",
		Generation: 2,
		Spec: map[string]interface{}{
			"topology": "hosted",
			"platform": "hcp",
		},
	}
}

func TestRenderHCPManifestsMatchesNestedRecipe(t *testing.T) {
	RegisterTestingT(t)
	objects, err := RenderHCPManifests(HCPManifestInput{
		Cluster:    "my-cluster",
		Namespace:  "hcp-my-cluster",
		Image:      "localhost/coreshift:tag",
		PullSecret: []byte("pull-secret-content"),
		EtcdSecret: EtcdSecret{Data: hcpSecretData("m0")},
		Endpoints:  []string{"https://10.0.0.1:30379", "https://10.0.0.2:30379"},
	})
	Expect(err).ToNot(HaveOccurred())

	namespace := findHCPObject(objects, kindNamespace, "hcp-my-cluster")
	Expect(namespace).ToNot(BeNil())
	pullSecret := findHCPObject(objects, kindSecret, hcpPullSecretName).(*corev1.Secret)
	Expect(pullSecret.Data[hcpPullSecretKey]).To(Equal([]byte("pull-secret-content")))
	clientSecret := findHCPObject(objects, kindSecret, EtcdClientTLSName).(*corev1.Secret)
	Expect(clientSecret.Data).To(HaveKeyWithValue(EtcdCACertKey, []byte("ca-m0")))
	claim := findHCPObject(objects, kindPersistentVolumeClaim, hcpStateClaimName).(*corev1.PersistentVolumeClaim)
	Expect(claim.Spec.Resources.Requests.Storage().String()).To(Equal(hcpStorageSize))

	deployment := findHCPObject(objects, "Deployment", "hcp").(*appsv1.Deployment)
	Expect(*deployment.Spec.Replicas).To(Equal(int32(1)))
	Expect(deployment.Spec.Strategy.Type).To(Equal(appsv1.RecreateDeploymentStrategyType))
	Expect(*deployment.Spec.Template.Spec.AutomountServiceAccountToken).To(BeFalse())

	container := deployment.Spec.Template.Spec.Containers[0]
	Expect(container.Image).To(Equal("localhost/coreshift:tag"))
	Expect(container.ImagePullPolicy).To(Equal(corev1.PullIfNotPresent))
	args := strings.Join(container.Args, " ")
	Expect(args).To(ContainSubstring("--pull-secret /secret/pull-secret"))
	Expect(args).To(ContainSubstring("--external-etcd-endpoints=https://10.0.0.1:30379,https://10.0.0.2:30379"))
	Expect(args).To(ContainSubstring("--external-etcd-ca=/etc/etcd-client/ca.crt"))
	Expect(args).To(ContainSubstring("--external-etcd-cert=/etc/etcd-client/etcd-client.crt"))
	Expect(args).To(ContainSubstring("--external-etcd-key=/etc/etcd-client/etcd-client.key"))
	Expect(container.Env).To(ContainElement(corev1.EnvVar{Name: "XDG_STATE_HOME", Value: hcpStateDir}))
	Expect(container.Env).To(ContainElement(corev1.EnvVar{Name: "XDG_CACHE_HOME", Value: hcpCacheDir}))
	Expect(container.Resources.Requests.Cpu().String()).To(Equal("500m"))
	Expect(container.Resources.Requests.Memory().String()).To(Equal("2Gi"))
	Expect(container.Resources.Limits.Memory().String()).To(Equal("4Gi"))
}

func TestHCPControllerPlacesOnPreferredMCNextToMemberZero(t *testing.T) {
	RegisterTestingT(t)
	transport := newHCPFakeTransport()
	transport.secrets["mc-b/etcd-my-cluster/my-cluster-etcd-m0-tls"] = hcpSecretData("mc-b")
	statuses := hcpFakeStatus{
		EtcdAdapterName: hcpStatusData(
			EtcdMemberStatus{Name: "my-cluster-etcd-m0", MC: "mc-b"},
			EtcdMemberStatus{Name: "my-cluster-etcd-m1", MC: "mc-a"},
		),
	}
	controller := hcpTestController(t, transport, statuses, "mc-b", "mc-a", "mc-b", "mc-c")

	report, err := controller.Reconcile(context.Background(), hcpTestCluster())

	Expect(err).ToNot(HaveOccurred())
	Expect(transport.applied).To(HaveLen(1))
	Expect(transport.applied).To(HaveKey("mc-b"))
	Expect(findHCPObject(transport.applied["mc-b"], "Deployment", "hcp")).ToNot(BeNil())
	Expect(report.Data["mc"]).To(Equal("mc-b"))
	Expect(report.Data["pod"]).To(Equal(""))
	Expect(report.Data["ready"]).To(Equal(false))
	Expect(report.Data["imageRef"]).To(Equal("localhost/coreshift:test"))
}

func TestHCPControllerWaitsForEtcdEndpoints(t *testing.T) {
	RegisterTestingT(t)
	transport := newHCPFakeTransport()
	controller := hcpTestController(t, transport, hcpFakeStatus{}, "mc-a", "mc-a")

	report, err := controller.Reconcile(context.Background(), hcpTestCluster())

	Expect(err).ToNot(HaveOccurred())
	Expect(transport.applied).To(BeEmpty())
	Expect(*report.Conditions[0].Reason).To(Equal("WaitingForEtcd"))
	Expect(conditionStatus(report, api.AdapterConditionTypeApplied)).To(Equal(api.AdapterConditionFalse))
}

func TestHCPControllerReHomesToASurvivingEtcdMember(t *testing.T) {
	RegisterTestingT(t)
	transport := newHCPFakeTransport()
	transport.secrets["mc-c/etcd-my-cluster/my-cluster-etcd-m1-tls"] = hcpSecretData("mc-c")
	statuses := hcpFakeStatus{
		HCPAdapterName: {"mc": "mc-a", "pod": "old-pod"},
		EtcdAdapterName: hcpStatusData(
			EtcdMemberStatus{Name: "my-cluster-etcd-m0", MC: "mc-a"},
			EtcdMemberStatus{Name: "my-cluster-etcd-m1", MC: "mc-c"},
			EtcdMemberStatus{Name: "my-cluster-etcd-m2", MC: "mc-b"},
		),
	}
	controller := hcpTestController(t, transport, statuses, "mc-a", "mc-b", "mc-c")

	report, err := controller.Reconcile(context.Background(), hcpTestCluster())

	Expect(err).ToNot(HaveOccurred())
	Expect(transport.deleted).To(ContainElement("mc-a/Namespace//hcp-my-cluster"))
	Expect(transport.applied).To(HaveKey("mc-c"))
	Expect(transport.applied).ToNot(HaveKey("mc-b"))
	Expect(*report.Conditions[0].Reason).To(Equal("ReHomed"))
	Expect(report.Data["mc"]).To(Equal("mc-c"))
	secret := findHCPObject(transport.applied["mc-c"], kindSecret, EtcdClientTLSName).(*corev1.Secret)
	Expect(secret.Data[EtcdCACertKey]).To(Equal([]byte("ca-mc-c")))
}

func TestHCPControllerPublishesKubeconfigWithoutLeakingCertMaterial(t *testing.T) {
	RegisterTestingT(t)
	transport := newHCPFakeTransport()
	transport.secrets["mc-a/etcd-my-cluster/my-cluster-etcd-m0-tls"] = hcpSecretData("SERVER-CERT-MARKER")
	transport.kubeconfig = []byte("apiVersion: v1\nkind: Config\n")
	transport.pods["mc-a"] = []ManifestState{{Kind: kindPod, Name: "hcp-pod", Ready: true}}
	statuses := hcpFakeStatus{
		EtcdAdapterName: hcpStatusData(
			EtcdMemberStatus{Name: "my-cluster-etcd-m0", MC: "mc-a"},
		),
	}
	controller := hcpTestController(t, transport, statuses, "mc-a", "mc-a")

	report, err := controller.Reconcile(context.Background(), hcpTestCluster())

	Expect(err).ToNot(HaveOccurred())
	Expect(report.Data["ready"]).To(Equal(true))
	Expect(report.Data["pod"]).To(Equal("hcp-pod"))
	Expect(report.Data["kubeconfigRef"]).To(Equal("apiVersion: v1\nkind: Config\n"))
	Expect(conditionStatus(report, api.AdapterConditionTypeAvailable)).To(Equal(api.AdapterConditionTrue))
	rendered := fmt.Sprint(report.Data)
	Expect(rendered).ToNot(ContainSubstring("SERVER-CERT-MARKER"))
	Expect(rendered).ToNot(ContainSubstring("PULL-SECRET-MARKER"))
}

func TestHCPControllerFinalizesWhenNothingWasMinted(t *testing.T) {
	RegisterTestingT(t)
	transport := newHCPFakeTransport()
	controller := hcpTestController(t, transport, hcpFakeStatus{}, "mc-a", "mc-a")
	cluster := hcpTestCluster()
	cluster.Deleting = true

	report, err := controller.Reconcile(context.Background(), cluster)

	Expect(err).ToNot(HaveOccurred())
	Expect(transport.deleted).To(BeEmpty())
	Expect(conditionStatus(report, api.AdapterConditionTypeFinalized)).
		To(Equal(api.AdapterConditionTrue))
}

func TestHCPControllerWaitsForNamespaceDeletion(t *testing.T) {
	RegisterTestingT(t)
	transport := newHCPFakeTransport()
	transport.namespaces["mc-a"] = map[string]bool{"hcp-my-cluster": true}
	transport.deferDelete = true
	statuses := hcpFakeStatus{HCPAdapterName: {"mc": "mc-a"}}
	controller := hcpTestController(t, transport, statuses, "mc-a", "mc-a")
	cluster := hcpTestCluster()
	cluster.Deleting = true

	report, err := controller.Reconcile(context.Background(), cluster)

	Expect(err).ToNot(HaveOccurred())
	Expect(conditionByType(report, api.AdapterConditionTypeFinalized)).To(BeNil())
	Expect(*report.Conditions[0].Reason).To(Equal("Deleting"))

	transport.deferDelete = false
	report, err = controller.Reconcile(context.Background(), cluster)

	Expect(err).ToNot(HaveOccurred())
	Expect(conditionStatus(report, api.AdapterConditionTypeFinalized)).
		To(Equal(api.AdapterConditionTrue))
}

func TestHCPControllerFinalizesWhenTheMCIsGone(t *testing.T) {
	RegisterTestingT(t)
	transport := newHCPFakeTransport()
	statuses := hcpFakeStatus{HCPAdapterName: {"mc": "mc-a"}}
	controller := hcpTestController(t, transport, statuses, "mc-b", "mc-b")
	cluster := hcpTestCluster()
	cluster.Deleting = true

	report, err := controller.Reconcile(context.Background(), cluster)

	Expect(err).ToNot(HaveOccurred())
	Expect(transport.deleted).To(BeEmpty())
	Expect(conditionStatus(report, api.AdapterConditionTypeFinalized)).
		To(Equal(api.AdapterConditionTrue))
}
