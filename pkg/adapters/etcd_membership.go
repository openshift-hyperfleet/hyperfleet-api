package adapters

import (
	"strconv"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

const (
	EtcdAdapterName  = "etcd"
	defaultEtcdSlots = 3
	EtcdMemberLabel  = "etcd.hyperfleet.io/member"
	etcdClientPort   = 2379
	etcdPeerPort     = 2380
)

// EtcdOptions configures the etcd adapter. Zero values fall back to defaults.
//
//nolint:govet // fieldalignment: grouping beats padding for readability
type EtcdOptions struct {
	Image        string
	StorageClass string
	StorageSize  string
	Slots        int
	// Namespace overrides the per-cluster namespace derived from the name.
	Namespace string
}

func (o EtcdOptions) withDefaults() EtcdOptions {
	if o.Image == "" {
		o.Image = "quay.io/coreos/etcd:v3.5.20"
	}
	if o.StorageSize == "" {
		o.StorageSize = "8Gi"
	}
	if o.Slots <= 0 {
		o.Slots = defaultEtcdSlots
	}
	return o
}

// MemberSlot is one stable etcd member position. TargetMC is chosen by
// placement; EP-1 only models the slot.
//
//nolint:govet // fieldalignment: grouping beats padding for readability
type MemberSlot struct {
	Name       string
	Index      int
	TargetMC   string
	Namespace  string
	ClientURL  string
	PeerURL    string
	MemberName string
}

// Membership is the desired etcd member set for one cluster.
type Membership struct {
	ClusterName string
	Namespace   string
	Slots       []MemberSlot
}

// NewMembership assigns target MCs to slots in order. Placement decides the
// MC list; this function only builds the stable slot names and addresses.
func NewMembership(cluster string, targetMCs []string, opts EtcdOptions) Membership {
	opts = opts.withDefaults()
	namespace := opts.Namespace
	if namespace == "" {
		namespace = "etcd-" + cluster
	}

	membership := Membership{ClusterName: cluster, Namespace: namespace}
	for index := 0; index < opts.Slots; index++ {
		slot := MemberSlot{
			Name:       "m" + strconv.Itoa(index),
			Index:      index,
			Namespace:  namespace,
			MemberName: cluster + "-etcd-m" + strconv.Itoa(index),
		}
		if index < len(targetMCs) {
			slot.TargetMC = targetMCs[index]
		}
		slot.ClientURL = etcdURL(slot.MemberName, namespace, etcdClientPort)
		slot.PeerURL = etcdURL(slot.MemberName, namespace, etcdPeerPort)
		membership.Slots = append(membership.Slots, slot)
	}
	return membership
}

func etcdURL(name, namespace string, port int) string {
	return "https://" + name + "." + namespace + ".svc:" + strconv.Itoa(port)
}

// Endpoints returns every member's client URL.
func (m Membership) Endpoints() []string {
	endpoints := make([]string, 0, len(m.Slots))
	for _, slot := range m.Slots {
		endpoints = append(endpoints, slot.ClientURL)
	}
	return endpoints
}

// EtcdMemberStatus is one member's published health.
type EtcdMemberStatus struct {
	Name      string `json:"name"`
	MC        string `json:"mc"`
	ClientURL string `json:"clientURL"`
	PeerURL   string `json:"peerURL"`
	Healthy   bool   `json:"healthy"`
}

// EtcdStatusData is the status data contract consumed by downstream adapters.
type EtcdStatusData struct {
	Endpoints    []string           `json:"endpoints"`
	Members      []EtcdMemberStatus `json:"members"`
	Quorum       bool               `json:"quorum"`
	QuorumSize   int                `json:"quorumSize"`
	MembersReady int                `json:"membersReady"`
}

// Status builds the status contract from observed member health.
func (m Membership) Status(healthy map[string]bool) EtcdStatusData {
	status := EtcdStatusData{
		Endpoints:  m.Endpoints(),
		QuorumSize: len(m.Slots)/2 + 1,
	}
	for _, slot := range m.Slots {
		member := EtcdMemberStatus{
			Name:      slot.MemberName,
			MC:        slot.TargetMC,
			ClientURL: slot.ClientURL,
			PeerURL:   slot.PeerURL,
			Healthy:   healthy[slot.MemberName],
		}
		if member.Healthy {
			status.MembersReady++
		}
		status.Members = append(status.Members, member)
	}
	status.Quorum = status.MembersReady >= status.QuorumSize
	return status
}

// Data renders the contract as the map persisted by the StatusReporter.
func (s EtcdStatusData) Data() map[string]interface{} {
	return map[string]interface{}{
		"endpoints":    s.Endpoints,
		"members":      s.Members,
		"quorum":       s.Quorum,
		"quorumSize":   s.QuorumSize,
		"membersReady": s.MembersReady,
	}
}

// Report publishes the membership state as one reconcile result.
func (m Membership) Report(cluster ClusterView, healthy map[string]bool) StatusReport {
	status := m.Status(healthy)

	reason, message := "Quorum", "Etcd quorum is healthy"
	if !status.Quorum {
		reason, message = "NoQuorum", "Etcd quorum is not available"
	}
	return StatusReport{
		ObservedGeneration: cluster.Generation,
		Conditions: []api.AdapterCondition{
			condition(api.AdapterConditionTypeAvailable, statusValue(status.Quorum), reason, message),
			condition(api.AdapterConditionTypeApplied, api.AdapterConditionTrue, "Applied", "Etcd members applied"),
			condition(api.AdapterConditionTypeHealth, statusValue(status.Quorum), reason, message),
		},
		Data: status.Data(),
	}
}

func statusValue(healthy bool) api.AdapterConditionStatus {
	if healthy {
		return api.AdapterConditionTrue
	}
	return api.AdapterConditionFalse
}
