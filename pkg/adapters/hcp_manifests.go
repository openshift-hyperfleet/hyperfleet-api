package adapters

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	hcpStateVolume      = "state"
	hcpCacheVolume      = "cache"
	hcpPullSecretVolume = "pull-secret"
	hcpEtcdClientVolume = "etcd-client"

	hcpStateDir      = "/var/lib/coreshift/state"
	hcpCacheDir      = "/var/lib/coreshift/cache"
	hcpEtcdClientDir = "/etc/etcd-client"
	hcpSecretDir     = "/secret"

	hcpPullSecretName = "pull-secret"
	hcpPullSecretKey  = "pull-secret"
	hcpStateClaimName = "hcp-state"
	hcpStorageSize    = "2Gi"

	hcpKubeconfigPath = hcpStateDir + "/coreshift/clusters/coreshift/admin.kubeconfig"
	hcpClusterLabel   = "hcp.hyperfleet.io/cluster"
)

// HCPManifestInput is everything needed to render one nested control plane.
type HCPManifestInput struct {
	Cluster    string
	Namespace  string
	Image      string
	PullSecret []byte
	EtcdSecret EtcdSecret
	Endpoints  []string
}

// RenderHCPManifests renders the namespace, client secret, pull secret, state
// claim, and single-pod deployment for a hosted control plane.
func RenderHCPManifests(in HCPManifestInput) ([]runtime.Object, error) {
	if in.Cluster == "" || in.Namespace == "" {
		return nil, fmt.Errorf("hcp cluster and namespace are required")
	}
	if in.Image == "" {
		return nil, fmt.Errorf("hcp image is required")
	}
	if len(in.PullSecret) == 0 {
		return nil, fmt.Errorf("hcp pull secret is empty")
	}
	if len(in.Endpoints) == 0 {
		return nil, fmt.Errorf("hcp requires etcd endpoints")
	}
	if err := validateHCPEtcdSecret(in.EtcdSecret); err != nil {
		return nil, err
	}

	labels := hcpLabels(in.Cluster)
	return []runtime.Object{
		hcpNamespaceObject(in.Namespace, in.Cluster),
		hcpPullSecret(in.Namespace, in.Cluster, in.PullSecret),
		hcpEtcdClientSecret(in.Namespace, in.Cluster, in.EtcdSecret),
		hcpStateClaim(in.Namespace, in.Cluster),
		hcpDeployment(in, labels),
	}, nil
}

func validateHCPEtcdSecret(secret EtcdSecret) error {
	if len(secret.Data) != 3 {
		return fmt.Errorf("hcp etcd client secret must have exactly 3 keys, got %d", len(secret.Data))
	}
	for _, key := range []string{EtcdCACertKey, EtcdClientCertKey, EtcdClientKeyKey} {
		if len(secret.Data[key]) == 0 {
			return fmt.Errorf("hcp etcd client secret is missing non-empty key %q", key)
		}
	}
	return nil
}

func hcpLabels(cluster string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "coreshift",
		"app.kubernetes.io/instance": cluster,
		hcpClusterLabel:              cluster,
	}
}

func hcpNamespaceObject(namespace, cluster string) *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: kindNamespace},
		ObjectMeta: metav1.ObjectMeta{
			Name:   namespace,
			Labels: map[string]string{"app.kubernetes.io/instance": cluster},
		},
	}
}

func hcpPullSecret(namespace, cluster string, pullSecret []byte) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: kindSecret},
		ObjectMeta: metav1.ObjectMeta{
			Name:      hcpPullSecretName,
			Namespace: namespace,
			Labels:    hcpLabels(cluster),
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{hcpPullSecretKey: pullSecret},
	}
}

func hcpEtcdClientSecret(namespace, cluster string, material EtcdSecret) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: kindSecret},
		ObjectMeta: metav1.ObjectMeta{
			Name:      EtcdClientTLSName,
			Namespace: namespace,
			Labels:    hcpLabels(cluster),
		},
		Type: corev1.SecretTypeOpaque,
		Data: material.Data,
	}
}

func hcpStateClaim(namespace, cluster string) *corev1.PersistentVolumeClaim {
	return &corev1.PersistentVolumeClaim{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: kindPersistentVolumeClaim},
		ObjectMeta: metav1.ObjectMeta{
			Name:      hcpStateClaimName,
			Namespace: namespace,
			Labels:    hcpLabels(cluster),
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(hcpStorageSize),
				},
			},
		},
	}
}

func hcpDeployment(in HCPManifestInput, labels map[string]string) *appsv1.Deployment {
	replicas := int32(1)
	automount := false
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "hcp",
			Namespace: in.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Strategy: appsv1.DeploymentStrategy{Type: appsv1.RecreateDeploymentStrategyType},
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					AutomountServiceAccountToken: &automount,
					Containers:                   []corev1.Container{hcpContainer(in)},
					Volumes: []corev1.Volume{
						{Name: hcpStateVolume, VolumeSource: corev1.VolumeSource{
							PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
								ClaimName: hcpStateClaimName,
							},
						}},
						{Name: hcpCacheVolume, VolumeSource: corev1.VolumeSource{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						}},
						{Name: hcpPullSecretVolume, VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: hcpPullSecretName},
						}},
						{Name: hcpEtcdClientVolume, VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{SecretName: EtcdClientTLSName},
						}},
					},
				},
			},
		},
	}
}

func hcpContainer(in HCPManifestInput) corev1.Container {
	return corev1.Container{
		Name:            "coreshift",
		Image:           in.Image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Args: []string{
			"--pull-secret", hcpSecretDir + "/" + hcpPullSecretKey,
			"--external-etcd-endpoints=" + strings.Join(in.Endpoints, ","),
			"--external-etcd-ca=" + hcpEtcdClientDir + "/" + EtcdCACertKey,
			"--external-etcd-cert=" + hcpEtcdClientDir + "/" + EtcdClientCertKey,
			"--external-etcd-key=" + hcpEtcdClientDir + "/" + EtcdClientKeyKey,
		},
		Env: []corev1.EnvVar{
			{Name: "XDG_STATE_HOME", Value: hcpStateDir},
			{Name: "XDG_CACHE_HOME", Value: hcpCacheDir},
		},
		VolumeMounts: []corev1.VolumeMount{
			{Name: hcpStateVolume, MountPath: hcpStateDir},
			{Name: hcpCacheVolume, MountPath: hcpCacheDir},
			{Name: hcpPullSecretVolume, MountPath: hcpSecretDir, ReadOnly: true},
			{Name: hcpEtcdClientVolume, MountPath: hcpEtcdClientDir, ReadOnly: true},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("2Gi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		},
	}
}
