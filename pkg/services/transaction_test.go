package services

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	serviceerrors "github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/metrics"
)

func TestServiceErrorFromTransaction(t *testing.T) {
	wantServiceError := serviceerrors.ConflictState("resource is busy")
	tests := []struct {
		err  error
		want *serviceerrors.ServiceError
		name string
	}{
		{name: "nil", err: nil, want: nil},
		{
			name: "service error",
			err:  fmt.Errorf("transaction callback: %w", wantServiceError),
			want: wantServiceError,
		},
		{
			name: "connection failure",
			err:  fmt.Errorf("commit: %w", &net.OpError{Op: "dial", Net: "tcp", Err: io.EOF}),
			want: serviceerrors.ServiceUnavailable("Database connection unavailable"),
		},
		{name: "ordinary failure", err: errors.New("commit failed"), want: serviceerrors.GeneralError("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := serviceErrorFromTransaction(tt.err)
			if tt.want == nil {
				if got != nil {
					t.Fatalf("serviceErrorFromTransaction() = %#v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("serviceErrorFromTransaction() = nil")
			}
			if got != tt.want &&
				(got.HTTPCode != tt.want.HTTPCode ||
					got.RFC9457Code != tt.want.RFC9457Code ||
					got.Reason != tt.want.Reason) {
				t.Fatalf("serviceErrorFromTransaction() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

type contextCheckingResourceDAO struct {
	*mockResourceDao
	callbackContext context.Context
}

func (d *contextCheckingResourceDAO) Create(ctx context.Context, resource *api.Resource) (*api.Resource, error) {
	d.callbackContext = ctx
	return d.mockResourceDao.Create(ctx, resource)
}

func TestResourceServiceCreateUsesRunnerContextAndHandlesCommitFailure(t *testing.T) {
	setupTestDescriptors()
	type contextKey string
	runner := &controlledTxRunner{
		callbackContext:  context.WithValue(t.Context(), contextKey("transaction"), "callback"),
		afterCallbackErr: errors.New("commit failed"),
	}
	resourceDao := &contextCheckingResourceDAO{mockResourceDao: newMockResourceDao()}
	service, err := NewResourceService(
		resourceDao,
		newMockResourceLabelDao(),
		newMockAdapterStatusDao(),
		newResourceConditionMock(),
		&resourceGenericMock{},
		runner,
	)
	if err != nil {
		t.Fatalf("NewResourceService() error = %v", err)
	}

	resource := testResource("Channel", "ch-new", "new")
	result, serviceErr := service.Create(t.Context(), "Channel", resource, nil)
	if result != nil {
		t.Fatalf("Create() result = %v, want nil after commit failure", result)
	}
	if serviceErr == nil {
		t.Fatal("Create() returned nil service error after commit failure")
	}
	if runner.calls != 1 {
		t.Fatalf("runner calls = %d, want 1", runner.calls)
	}
	if got := resourceDao.callbackContext.Value(contextKey("transaction")); got != "callback" {
		t.Fatalf("DAO context value = %v, want callback", got)
	}
}

func TestResourceServiceCreateInvalidInputSkipsTransaction(t *testing.T) {
	setupTestDescriptors()
	runner := &controlledTxRunner{}
	service, err := NewResourceService(
		newMockResourceDao(),
		newMockResourceLabelDao(),
		newMockAdapterStatusDao(),
		newResourceConditionMock(),
		&resourceGenericMock{},
		runner,
	)
	if err != nil {
		t.Fatalf("NewResourceService() error = %v", err)
	}

	_, serviceErr := service.Create(t.Context(), "Unknown", &api.Resource{Name: "new"}, nil)
	if serviceErr == nil {
		t.Fatal("Create() returned nil error for an unknown kind")
	}
	if runner.calls != 0 {
		t.Fatalf("runner calls = %d, want 0", runner.calls)
	}
}

func TestResourceServiceEmitsReconciliationMetricOnlyAfterCommit(t *testing.T) {
	setupAdapterStatusDescriptors()
	metrics.ResetReconciliationMetrics()
	t.Cleanup(metrics.ResetReconciliationMetrics)

	failedService := newTransactionTestResourceService(&controlledTxRunner{
		afterCallbackErr: errors.New("commit failed"),
	})
	_, serviceErr := failedService.Create(
		t.Context(), "TestResource", testResource("TestResource", "failed", "failed"), nil,
	)
	if serviceErr == nil {
		t.Fatal("Create() returned nil service error after commit failure")
	}
	if got := reconciliationMetricCount(t, "TestResource", false); got != 0 {
		t.Fatalf("reconciliation metric count = %v, want 0 after commit failure", got)
	}

	successService := newTransactionTestResourceService(&controlledTxRunner{})
	_, serviceErr = successService.Create(
		t.Context(), "TestResource", testResource("TestResource", "success", "success"), nil,
	)
	if serviceErr != nil {
		t.Fatalf("Create() error = %v", serviceErr)
	}
	if got := reconciliationMetricCount(t, "TestResource", false); got != 1 {
		t.Fatalf("reconciliation metric count = %v, want 1 after commit", got)
	}
}

func TestDeleteResourceTreeCollectsCascadeReconciliationOutcomes(t *testing.T) {
	setupDescriptorsWithCascadeAndRequiredAdapters()
	resourceDao := newMockResourceDao()
	service := newTransactionTestResourceService(&controlledTxRunner{}).(*sqlResourceService)
	service.resourceDao = resourceDao

	parent := testResource("Workspace", "workspace", "workspace")
	child := testResourceWithOwner("Task", "task", "task", parent.ID)
	resourceDao.addResource(parent)
	resourceDao.addResource(child)

	now := parent.CreatedTime
	parent.MarkDeleted("test", now)
	parent.IncrementGeneration()
	outcomes, serviceErr := service.deleteResourceTree(t.Context(), parent, "test", now)
	if serviceErr != nil {
		t.Fatalf("deleteResourceTree() error = %v", serviceErr)
	}
	if len(outcomes) != 2 {
		t.Fatalf("outcome count = %d, want 2", len(outcomes))
	}
	if outcomes[0].kind != "Task" || outcomes[1].kind != "Workspace" {
		t.Fatalf("outcome kinds = %q, %q; want Task, Workspace", outcomes[0].kind, outcomes[1].kind)
	}
}

func newTransactionTestResourceService(runner *controlledTxRunner) ResourceService {
	service, err := NewResourceService(
		newMockResourceDao(),
		newMockResourceLabelDao(),
		newMockAdapterStatusDao(),
		newResourceConditionMock(),
		&resourceGenericMock{},
		runner,
	)
	if err != nil {
		panic("newTransactionTestResourceService: " + err.Error())
	}
	return service
}

func reconciliationMetricCount(t *testing.T, resourceType string, isDelete bool) float64 {
	t.Helper()
	families, err := prometheus.DefaultGatherer.Gather()
	if err != nil {
		t.Fatalf("gather reconciliation metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "hyperfleet_api_reconciliation_started_total" {
			continue
		}
		for _, metric := range family.Metric {
			if metricLabelValue(metric, "resource_type") == resourceType &&
				metricLabelValue(metric, "is_delete") == strconv.FormatBool(isDelete) {
				return metric.GetCounter().GetValue()
			}
		}
	}
	return 0
}

func metricLabelValue(metric *dto.Metric, name string) string {
	for _, label := range metric.Label {
		if label.GetName() == name {
			return label.GetValue()
		}
	}
	return ""
}
