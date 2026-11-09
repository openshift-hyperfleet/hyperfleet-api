package adapters

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	svcerrors "github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

func TestEtcdMembershipBuildsStableSlots(t *testing.T) {
	RegisterTestingT(t)

	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})

	Expect(membership.Namespace).To(Equal("etcd-my-cluster"))
	Expect(membership.Slots).To(HaveLen(3))
	Expect(membership.Slots[0]).To(Equal(MemberSlot{
		Name:       "m0",
		Index:      0,
		TargetMC:   "mc-a",
		Namespace:  "etcd-my-cluster",
		ClientURL:  "https://my-cluster-etcd-m0.etcd-my-cluster.svc:2379",
		PeerURL:    "https://my-cluster-etcd-m0.etcd-my-cluster.svc:2380",
		MemberName: "my-cluster-etcd-m0",
	}))
	Expect(membership.Endpoints()).To(Equal([]string{
		"https://my-cluster-etcd-m0.etcd-my-cluster.svc:2379",
		"https://my-cluster-etcd-m1.etcd-my-cluster.svc:2379",
		"https://my-cluster-etcd-m2.etcd-my-cluster.svc:2379",
	}))
}

func TestEtcdMembershipUsesConfiguredSlotsAndNamespace(t *testing.T) {
	RegisterTestingT(t)

	membership := NewMembership("c1", []string{"mc-a"}, EtcdOptions{Slots: 1, Namespace: "custom"})

	Expect(membership.Namespace).To(Equal("custom"))
	Expect(membership.Slots).To(HaveLen(1))
	Expect(membership.Slots[0].TargetMC).To(Equal("mc-a"))
}

func TestEtcdStatusTracksHealthAndQuorum(t *testing.T) {
	RegisterTestingT(t)
	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})

	status := membership.Status(map[string]bool{
		"my-cluster-etcd-m0": true,
		"my-cluster-etcd-m1": true,
	})

	Expect(status.QuorumSize).To(Equal(2))
	Expect(status.MembersReady).To(Equal(2))
	Expect(status.Quorum).To(BeTrue())
	Expect(status.Members).To(HaveLen(3))
	Expect(status.Members[2].Healthy).To(BeFalse())
}

func TestEtcdStatusReportsNoQuorum(t *testing.T) {
	RegisterTestingT(t)
	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})

	status := membership.Status(map[string]bool{"my-cluster-etcd-m0": true})

	Expect(status.Quorum).To(BeFalse())
	Expect(status.MembersReady).To(Equal(1))
}

func TestEtcdReportPublishesStatusThroughReporter(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)
	resourceSvc := services.NewMockResourceService(ctrl)

	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	reporter := &serviceReporter{resources: resourceSvc, now: func() time.Time { return now }}
	membership := NewMembership("my-cluster", []string{"mc-a", "mc-b", "mc-c"}, EtcdOptions{})
	cluster := ClusterView{ID: "cluster-1", Generation: 4}

	var stored *api.AdapterStatus
	resourceSvc.EXPECT().ProcessAdapterStatus(
		gomock.Any(), api.ResourceTypeCluster, cluster.ID, gomock.Any(),
	).DoAndReturn(func(
		_ context.Context, _, _ string, status *api.AdapterStatus,
	) (*api.AdapterStatus, *svcerrors.ServiceError) {
		stored = status
		return status, nil
	})

	report := membership.Report(cluster, map[string]bool{
		"my-cluster-etcd-m0": true,
		"my-cluster-etcd-m1": true,
	})
	Expect(reporter.Report(context.Background(), cluster, EtcdAdapterName, report)).To(Succeed())

	Expect(stored.Adapter).To(Equal(EtcdAdapterName))
	var data EtcdStatusData
	Expect(json.Unmarshal(stored.Data, &data)).To(Succeed())
	Expect(data.Quorum).To(BeTrue())
	Expect(data.Endpoints).To(HaveLen(3))
	Expect(data.Members).To(HaveLen(3))
}
