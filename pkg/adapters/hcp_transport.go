package adapters

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
)

// SecretReader reads a Secret's decoded data from a management cluster.
type SecretReader interface {
	ReadSecret(ctx context.Context, mc, namespace, name string) (map[string][]byte, error)
}

// PodExecer runs a command inside a pod on a management cluster.
type PodExecer interface {
	Exec(ctx context.Context, mc, namespace, pod string, command []string) ([]byte, error)
}

var (
	_ SecretReader = (*KubeconfigTransport)(nil)
	_ PodExecer    = (*KubeconfigTransport)(nil)
)

// ReadSecret returns decoded Secret data. Certificate material never leaves
// this call as text.
func (t *KubeconfigTransport) ReadSecret(
	ctx context.Context, mc, namespace, name string,
) (map[string][]byte, error) {
	client, err := t.clientForMC(mc)
	if err != nil {
		return nil, err
	}
	secret, err := client.Resource(etcdKindResources[kindSecret]).
		Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get secret %q in %q: %w", name, mc, err)
	}
	encoded, found, err := unstructured.NestedStringMap(secret.Object, "data")
	if err != nil || !found {
		return nil, fmt.Errorf("read secret %q data in %q: %w", name, mc, err)
	}
	data := make(map[string][]byte, len(encoded))
	for key, value := range encoded {
		decoded, decodeErr := base64.StdEncoding.DecodeString(value)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode secret %q key %q: %w", name, key, decodeErr)
		}
		data[key] = decoded
	}
	return data, nil
}

// Exec runs command inside pod and returns stdout.
func (t *KubeconfigTransport) Exec(
	ctx context.Context, mc, namespace, pod string, command []string,
) ([]byte, error) {
	t.mu.Lock()
	path := t.kubeconfigs[mc]
	t.mu.Unlock()
	if path == "" {
		return nil, fmt.Errorf("no kubeconfig configured for management cluster %q", mc)
	}
	config, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("build client for management cluster %q: %w", mc, err)
	}
	request := client.CoreV1().RESTClient().Post().
		Resource("pods").Namespace(namespace).Name(pod).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Command: command,
			Stdout:  true,
			Stderr:  true,
		}, scheme.ParameterCodec)
	executor, err := remotecommand.NewSPDYExecutor(config, http.MethodPost, request.URL())
	if err != nil {
		return nil, fmt.Errorf("build exec for pod %q in %q: %w", pod, mc, err)
	}

	var stdout, stderr bytes.Buffer
	err = executor.StreamWithContext(ctx, remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})
	if err != nil {
		return nil, fmt.Errorf(
			"exec in pod %q on %q: %w: %s", pod, mc, err, strings.TrimSpace(stderr.String()),
		)
	}
	return stdout.Bytes(), nil
}
