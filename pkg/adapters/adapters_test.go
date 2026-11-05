package adapters

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

func TestServiceReporterReportsThroughProcessAdapterStatus(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)
	resourceSvc := services.NewMockResourceService(ctrl)

	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	reporter := &serviceReporter{resources: resourceSvc, now: func() time.Time { return now }}
	cluster := ClusterView{ID: "cluster-1", Generation: 3}

	report, err := NoopController{}.Reconcile(context.Background(), cluster)
	Expect(err).ToNot(HaveOccurred())

	var stored *api.AdapterStatus
	resourceSvc.EXPECT().ProcessAdapterStatus(
		gomock.Any(), api.ResourceTypeCluster, cluster.ID, gomock.Any(),
	).DoAndReturn(func(
		_ context.Context, _, _ string, status *api.AdapterStatus,
	) (*api.AdapterStatus, *errors.ServiceError) {
		stored = status
		return status, nil
	})

	Expect(reporter.Report(context.Background(), cluster, NoopAdapterName, report)).To(Succeed())

	Expect(stored.Adapter).To(Equal(NoopAdapterName))
	Expect(stored.ObservedGeneration).To(Equal(cluster.Generation))
	Expect(stored.CreatedTime).To(Equal(now))
	Expect(stored.LastReportTime).To(Equal(now))
	Expect(string(stored.Data)).To(Equal("{}"))

	var conditions []api.AdapterCondition
	Expect(json.Unmarshal(stored.Conditions, &conditions)).To(Succeed())
	Expect(conditions).To(HaveLen(4))
	for _, condition := range conditions {
		Expect(condition.Status).To(Equal(api.AdapterConditionTrue))
		Expect(condition.LastTransitionTime).To(Equal(now))
	}
}

func TestServiceReporterPreservesConditionTransitionTime(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)
	resourceSvc := services.NewMockResourceService(ctrl)

	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	previous := now.Add(-time.Hour)
	reporter := &serviceReporter{resources: resourceSvc, now: func() time.Time { return now }}

	var stored *api.AdapterStatus
	resourceSvc.EXPECT().ProcessAdapterStatus(
		gomock.Any(), api.ResourceTypeCluster, "cluster-1", gomock.Any(),
	).DoAndReturn(func(
		_ context.Context, _, _ string, status *api.AdapterStatus,
	) (*api.AdapterStatus, *errors.ServiceError) {
		stored = status
		return status, nil
	})

	err := reporter.Report(context.Background(), ClusterView{ID: "cluster-1"}, NoopAdapterName, StatusReport{
		Conditions: []api.AdapterCondition{{
			Type:               api.AdapterConditionTypeAvailable,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: previous,
		}},
	})
	Expect(err).ToNot(HaveOccurred())

	var conditions []api.AdapterCondition
	Expect(json.Unmarshal(stored.Conditions, &conditions)).To(Succeed())
	Expect(conditions[0].LastTransitionTime).To(Equal(previous))
}

func TestServiceReporterReturnsServiceError(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)
	resourceSvc := services.NewMockResourceService(ctrl)

	svcErr := errors.GeneralError("status unavailable")
	resourceSvc.EXPECT().ProcessAdapterStatus(
		gomock.Any(), api.ResourceTypeCluster, "cluster-1", gomock.Any(),
	).Return(nil, svcErr)

	reporter := &serviceReporter{resources: resourceSvc, now: time.Now}
	err := reporter.Report(context.Background(), ClusterView{ID: "cluster-1"}, NoopAdapterName, StatusReport{})
	Expect(err).To(Equal(svcErr))
}

func TestNoopControllerReportsHealthy(t *testing.T) {
	RegisterTestingT(t)
	cluster := ClusterView{Generation: 7}

	report, err := NoopController{}.Reconcile(context.Background(), cluster)

	Expect(err).ToNot(HaveOccurred())
	Expect(NoopController{}.Name()).To(Equal(NoopAdapterName))
	Expect(report.ObservedGeneration).To(Equal(cluster.Generation))
	Expect(report.Data).ToNot(BeNil())

	conditions := make(map[string]api.AdapterCondition, len(report.Conditions))
	for _, condition := range report.Conditions {
		conditions[condition.Type] = condition
	}
	for _, conditionType := range []string{
		api.AdapterConditionTypeAvailable,
		api.AdapterConditionTypeApplied,
		api.AdapterConditionTypeHealth,
		api.AdapterConditionTypeFinalized,
	} {
		Expect(conditions).To(HaveKey(conditionType))
		Expect(conditions[conditionType].Status).To(Equal(api.AdapterConditionTrue))
	}
}
