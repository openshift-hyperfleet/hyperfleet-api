package adapters

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	etcdDataDir    = "/var/lib/data"
	etcdTLSDir     = "/etc/etcd/tls"
	etcdStateDir   = "/etc/etcd/clusterstate"
	etcdDataVolume = "data"
	etcdTLSVolume  = "tls"
	etcdStateVol   = "cluster-state"

	etcdManifestsPerMember = 3
)

// RenderEtcdManifests returns the Secret, Service, and StatefulSet for every
// member slot. Applying the result is the transport's job.
func RenderEtcdManifests(
	membership Membership, pki *EtcdPKI, opts EtcdOptions,
) ([]runtime.Object, error) {
	if pki == nil {
		return nil, fmt.Errorf("etcd PKI is required")
	}
	opts = opts.withDefaults()
	if _, err := resource.ParseQuantity(opts.StorageSize); err != nil {
		return nil, fmt.Errorf("invalid etcd storage size %q: %w", opts.StorageSize, err)
	}

	objects := make([]runtime.Object, 0, len(membership.Slots)*etcdManifestsPerMember)
	for _, slot := range membership.Slots {
		memberObjects, err := renderMemberManifests(membership, slot, pki, opts)
		if err != nil {
			return nil, err
		}
		objects = append(objects, memberObjects...)
	}
	return objects, nil
}

// RenderEtcdManifestsByMC groups each member's manifests by target MC.
func RenderEtcdManifestsByMC(
	membership Membership, pki *EtcdPKI, opts EtcdOptions,
) (map[string][]runtime.Object, error) {
	objects, err := RenderEtcdManifests(membership, pki, opts)
	if err != nil {
		return nil, err
	}

	byMC := make(map[string][]runtime.Object, len(membership.Slots))
	for index, slot := range membership.Slots {
		start := index * etcdManifestsPerMember
		byMC[slot.TargetMC] = append(
			byMC[slot.TargetMC],
			objects[start:start+etcdManifestsPerMember]...,
		)
	}
	return byMC, nil
}

func renderMemberManifests(
	membership Membership, slot MemberSlot, pki *EtcdPKI, opts EtcdOptions,
) ([]runtime.Object, error) {
	if pki.ServerCerts[slot.MemberName].CertPEM == nil ||
		pki.PeerCerts[slot.MemberName].CertPEM == nil {
		return nil, fmt.Errorf("missing cert material for etcd member %q", slot.MemberName)
	}
	return []runtime.Object{
		memberSecret(membership, slot, pki),
		memberService(membership, slot),
		memberStatefulSet(membership, slot, pki, opts),
	}, nil
}

func memberNamespace(membership Membership) *corev1.Namespace {
	return &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: kindNamespace},
		ObjectMeta: metav1.ObjectMeta{
			Name:   membership.Namespace,
			Labels: map[string]string{"app.kubernetes.io/instance": membership.ClusterName},
		},
	}
}

func memberSecret(membership Membership, slot MemberSlot, pki *EtcdPKI) *corev1.Secret {
	material := pki.MemberSecret(slot)
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: kindSecret},
		ObjectMeta: metav1.ObjectMeta{
			Name:      material.Name,
			Namespace: material.Namespace,
			Labels:    memberLabels(membership, slot),
		},
		Type: corev1.SecretTypeOpaque,
		Data: material.Data,
	}
}

func memberService(membership Membership, slot MemberSlot) *corev1.Service {
	labels := memberLabels(membership, slot)
	spec := corev1.ServiceSpec{
		Selector: labels,
		Ports: []corev1.ServicePort{
			{
				Name:       "client",
				Port:       etcdClientPort,
				TargetPort: intstr.FromInt32(etcdClientPort),
			},
			{
				Name:       "peer",
				Port:       etcdPeerPort,
				TargetPort: intstr.FromInt32(etcdPeerPort),
			},
		},
	}
	if slot.NodeIP != "" {
		spec.Type = corev1.ServiceTypeNodePort
		spec.Ports[0].NodePort = slot.ClientNodePort
		spec.Ports[1].NodePort = slot.PeerNodePort
	}
	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: kindService},
		ObjectMeta: metav1.ObjectMeta{
			Name:      slot.MemberName,
			Namespace: slot.Namespace,
			Labels:    labels,
		},
		Spec: spec,
	}
}

func memberStatefulSet(
	membership Membership, slot MemberSlot, pki *EtcdPKI, opts EtcdOptions,
) *appsv1.StatefulSet {
	labels := memberLabels(membership, slot)
	replicas := int32(1)
	mounts := memberVolumeMounts()

	var storageClass *string
	if opts.StorageClass != "" {
		storageClass = &opts.StorageClass
	}

	return &appsv1.StatefulSet{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: kindStatefulSet},
		ObjectMeta: metav1.ObjectMeta{
			Name:      slot.MemberName,
			Namespace: slot.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: slot.MemberName,
			Selector:    &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					InitContainers: []corev1.Container{
						resetMemberContainer(membership, slot, opts.Image, mounts),
					},
					Containers: []corev1.Container{
						etcdContainer(membership, slot, opts.Image, mounts),
					},
					Volumes: []corev1.Volume{
						{
							Name: etcdTLSVolume,
							VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{
								SecretName: pki.MemberSecret(slot).Name,
							}},
						},
						{
							Name:         etcdStateVol,
							VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{{
				ObjectMeta: metav1.ObjectMeta{Name: etcdDataVolume},
				Spec: corev1.PersistentVolumeClaimSpec{
					AccessModes:      []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
					StorageClassName: storageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: resource.MustParse(opts.StorageSize),
						},
					},
				},
			}},
		},
	}
}

