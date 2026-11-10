package adapters

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

// HCPAdapterName is the PoC stand-in for the product hcp adapter (E8). Its
// substrate is coreshift running as one pod on a management cluster.
const HCPAdapterName = "hcp"

// HCPOptions configures the nested coreshift adapter.
type HCPOptions struct {
	Image          string
	PullSecretFile string
	PreferredMC    string
}

// HCPController places a hosted control plane next to etcd member 0.
type HCPController struct {
	fleet     FleetLister
	transport Transport
	statuses  StatusDataReader
	opts      HCPOptions
}

var _ Controller = (*HCPController)(nil)

// NewHCPController returns an hcp controller backed by the fleet and transport.
func NewHCPController(
	fleetView FleetLister, transport Transport, statuses StatusDataReader, opts HCPOptions,
) *HCPController {
	return &HCPController{fleet: fleetView, transport: transport, statuses: statuses, opts: opts}
}

func (c *HCPController) Name() string { return HCPAdapterName }

func (c *HCPController) Reconcile(
	ctx context.Context, cluster ClusterView,
) (StatusReport, error) {
	if cluster.Deleting {
		return c.reconcileDelete(ctx, cluster)
	}

	prior, err := c.adapterData(ctx, cluster, HCPAdapterName)
	if err != nil {
		return hcpErrorReport(cluster, err), err
	}

	etcdData, err := c.adapterData(ctx, cluster, EtcdAdapterName)
	if err != nil {
		return hcpErrorReport(cluster, err), err
	}
	etcdStatus, err := decodeEtcdStatus(etcdData)
	if err != nil {
		return hcpErrorReport(cluster, err), err
	}
	if len(etcdStatus.Endpoints) == 0 {
		return hcpBlockedReport(cluster), nil
	}

	mcs, err := c.fleet()
	if err != nil {
		err = fmt.Errorf("list management fleet: %w", err)
		return hcpErrorReport(cluster, err), err
	}
	names := make([]string, len(mcs))
	for index, mc := range mcs {
		names[index] = mc.Name
	}

	previous := hcpStateFrom(prior)
	rehomed := previous.MC != "" && !contains(names, previous.MC)
	if rehomed {
		c.dropLostNamespace(ctx, previous.MC, hcpNamespace(cluster.Name))
	}

	target := previous.MC
	if target == "" || rehomed {
		target = hcpTargetMC(names, etcdStatus.Members, c.opts.PreferredMC)
	}
	if target == "" {
		err = fmt.Errorf("no management cluster available for the hcp control plane")
		return hcpErrorReport(cluster, err), err
	}

	namespace := hcpNamespace(cluster.Name)
	etcdSecret, err := c.etcdClientSecret(ctx, cluster.Name, namespace, target, etcdStatus.Members)
	if err != nil {
		return hcpErrorReport(cluster, err), err
	}
	pullSecret, err := c.pullSecret()
	if err != nil {
		return hcpErrorReport(cluster, err), err
	}

	objects, err := RenderHCPManifests(HCPManifestInput{
		Cluster:    cluster.Name,
		Namespace:  namespace,
		Image:      c.opts.Image,
		PullSecret: pullSecret,
		EtcdSecret: etcdSecret,
		Endpoints:  etcdStatus.Endpoints,
	})
	if err != nil {
		return hcpErrorReport(cluster, err), err
	}
	if applyErr := c.transport.Apply(ctx, target, objects); applyErr != nil {
		err = fmt.Errorf("apply hcp manifests to %s: %w", target, applyErr)
		return hcpErrorReport(cluster, err), err
	}

	pod, kubeconfig, ready, err := c.observe(ctx, cluster.Name, target)
	if err != nil {
		return hcpErrorReport(cluster, err), err
	}
	data := hcpData(target, pod, c.opts.Image, ready, kubeconfig)
	if rehomed {
		message := "Hosted control plane moved to " + target
		if ready {
			return hcpReadyReport(cluster, data, "ReHomed", message), nil
		}
		return hcpPendingReport(cluster, "ReHomed", message, data), nil
	}
	if ready {
		return hcpReadyReport(cluster, data, "Ready", "Hosted control plane is ready"), nil
	}
	return hcpPendingReport(cluster, "Creating", "Hosted control plane is booting", data), nil
}

