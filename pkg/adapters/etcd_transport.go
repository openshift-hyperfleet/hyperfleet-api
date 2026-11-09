package adapters

import (
	"context"
	"fmt"
	"sort"
	"sync"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	kindPod         = "Pod"
	kindService     = "Service"
	kindSecret      = "Secret"
	kindStatefulSet = "StatefulSet"
)

var etcdKindResources = map[string]schema.GroupVersionResource{
	kindPod:         corev1.SchemeGroupVersion.WithResource("pods"),
	kindService:     corev1.SchemeGroupVersion.WithResource("services"),
	kindSecret:      corev1.SchemeGroupVersion.WithResource("secrets"),
	kindStatefulSet: appsv1.SchemeGroupVersion.WithResource("statefulsets"),
}

// ManifestState is the observed state of one object read back from an MC.
type ManifestState struct {
	Kind      string
	Name      string
	Namespace string
	Ready     bool
	Labels    map[string]string
}

// Transport delivers manifests to a target management cluster and reads
// state back. ManifestWork is the product implementation; this is the port.
type Transport interface {
	Apply(ctx context.Context, mc string, objects []runtime.Object) error
	List(ctx context.Context, mc, kind, namespace, labelSelector string) ([]ManifestState, error)
}

// KubeconfigTransport talks to each MC through a kubeconfig path.
type KubeconfigTransport struct {
	kubeconfigs map[string]string
	newClient   func(path string) (dynamic.Interface, error)
	clients     map[string]dynamic.Interface
	mu          sync.Mutex
}

var _ Transport = (*KubeconfigTransport)(nil)

// NewKubeconfigTransport returns a transport keyed by management cluster name.
func NewKubeconfigTransport(kubeconfigs map[string]string) *KubeconfigTransport {
	copied := make(map[string]string, len(kubeconfigs))
	for mc, path := range kubeconfigs {
		copied[mc] = path
	}
	return &KubeconfigTransport{
		kubeconfigs: copied,
		newClient:   dynamicClientFromKubeconfig,
		clients:     make(map[string]dynamic.Interface),
	}
}

func (t *KubeconfigTransport) Apply(ctx context.Context, mc string, objects []runtime.Object) error {
	client, err := t.clientForMC(mc)
	if err != nil {
		return err
	}
	for _, object := range objects {
		manifest, convErr := toUnstructured(object)
		if convErr != nil {
			return convErr
		}
		gvr, gvrErr := gvrForObject(manifest)
		if gvrErr != nil {
			return gvrErr
		}
		resource := client.Resource(gvr).Namespace(manifest.GetNamespace())
		existing, getErr := resource.Get(ctx, manifest.GetName(), metav1.GetOptions{})
		switch {
		case apierrors.IsNotFound(getErr):
			if _, err = resource.Create(ctx, manifest, metav1.CreateOptions{}); err != nil {
				return fmt.Errorf("create %s %q: %w", gvr.Resource, manifest.GetName(), err)
			}
		case getErr != nil:
			return fmt.Errorf("get %s %q: %w", gvr.Resource, manifest.GetName(), getErr)
		default:
			manifest.SetResourceVersion(existing.GetResourceVersion())
			if _, err = resource.Update(ctx, manifest, metav1.UpdateOptions{}); err != nil {
				return fmt.Errorf("update %s %q: %w", gvr.Resource, manifest.GetName(), err)
			}
		}
	}
	return nil
}

func (t *KubeconfigTransport) List(
	ctx context.Context, mc, kind, namespace, labelSelector string,
) ([]ManifestState, error) {
	gvr, ok := etcdKindResources[kind]
	if !ok {
		return nil, fmt.Errorf("unsupported manifest kind %q", kind)
	}
	client, err := t.clientForMC(mc)
	if err != nil {
		return nil, err
	}
	list, err := client.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("list %s in %q: %w", gvr.Resource, mc, err)
	}

	states := make([]ManifestState, 0, len(list.Items))
	for index := range list.Items {
		item := &list.Items[index]
		states = append(states, ManifestState{
			Kind:      kind,
			Name:      item.GetName(),
			Namespace: item.GetNamespace(),
			Ready:     manifestReady(item),
			Labels:    item.GetLabels(),
		})
	}
	sort.Slice(states, func(i, j int) bool { return states[i].Name < states[j].Name })
	return states, nil
}

// ObservedHealth maps ready member pods back to their slot member names.
func (m Membership) ObservedHealth(pods []ManifestState) map[string]bool {
	slots := make(map[string]string, len(m.Slots))
	for _, slot := range m.Slots {
		slots[slot.Name] = slot.MemberName
	}
	healthy := make(map[string]bool, len(m.Slots))
	for _, pod := range pods {
		if memberName, ok := slots[pod.Labels[EtcdMemberLabel]]; ok && pod.Ready {
			healthy[memberName] = true
		}
	}
	return healthy
}

func (t *KubeconfigTransport) clientForMC(mc string) (dynamic.Interface, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if client, ok := t.clients[mc]; ok {
		return client, nil
	}
	path, ok := t.kubeconfigs[mc]
	if !ok || path == "" {
		return nil, fmt.Errorf("no kubeconfig configured for management cluster %q", mc)
	}
	client, err := t.newClient(path)
	if err != nil {
		return nil, fmt.Errorf("build client for management cluster %q: %w", mc, err)
	}
	t.clients[mc] = client
	return client, nil
}

func dynamicClientFromKubeconfig(path string) (dynamic.Interface, error) {
	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	return dynamic.NewForConfig(config)
}

func toUnstructured(object runtime.Object) (*unstructured.Unstructured, error) {
	if manifest, ok := object.(*unstructured.Unstructured); ok {
		return manifest, nil
	}
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
	if err != nil {
		return nil, fmt.Errorf("convert manifest: %w", err)
	}
	return &unstructured.Unstructured{Object: content}, nil
}

func gvrForObject(object *unstructured.Unstructured) (schema.GroupVersionResource, error) {
	if object.GetKind() == "" {
		return schema.GroupVersionResource{}, fmt.Errorf("manifest %q has no kind", object.GetName())
	}
	gvr, _ := meta.UnsafeGuessKindToResource(object.GroupVersionKind())
	return gvr, nil
}

func manifestReady(object *unstructured.Unstructured) bool {
	switch object.GetKind() {
	case kindPod:
		conditions, found, err := unstructured.NestedSlice(object.Object, "status", "conditions")
		if err != nil || !found {
			return false
		}
		for _, item := range conditions {
			condition, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if condition["type"] == "Ready" && condition["status"] == "True" {
				return true
			}
		}
	case kindStatefulSet:
		ready, found, err := unstructured.NestedInt64(object.Object, "status", "readyReplicas")
		return err == nil && found && ready > 0
	}
	return false
}