func etcdContainer(
	membership Membership, slot MemberSlot, image string, mounts []corev1.VolumeMount,
) corev1.Container {
	args := []string{
		"--name=" + slot.MemberName,
		"--data-dir=" + etcdDataDir,
		"--listen-client-urls=https://0.0.0.0:2379",
		"--advertise-client-urls=" + slot.ClientURL,
		"--listen-peer-urls=https://0.0.0.0:2380",
		"--initial-advertise-peer-urls=" + slot.PeerURL,
		"--initial-cluster=" + initialCluster(membership),
		"--initial-cluster-token=" + membership.ClusterName,
		"--client-cert-auth",
		"--trusted-ca-file=" + etcdTLSDir + "/" + EtcdCACertKey,
		"--cert-file=" + etcdTLSDir + "/" + EtcdServerCertKey,
		"--key-file=" + etcdTLSDir + "/" + EtcdServerKeyKey,
		"--peer-client-cert-auth",
		"--experimental-peer-skip-client-san-verification",
		"--peer-trusted-ca-file=" + etcdTLSDir + "/" + EtcdCACertKey,
		"--peer-cert-file=" + etcdTLSDir + "/" + EtcdPeerCertKey,
		"--peer-key-file=" + etcdTLSDir + "/" + EtcdPeerKeyKey,
		"--tls-min-version=TLS1.2",
	}
	command := "ETCD_INITIAL_CLUSTER_STATE=$(cat " + etcdStateDir + "/state) exec etcd " +
		strings.Join(args, " ")

	return corev1.Container{
		Name:    "etcd",
		Image:   image,
		Command: []string{"/bin/sh", "-c"},
		Args:    []string{command},
		Ports: []corev1.ContainerPort{
			{Name: "client", ContainerPort: etcdClientPort},
			{Name: "peer", ContainerPort: etcdPeerPort},
		},
		VolumeMounts: mounts,
	}
}

func resetMemberContainer(
	membership Membership, slot MemberSlot, image string, mounts []corev1.VolumeMount,
) corev1.Container {
	return corev1.Container{
		Name:         "reset-member",
		Image:        image,
		Command:      []string{"/bin/sh", "-c"},
		Args:         []string{rejoinScript(membership, slot)},
		VolumeMounts: mounts,
	}
}

func memberVolumeMounts() []corev1.VolumeMount {
	return []corev1.VolumeMount{
		{Name: etcdDataVolume, MountPath: etcdDataDir},
		{Name: etcdTLSVolume, MountPath: etcdTLSDir},
		{Name: etcdStateVol, MountPath: etcdStateDir},
	}
}

func rejoinScript(membership Membership, slot MemberSlot) string {
	return fmt.Sprintf(`#!/bin/sh
set -eu
export ETCDCTL_API=3
export ETCDCTL_CACERT=%[1]s/%[2]s
export ETCDCTL_CERT=%[1]s/%[10]s
export ETCDCTL_KEY=%[1]s/%[11]s
export ETCDCTL_ENDPOINTS=%[5]s

echo new > %[6]s/state
if [ -f %[7]s/member/snap/db ]; then
  echo "etcd data exists, not re-joining"
  exit 0
fi

if ! etcdctl member list >/dev/null 2>&1; then
  echo "etcd cluster is not reachable, starting a new cluster"
  exit 0
fi

MEMBER_ID=$(etcdctl member list -w simple | awk -F, -v name="%[8]s" -v peer="%[9]s" \
  '{ gsub(/^ +| +$/, "", $3); gsub(/^ +| +$/, "", $4); \
     if ($3 == name || $4 == peer) { print $1; exit } }')
if [ -n "${MEMBER_ID}" ]; then
  etcdctl member remove "${MEMBER_ID}"
fi
etcdctl member add "%[8]s" --peer-urls "%[9]s"
echo existing > %[6]s/state
`,
		etcdTLSDir, EtcdCACertKey, EtcdServerCertKey, EtcdServerKeyKey,
		strings.Join(membership.Endpoints(), ","), etcdStateDir, etcdDataDir,
		slot.MemberName, slot.PeerURL, EtcdClientCertKey, EtcdClientKeyKey,
	)
}

func initialCluster(membership Membership) string {
	parts := make([]string, 0, len(membership.Slots))
	for _, slot := range membership.Slots {
		parts = append(parts, slot.MemberName+"="+slot.PeerURL)
	}
	return strings.Join(parts, ",")
}

func memberLabels(membership Membership, slot MemberSlot) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     "etcd",
		"app.kubernetes.io/instance": membership.ClusterName,
		EtcdMemberLabel:              slot.Name,
	}
}