// reconcileDelete removes the hosted control plane namespace and finalizes once
// it is gone. A lost MC takes its namespace with it.
func (c *HCPController) reconcileDelete(
	ctx context.Context, cluster ClusterView,
) (StatusReport, error) {
	prior, err := c.adapterData(ctx, cluster, HCPAdapterName)
	if err != nil {
		return hcpTeardownErrorReport(cluster, err), err
	}
	state := hcpStateFrom(prior)
	if state.MC == "" {
		return hcpFinalizedReport(cluster), nil
	}

	mcs, err := c.fleet()
	if err != nil {
		err = fmt.Errorf("list management fleet: %w", err)
		return hcpTeardownErrorReport(cluster, err), err
	}
	names := make([]string, len(mcs))
	for index, mc := range mcs {
		names[index] = mc.Name
	}
	if !contains(names, state.MC) {
		return hcpFinalizedReport(cluster), nil
	}

	namespace := hcpNamespace(cluster.Name)
	if deleteErr := c.transport.Delete(ctx, state.MC, kindNamespace, "", namespace); deleteErr != nil {
		err = fmt.Errorf("delete hcp namespace %q: %w", namespace, deleteErr)
		return hcpTeardownErrorReport(cluster, err), err
	}
	gone, err := c.namespaceGone(ctx, state.MC, namespace)
	if err != nil {
		return hcpTeardownErrorReport(cluster, err), err
	}
	if !gone {
		return hcpDeletingReport(cluster), nil
	}
	return hcpFinalizedReport(cluster), nil
}

func (c *HCPController) dropLostNamespace(ctx context.Context, mc, namespace string) {
	// The old MC is gone; its namespace goes with it. Best effort, because the
	// transport may no longer hold a kubeconfig for that MC.
	if err := c.transport.Delete(ctx, mc, kindNamespace, "", namespace); err != nil {
		return
	}
}

func (c *HCPController) adapterData(
	ctx context.Context, cluster ClusterView, adapter string,
) (map[string]interface{}, error) {
	if c.statuses == nil {
		return nil, nil
	}
	return c.statuses(ctx, cluster, adapter)
}

func (c *HCPController) pullSecret() ([]byte, error) {
	if c.opts.PullSecretFile == "" {
		return nil, fmt.Errorf("hcp pull secret file is not configured")
	}
	content, err := os.ReadFile(c.opts.PullSecretFile)
	if err != nil {
		return nil, fmt.Errorf("read hcp pull secret: %w", err)
	}
	return content, nil
}

func (c *HCPController) etcdClientSecret(
	ctx context.Context, cluster, namespace, target string, members []EtcdMemberStatus,
) (EtcdSecret, error) {
	member, found := hcpEtcdMember(members, target)
	if !found {
		member = EtcdMemberStatus{Name: cluster + "-etcd-m0", MC: target}
	}
	reader, ok := c.transport.(SecretReader)
	if !ok {
		return EtcdSecret{}, fmt.Errorf("transport does not support reading secrets")
	}
	name := member.Name + "-tls"
	data, err := reader.ReadSecret(ctx, member.MC, "etcd-"+cluster, name)
	if err != nil {
		return EtcdSecret{}, fmt.Errorf("read etcd member secret %q: %w", name, err)
	}
	material := EtcdSecret{
		Name:      EtcdClientTLSName,
		Namespace: namespace,
		Data: map[string][]byte{
			EtcdCACertKey:     data[EtcdCACertKey],
			EtcdClientCertKey: data[EtcdClientCertKey],
			EtcdClientKeyKey:  data[EtcdClientKeyKey],
		},
	}
	if err := validateHCPEtcdSecret(material); err != nil {
		return EtcdSecret{}, err
	}
	return material, nil
}

func (c *HCPController) observe(
	ctx context.Context, cluster, mc string,
) (string, string, bool, error) {
	selector := hcpClusterLabel + "=" + cluster
	pods, err := c.transport.List(ctx, mc, kindPod, hcpNamespace(cluster), selector)
	if err != nil {
		return "", "", false, fmt.Errorf("list hcp pods on %s: %w", mc, err)
	}
	for _, pod := range pods {
		if !pod.Ready {
			continue
		}
		execer, ok := c.transport.(PodExecer)
		if !ok {
			return "", "", false, fmt.Errorf("transport does not support pod exec")
		}
		output, execErr := execer.Exec(
			ctx, mc, hcpNamespace(cluster), pod.Name, []string{"cat", hcpKubeconfigPath},
		)
		if execErr != nil || len(output) == 0 {
			continue
		}
		return pod.Name, string(output), true, nil
	}
	if len(pods) > 0 {
		return pods[0].Name, "", false, nil
	}
	return "", "", false, nil
}

func (c *HCPController) namespaceGone(
	ctx context.Context, mc, namespace string,
) (bool, error) {
	states, err := c.transport.List(ctx, mc, kindNamespace, "", "")
	if err != nil {
		return false, fmt.Errorf("list hcp namespaces on %q: %w", mc, err)
	}
	for _, state := range states {
		if state.Name == namespace {
			return false, nil
		}
	}
	return true, nil
}

func hcpNamespace(cluster string) string { return "hcp-" + cluster }

func hcpTargetMC(names []string, members []EtcdMemberStatus, preferred string) string {
	inFleet := make(map[string]bool, len(names))
	for _, name := range names {
		inFleet[name] = true
	}
	if inFleet[preferred] {
		for _, member := range members {
			if member.MC == preferred {
				return preferred
			}
		}
	}
	for _, member := range members {
		if inFleet[member.MC] {
			return member.MC
		}
	}
	if inFleet[preferred] {
		return preferred
	}
	if len(names) == 0 {
		return ""
	}
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	return sorted[0]
}

func hcpEtcdMember(members []EtcdMemberStatus, mc string) (EtcdMemberStatus, bool) {
	for _, member := range members {
		if member.MC == mc {
			return member, true
		}
	}
	if len(members) > 0 {
		return members[0], true
	}
	return EtcdMemberStatus{}, false
}

func decodeEtcdStatus(data map[string]interface{}) (EtcdStatusData, error) {
	if data == nil {
		return EtcdStatusData{}, nil
	}
	content, err := json.Marshal(data)
	if err != nil {
		return EtcdStatusData{}, fmt.Errorf("encode etcd status data: %w", err)
	}
	var status EtcdStatusData
	if err := json.Unmarshal(content, &status); err != nil {
		return EtcdStatusData{}, fmt.Errorf("decode etcd status data: %w", err)
	}
	return status, nil
}

type hcpState struct {
	MC string
}

func hcpStateFrom(data map[string]interface{}) hcpState {
	if data == nil {
		return hcpState{}
	}
	mc := ""
	if value, ok := data["mc"].(string); ok {
		mc = value
	}
	return hcpState{MC: mc}
}

func hcpData(mc, pod, image string, ready bool, kubeconfig string) map[string]interface{} {
	return map[string]interface{}{
		"mc":            mc,
		"pod":           pod,
		"imageRef":      image,
		"ready":         ready,
		"kubeconfigRef": kubeconfig,
	}
}

func hcpBlockedReport(cluster ClusterView) StatusReport {
	const reason = "WaitingForEtcd"
	const message = "Etcd endpoints are not ready"
	return StatusReport{
		ObservedGeneration: cluster.Generation,
		Conditions: []api.AdapterCondition{
			condition(api.AdapterConditionTypeAvailable, api.AdapterConditionFalse, reason, message),
			condition(api.AdapterConditionTypeApplied, api.AdapterConditionFalse, reason, message),
			condition(api.AdapterConditionTypeHealth, api.AdapterConditionFalse, reason, message),
		},
	}
}

func hcpPendingReport(
	cluster ClusterView, reason, message string, data map[string]interface{},
) StatusReport {
	return StatusReport{
		ObservedGeneration: cluster.Generation,
		Conditions: []api.AdapterCondition{
			condition(api.AdapterConditionTypeAvailable, api.AdapterConditionFalse, reason, message),
			condition(api.AdapterConditionTypeApplied, api.AdapterConditionTrue, reason, message),
			condition(api.AdapterConditionTypeHealth, api.AdapterConditionFalse, reason, message),
		},
		Data: data,
	}
}

func hcpReadyReport(
	cluster ClusterView, data map[string]interface{}, reason, message string,
) StatusReport {
	return StatusReport{
		ObservedGeneration: cluster.Generation,
		Conditions: []api.AdapterCondition{
			condition(api.AdapterConditionTypeAvailable, api.AdapterConditionTrue, reason, message),
			condition(api.AdapterConditionTypeApplied, api.AdapterConditionTrue, reason, message),
			condition(api.AdapterConditionTypeHealth, api.AdapterConditionTrue, reason, message),
		},
		Data: data,
	}
}

func hcpDeletingReport(cluster ClusterView) StatusReport {
	const reason = "Deleting"
	const message = "Hosted control plane teardown is in progress"
	return StatusReport{
		ObservedGeneration: cluster.Generation,
		Conditions: []api.AdapterCondition{
			condition(api.AdapterConditionTypeAvailable, api.AdapterConditionFalse, reason, message),
			condition(api.AdapterConditionTypeApplied, api.AdapterConditionTrue, reason, message),
			condition(api.AdapterConditionTypeHealth, api.AdapterConditionFalse, reason, message),
		},
	}
}

func hcpFinalizedReport(cluster ClusterView) StatusReport {
	const reason = "Finalized"
	const message = "Hosted control plane has been removed"
	return StatusReport{
		ObservedGeneration: cluster.Generation,
		Conditions: []api.AdapterCondition{
			condition(api.AdapterConditionTypeAvailable, api.AdapterConditionFalse, reason, message),
			condition(api.AdapterConditionTypeApplied, api.AdapterConditionTrue, reason, message),
			condition(api.AdapterConditionTypeHealth, api.AdapterConditionFalse, reason, message),
			condition(api.AdapterConditionTypeFinalized, api.AdapterConditionTrue, reason, message),
		},
	}
}

func hcpTeardownErrorReport(cluster ClusterView, err error) StatusReport {
	const reason = "TeardownFailed"
	message := err.Error()
	return StatusReport{
		ObservedGeneration: cluster.Generation,
		Conditions: []api.AdapterCondition{
			condition(api.AdapterConditionTypeAvailable, api.AdapterConditionFalse, reason, message),
			condition(api.AdapterConditionTypeApplied, api.AdapterConditionFalse, reason, message),
			condition(api.AdapterConditionTypeHealth, api.AdapterConditionFalse, reason, message),
		},
	}
}

func hcpErrorReport(cluster ClusterView, err error) StatusReport {
	const reason = "Error"
	message := err.Error()
	return StatusReport{
		ObservedGeneration: cluster.Generation,
		Conditions: []api.AdapterCondition{
			condition(api.AdapterConditionTypeAvailable, api.AdapterConditionFalse, reason, message),
			condition(api.AdapterConditionTypeApplied, api.AdapterConditionFalse, reason, message),
			condition(api.AdapterConditionTypeHealth, api.AdapterConditionFalse, reason, message),
		},
	}
}
