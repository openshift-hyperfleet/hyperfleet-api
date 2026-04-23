package services

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

const (
	testClusterID = "test-cluster-id"
	systemActor   = "system@hyperfleet.local"
)

// testAdapterConfig creates a test adapter config with default values
func testAdapterConfig() *config.AdapterRequirementsConfig {
	return &config.AdapterRequirementsConfig{
		Required: config.RequiredAdapters{
			Cluster:  []string{"validation", "dns", "pullsecret", "hypershift"},
			Nodepool: []string{"validation", "hypershift"},
		},
	}
}

// Mock implementations for testing ProcessAdapterStatus

type mockClusterDao struct {
	clusters map[string]*api.Cluster
}

func newMockClusterDao() *mockClusterDao {
	return &mockClusterDao{
		clusters: make(map[string]*api.Cluster),
	}
}

func (d *mockClusterDao) Get(ctx context.Context, id string) (*api.Cluster, error) {
	if c, ok := d.clusters[id]; ok {
		return c, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *mockClusterDao) Create(ctx context.Context, cluster *api.Cluster) (*api.Cluster, error) {
	if cluster.CreatedTime.IsZero() {
		now := time.Now()
		cluster.CreatedTime = now
	}
	if cluster.UpdatedTime.IsZero() {
		cluster.UpdatedTime = cluster.CreatedTime
	}
	d.clusters[cluster.ID] = cluster
	return cluster, nil
}

func (d *mockClusterDao) Replace(ctx context.Context, cluster *api.Cluster) (*api.Cluster, error) {
	d.clusters[cluster.ID] = cluster
	return cluster, nil
}

func (d *mockClusterDao) Save(ctx context.Context, cluster *api.Cluster) error {
	d.clusters[cluster.ID] = cluster
	return nil
}

func (d *mockClusterDao) Delete(ctx context.Context, id string) error {
	delete(d.clusters, id)
	return nil
}

func (d *mockClusterDao) FindByIDs(ctx context.Context, ids []string) (api.ClusterList, error) {
	var result api.ClusterList
	for _, id := range ids {
		if c, ok := d.clusters[id]; ok {
			result = append(result, c)
		}
	}
	return result, nil
}

func (d *mockClusterDao) All(ctx context.Context) (api.ClusterList, error) {
	var result api.ClusterList
	for _, c := range d.clusters {
		result = append(result, c)
	}
	return result, nil
}

var _ dao.ClusterDao = &mockClusterDao{}

type mockAdapterStatusDao struct {
	statuses map[string]*api.AdapterStatus
}

func newMockAdapterStatusDao() *mockAdapterStatusDao {
	return &mockAdapterStatusDao{
		statuses: make(map[string]*api.AdapterStatus),
	}
}

func (d *mockAdapterStatusDao) Get(ctx context.Context, id string) (*api.AdapterStatus, error) {
	if s, ok := d.statuses[id]; ok {
		return s, nil
	}
	return nil, errors.NotFound("AdapterStatus").AsError()
}

func (d *mockAdapterStatusDao) Create(ctx context.Context, status *api.AdapterStatus) (*api.AdapterStatus, error) {
	d.statuses[status.ID] = status
	return status, nil
}

func (d *mockAdapterStatusDao) Replace(ctx context.Context, status *api.AdapterStatus) (*api.AdapterStatus, error) {
	d.statuses[status.ID] = status
	return status, nil
}

func (d *mockAdapterStatusDao) Upsert(ctx context.Context, status *api.AdapterStatus) (*api.AdapterStatus, error) {
	key := status.ResourceType + ":" + status.ResourceID + ":" + status.Adapter
	if existing, ok := d.statuses[key]; ok {
		isStoredFresherOrEqual := existing.ObservedGeneration > status.ObservedGeneration ||
			(existing.ObservedGeneration == status.ObservedGeneration &&
				!existing.LastReportTime.Before(status.LastReportTime))
		if isStoredFresherOrEqual {
			return existing, nil
		}

		status.ID = existing.ID
		if !existing.CreatedTime.IsZero() {
			status.CreatedTime = existing.CreatedTime
		}
	} else {
		status.ID = key
	}

	d.statuses[key] = status
	return status, nil
}

func (d *mockAdapterStatusDao) Delete(ctx context.Context, id string) error {
	delete(d.statuses, id)
	return nil
}

func (d *mockAdapterStatusDao) FindByResource(
	ctx context.Context,
	resourceType, resourceID string,
) (api.AdapterStatusList, error) {
	var result api.AdapterStatusList
	for _, s := range d.statuses {
		if s.ResourceType == resourceType && s.ResourceID == resourceID {
			result = append(result, s)
		}
	}
	return result, nil
}

func (d *mockAdapterStatusDao) FindByResourceIDs(
	ctx context.Context,
	resourceType string,
	resourceIDs []string,
) (api.AdapterStatusList, error) {
	var result api.AdapterStatusList
	resourceIDSet := make(map[string]bool, len(resourceIDs))
	for _, id := range resourceIDs {
		resourceIDSet[id] = true
	}
	for _, s := range d.statuses {
		if s.ResourceType == resourceType && resourceIDSet[s.ResourceID] {
			result = append(result, s)
		}
	}
	return result, nil
}

func (d *mockAdapterStatusDao) FindByResourcePaginated(
	ctx context.Context,
	resourceType, resourceID string,
	offset, limit int,
) (api.AdapterStatusList, int64, error) {
	statuses, _ := d.FindByResource(ctx, resourceType, resourceID)
	return statuses, int64(len(statuses)), nil
}

func (d *mockAdapterStatusDao) FindByResourceAndAdapter(
	ctx context.Context,
	resourceType, resourceID, adapter string,
) (*api.AdapterStatus, error) {
	for _, s := range d.statuses {
		if s.ResourceType == resourceType && s.ResourceID == resourceID && s.Adapter == adapter {
			return s, nil
		}
	}
	return nil, errors.NotFound("AdapterStatus").AsError()
}

func (d *mockAdapterStatusDao) All(ctx context.Context) (api.AdapterStatusList, error) {
	var result api.AdapterStatusList
	for _, s := range d.statuses {
		result = append(result, s)
	}
	return result, nil
}

var _ dao.AdapterStatusDao = &mockAdapterStatusDao{}

// TestProcessAdapterStatus_FirstUnknownCondition tests that the first Unknown Available condition is stored
func TestProcessAdapterStatus_FirstUnknownCondition(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()
	config := testAdapterConfig()
	service := NewClusterService(clusterDao, newMockNodePoolDao(), adapterStatusDao, config)

	ctx := context.Background()
	clusterID := testClusterID

	// Create cluster first
	cluster := &api.Cluster{Generation: 1}
	cluster.ID = clusterID
	_, svcErr := service.Create(ctx, cluster)
	g.Expect(svcErr).To(BeNil())

	// Send first status with Available=Unknown
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	conditionsJSON, _ := json.Marshal(conditions)
	now := time.Now()
	adapterStatus := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         clusterID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}

	result, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)

	// First report with Unknown should be accepted
	g.Expect(err).To(BeNil())
	g.Expect(result).ToNot(BeNil(), "First report with Available=Unknown should be stored")
	g.Expect(result.Adapter).To(Equal("test-adapter"))

	// Verify the status was stored
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, "Cluster", clusterID)
	g.Expect(len(storedStatuses)).To(Equal(1), "First Unknown status should be stored")
}

// TestProcessAdapterStatus_SubsequentUnknownCondition tests that subsequent Unknown Available conditions are discarded
func TestProcessAdapterStatus_SubsequentUnknownCondition(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := testAdapterConfig()
	service := NewClusterService(clusterDao, newMockNodePoolDao(), adapterStatusDao, config)

	ctx := context.Background()
	clusterID := testClusterID

	now := time.Now()
	clusterDao.clusters[clusterID] = &api.Cluster{
		Meta:       api.Meta{ID: clusterID, CreatedTime: now, UpdatedTime: now},
		Generation: 1,
	}

	// Pre-populate an existing adapter status to simulate a previously stored report
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	existingStatus := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         clusterID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}
	_, _ = adapterStatusDao.Upsert(ctx, existingStatus)

	// Now send another Unknown status report
	newAdapterStatus := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         clusterID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}

	result, err := service.ProcessAdapterStatus(ctx, clusterID, newAdapterStatus)

	g.Expect(err).To(BeNil())
	g.Expect(result).To(BeNil(), "Subsequent Unknown status should be discarded")
}

// TestProcessAdapterStatus_InvalidStatusReturnsValidationError tests that a non-True/False/Unknown
// Available status is rejected with a validation error.
func TestProcessAdapterStatus_InvalidStatusReturnsValidationError(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()
	config := testAdapterConfig()
	service := NewClusterService(clusterDao, newMockNodePoolDao(), adapterStatusDao, config)

	ctx := context.Background()
	clusterID := testClusterID

	cluster := &api.Cluster{Generation: 1}
	cluster.ID = clusterID
	_, svcErr := service.Create(ctx, cluster)
	g.Expect(svcErr).To(BeNil())

	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: "Pending", LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	conditionsJSON, _ := json.Marshal(conditions)
	now := time.Now()
	adapterStatus := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         clusterID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}

	result, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)

	g.Expect(err).ToNot(BeNil(), "Invalid status should return a validation error")
	g.Expect(err.HTTPCode).To(Equal(http.StatusBadRequest))
	g.Expect(result).To(BeNil())
}

// TestProcessAdapterStatus_EmptyStatusReturnsValidationError tests that an empty Available status
// is rejected with a validation error.
func TestProcessAdapterStatus_EmptyStatusReturnsValidationError(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()
	config := testAdapterConfig()
	service := NewClusterService(clusterDao, newMockNodePoolDao(), adapterStatusDao, config)

	ctx := context.Background()
	clusterID := testClusterID

	cluster := &api.Cluster{Generation: 1}
	cluster.ID = clusterID
	_, svcErr := service.Create(ctx, cluster)
	g.Expect(svcErr).To(BeNil())

	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: "", LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	conditionsJSON, _ := json.Marshal(conditions)
	now := time.Now()
	adapterStatus := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         clusterID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}

	result, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)

	g.Expect(err).ToNot(BeNil(), "Empty status should return a validation error")
	g.Expect(err.HTTPCode).To(Equal(http.StatusBadRequest))
	g.Expect(result).To(BeNil())
}

// TestProcessAdapterStatus_TrueCondition tests that True Available condition upserts and aggregates
func TestProcessAdapterStatus_TrueCondition(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := testAdapterConfig()
	service := NewClusterService(clusterDao, newMockNodePoolDao(), adapterStatusDao, config)

	ctx := context.Background()
	clusterID := testClusterID

	// Create the cluster first
	cluster := &api.Cluster{
		Generation: 1,
	}
	cluster.ID = clusterID
	_, svcErr := service.Create(ctx, cluster)
	g.Expect(svcErr).To(BeNil())

	// Create adapter status with all mandatory conditions
	conditions := []api.AdapterCondition{
		{
			Type:               api.ConditionTypeAvailable,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               api.ConditionTypeApplied,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               api.ConditionTypeHealth,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	now := time.Now()
	adapterStatus := &api.AdapterStatus{
		ResourceType:   "Cluster",
		ResourceID:     clusterID,
		Adapter:        "test-adapter",
		Conditions:     conditionsJSON,
		CreatedTime:    now,
		LastReportTime: now,
	}

	result, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)

	g.Expect(err).To(BeNil())
	g.Expect(result).ToNot(BeNil(), "ProcessAdapterStatus should return the upserted status")
	g.Expect(result.Adapter).To(Equal("test-adapter"))

	// Verify the status was stored
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, "Cluster", clusterID)
	g.Expect(len(storedStatuses)).To(Equal(1), "Status should be stored for True condition")
}

// TestProcessAdapterStatus_FalseCondition tests that False Available condition upserts and aggregates
func TestProcessAdapterStatus_FalseCondition(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := testAdapterConfig()
	service := NewClusterService(clusterDao, newMockNodePoolDao(), adapterStatusDao, config)

	ctx := context.Background()
	clusterID := testClusterID

	// Create the cluster first
	cluster := &api.Cluster{
		Generation: 1,
	}
	cluster.ID = clusterID
	_, svcErr := service.Create(ctx, cluster)
	g.Expect(svcErr).To(BeNil())

	// Create adapter status with all mandatory conditions
	conditions := []api.AdapterCondition{
		{
			Type:               api.ConditionTypeAvailable,
			Status:             api.AdapterConditionFalse,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               api.ConditionTypeApplied,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               api.ConditionTypeHealth,
			Status:             api.AdapterConditionFalse,
			LastTransitionTime: time.Now(),
		},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	now := time.Now()
	adapterStatus := &api.AdapterStatus{
		ResourceType:   "Cluster",
		ResourceID:     clusterID,
		Adapter:        "test-adapter",
		Conditions:     conditionsJSON,
		CreatedTime:    now,
		LastReportTime: now,
	}

	result, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)

	g.Expect(err).To(BeNil())
	g.Expect(result).ToNot(BeNil(), "ProcessAdapterStatus should return the upserted status")

	// Verify the status was stored
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, "Cluster", clusterID)
	g.Expect(len(storedStatuses)).To(Equal(1), "Status should be stored for False condition")
}

// TestProcessAdapterStatus_FirstMultipleConditions_AvailableUnknown tests that first reports with
// Available=Unknown are accepted even when other conditions are present
func TestProcessAdapterStatus_FirstMultipleConditions_AvailableUnknown(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := testAdapterConfig()
	service := NewClusterService(clusterDao, newMockNodePoolDao(), adapterStatusDao, config)

	ctx := context.Background()
	clusterID := testClusterID

	// Create cluster first
	cluster := &api.Cluster{Generation: 1}
	cluster.ID = clusterID
	_, svcErr := service.Create(ctx, cluster)
	g.Expect(svcErr).To(BeNil())

	// Create first adapter status with all mandatory conditions but Available=Unknown
	conditions := []api.AdapterCondition{
		{
			Type:               api.ConditionTypeAvailable,
			Status:             api.AdapterConditionUnknown,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               api.ConditionTypeApplied,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               api.ConditionTypeHealth,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               api.ConditionTypeReady,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	now := time.Now()
	adapterStatus := &api.AdapterStatus{
		ResourceType:   "Cluster",
		ResourceID:     clusterID,
		Adapter:        "test-adapter",
		Conditions:     conditionsJSON,
		CreatedTime:    now,
		LastReportTime: now,
	}

	result, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)

	g.Expect(err).To(BeNil())
	g.Expect(result).ToNot(BeNil(), "First report with Available=Unknown should be accepted")

	// Verify the status was stored
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, "Cluster", clusterID)
	g.Expect(len(storedStatuses)).To(Equal(1), "First status with Available=Unknown should be stored")
}

// TestProcessAdapterStatus_SubsequentMultipleConditions_AvailableUnknown tests that subsequent reports
// with multiple conditions including Available=Unknown are discarded
func TestProcessAdapterStatus_SubsequentMultipleConditions_AvailableUnknown(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := testAdapterConfig()
	service := NewClusterService(clusterDao, newMockNodePoolDao(), adapterStatusDao, config)

	ctx := context.Background()
	clusterID := testClusterID

	now := time.Now()
	clusterDao.clusters[clusterID] = &api.Cluster{
		Meta:       api.Meta{ID: clusterID, CreatedTime: now, UpdatedTime: now},
		Generation: 1,
	}

	// Pre-populate an existing adapter status
	existingConditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	existingConditionsJSON, _ := json.Marshal(existingConditions)

	existingStatus := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         clusterID,
		Adapter:            "test-adapter",
		Conditions:         existingConditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}
	_, _ = adapterStatusDao.Upsert(ctx, existingStatus)

	// Now send another report with multiple conditions including Available=Unknown
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeReady, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: "Progressing", Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	adapterStatus := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         clusterID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}

	result, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)

	g.Expect(err).To(BeNil())
	g.Expect(result).To(BeNil(), "Subsequent Available=Unknown should be discarded")
}

func TestClusterAvailableReadyTransitions(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()

	adapterConfig := testAdapterConfig()
	// Keep this small so we can cover transitions succinctly.
	adapterConfig.Required.Cluster = []string{"validation", "dns"}

	service := NewClusterService(clusterDao, newMockNodePoolDao(), adapterStatusDao, adapterConfig)

	ctx := context.Background()
	clusterID := testClusterID

	cluster := &api.Cluster{Generation: 1}
	cluster.ID = clusterID
	_, svcErr := service.Create(ctx, cluster)
	g.Expect(svcErr).To(BeNil())

	getSynth := func() (api.ResourceCondition, api.ResourceCondition) {
		stored, getErr := clusterDao.Get(ctx, clusterID)
		g.Expect(getErr).To(BeNil())

		var conds []api.ResourceCondition
		g.Expect(json.Unmarshal(stored.StatusConditions, &conds)).To(Succeed())
		g.Expect(len(conds)).To(BeNumerically(">=", 2))

		var available, ready *api.ResourceCondition
		for i := range conds {
			switch conds[i].Type {
			case api.ConditionTypeAvailable:
				available = &conds[i]
			case api.ConditionTypeReady:
				ready = &conds[i]
			}
		}
		g.Expect(available).ToNot(BeNil())
		g.Expect(ready).ToNot(BeNil())
		return *available, *ready
	}

	upsert := func(adapter string, available api.AdapterConditionStatus, observedGen int32) {
		conditions := []api.AdapterCondition{
			{Type: api.ConditionTypeAvailable, Status: available, LastTransitionTime: time.Now()},
			{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
			{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		}
		conditionsJSON, _ := json.Marshal(conditions)
		now := time.Now()

		adapterStatus := &api.AdapterStatus{
			ResourceType:       "Cluster",
			ResourceID:         clusterID,
			Adapter:            adapter,
			ObservedGeneration: observedGen,
			Conditions:         conditionsJSON,
			CreatedTime:        now,
			LastReportTime:     now,
		}

		_, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)
		g.Expect(err).To(BeNil())
	}

	// No adapter statuses yet.
	_, err := service.UpdateClusterStatusFromAdapters(ctx, clusterID)
	g.Expect(err).To(BeNil())
	avail, ready := getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionFalse))
	g.Expect(avail.ObservedGeneration).To(Equal(int32(1)))
	g.Expect(ready.Status).To(Equal(api.ConditionFalse))
	g.Expect(ready.ObservedGeneration).To(Equal(int32(1)))

	// Partial adapters: still not Available/Ready.
	upsert("validation", api.AdapterConditionTrue, 1)
	avail, ready = getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionFalse))
	g.Expect(ready.Status).To(Equal(api.ConditionFalse))

	// All required adapters available at gen=1 => Available=True, Ready=True.
	upsert("dns", api.AdapterConditionTrue, 1)
	avail, ready = getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionTrue))
	g.Expect(avail.ObservedGeneration).To(Equal(int32(1)))
	g.Expect(ready.Status).To(Equal(api.ConditionTrue))
	g.Expect(ready.ObservedGeneration).To(Equal(int32(1)))

	// Bump resource generation => Ready flips to False; Available remains True.
	clusterDao.clusters[clusterID].Generation = 2
	_, err = service.UpdateClusterStatusFromAdapters(ctx, clusterID)
	g.Expect(err).To(BeNil())
	avail, ready = getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionTrue))
	g.Expect(avail.ObservedGeneration).To(Equal(int32(1)))
	g.Expect(ready.Status).To(Equal(api.ConditionFalse))
	g.Expect(ready.ObservedGeneration).To(Equal(int32(2)))

	// One adapter updates to gen=2 => Ready still False; Available still True (minObservedGeneration still 1).
	upsert("validation", api.AdapterConditionTrue, 2)
	avail, ready = getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionTrue))
	g.Expect(avail.ObservedGeneration).To(Equal(int32(1)))
	g.Expect(ready.Status).To(Equal(api.ConditionFalse))

	// One adapter updates to gen=1 => Ready still False; Available still True (minObservedGeneration still 1).
	// This is an edge case where an adapter reports a gen=1 status after a gen=2 status.
	// Since we don't allow downgrading observed generations, we should not overwrite the cluster conditions.
	// And Available should remain True, but in reality it should be False.
	// This should be an unexpected edge case, since once a resource changes generation,
	// all adapters should report a gen=2 status.
	// So, while we are keeping Available True for gen=1,
	// there should be soon an update to gen=2, which will overwrite the Available condition.
	upsert("validation", api.AdapterConditionFalse, 1)
	avail, ready = getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionTrue)) // <-- this is the edge case
	g.Expect(avail.ObservedGeneration).To(Equal(int32(1)))
	g.Expect(ready.Status).To(Equal(api.ConditionFalse))

	// All required adapters at gen=2 => Ready becomes True, Available minObservedGeneration becomes 2.
	upsert("dns", api.AdapterConditionTrue, 2)
	avail, ready = getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionTrue))
	g.Expect(avail.ObservedGeneration).To(Equal(int32(2)))
	g.Expect(ready.Status).To(Equal(api.ConditionTrue))

	// One required adapter goes False => both Available and Ready become False.
	upsert("dns", api.AdapterConditionFalse, 2)
	avail, ready = getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionFalse))
	g.Expect(avail.ObservedGeneration).To(Equal(int32(2)))
	g.Expect(ready.Status).To(Equal(api.ConditionFalse))

	// Available=Unknown is a no-op (does not store, does not overwrite cluster conditions).
	prevStatus := api.Cluster{}.StatusConditions
	prevStatus = append(prevStatus, clusterDao.clusters[clusterID].StatusConditions...)
	unknownConds := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	unknownJSON, _ := json.Marshal(unknownConds)
	unknownStatus := &api.AdapterStatus{
		ResourceType: "Cluster",
		ResourceID:   clusterID,
		Adapter:      "dns",
		Conditions:   unknownJSON,
	}
	result, svcErr := service.ProcessAdapterStatus(ctx, clusterID, unknownStatus)
	g.Expect(svcErr).To(BeNil())
	g.Expect(result).To(BeNil())
	g.Expect(clusterDao.clusters[clusterID].StatusConditions).To(Equal(prevStatus))
}

func TestClusterStaleAdapterStatusUpdatePolicy(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()

	adapterConfig := testAdapterConfig()
	adapterConfig.Required.Cluster = []string{"validation", "dns"}

	service := NewClusterService(clusterDao, newMockNodePoolDao(), adapterStatusDao, adapterConfig)

	ctx := context.Background()
	clusterID := testClusterID

	cluster := &api.Cluster{Generation: 2}
	cluster.ID = clusterID
	_, svcErr := service.Create(ctx, cluster)
	g.Expect(svcErr).To(BeNil())

	getAvailable := func() api.ResourceCondition {
		stored, getErr := clusterDao.Get(ctx, clusterID)
		g.Expect(getErr).To(BeNil())

		var conds []api.ResourceCondition
		g.Expect(json.Unmarshal(stored.StatusConditions, &conds)).To(Succeed())
		for i := range conds {
			if conds[i].Type == api.ConditionTypeAvailable {
				return conds[i]
			}
		}
		g.Expect(true).To(BeFalse(), "Available condition not found")
		return api.ResourceCondition{}
	}

	upsert := func(adapter string, available api.AdapterConditionStatus, observedGen int32) {
		conditions := []api.AdapterCondition{
			{Type: api.ConditionTypeAvailable, Status: available, LastTransitionTime: time.Now()},
			{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
			{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		}
		conditionsJSON, _ := json.Marshal(conditions)
		now := time.Now()

		adapterStatus := &api.AdapterStatus{
			ResourceType:       "Cluster",
			ResourceID:         clusterID,
			Adapter:            adapter,
			ObservedGeneration: observedGen,
			Conditions:         conditionsJSON,
			CreatedTime:        now,
			LastReportTime:     now,
		}

		_, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)
		g.Expect(err).To(BeNil())
	}

	// Current generation statuses => Available=True at observed_generation=2.
	upsert("validation", api.AdapterConditionTrue, 2)
	upsert("dns", api.AdapterConditionTrue, 2)
	available := getAvailable()
	g.Expect(available.Status).To(Equal(api.ConditionTrue))
	g.Expect(available.ObservedGeneration).To(Equal(int32(2)))

	// Stale True should not override newer True.
	upsert("validation", api.AdapterConditionTrue, 1)
	available = getAvailable()
	g.Expect(available.Status).To(Equal(api.ConditionTrue))
	g.Expect(available.ObservedGeneration).To(Equal(int32(2)))

	// Stale False is more restrictive and should override.
	upsert("validation", api.AdapterConditionFalse, 1)
	available = getAvailable()
	g.Expect(available.Status).To(Equal(api.ConditionTrue))
	g.Expect(available.ObservedGeneration).To(Equal(int32(2)))
}

func TestClusterSyntheticTimestampsStableWithoutAdapterStatus(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()

	adapterConfig := testAdapterConfig()
	adapterConfig.Required.Cluster = []string{"validation"}

	service := NewClusterService(clusterDao, newMockNodePoolDao(), adapterStatusDao, adapterConfig)

	ctx := context.Background()
	clusterID := testClusterID

	fixedNow := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	initialConditions := []api.ResourceCondition{
		{
			Type:               api.ConditionTypeAvailable,
			Status:             api.ConditionFalse,
			ObservedGeneration: 1,
			LastTransitionTime: fixedNow,
			CreatedTime:        fixedNow,
			LastUpdatedTime:    fixedNow,
		},
		{
			Type:               api.ConditionTypeReady,
			Status:             api.ConditionFalse,
			ObservedGeneration: 1,
			LastTransitionTime: fixedNow,
			CreatedTime:        fixedNow,
			LastUpdatedTime:    fixedNow,
		},
	}
	initialConditionsJSON, _ := json.Marshal(initialConditions)

	cluster := &api.Cluster{
		Generation:       1,
		StatusConditions: initialConditionsJSON,
	}
	cluster.ID = clusterID
	cluster.CreatedTime = fixedNow
	cluster.UpdatedTime = fixedNow
	created, svcErr := service.Create(ctx, cluster)
	g.Expect(svcErr).To(BeNil())

	var createdConds []api.ResourceCondition
	g.Expect(json.Unmarshal(created.StatusConditions, &createdConds)).To(Succeed())
	g.Expect(len(createdConds)).To(BeNumerically(">=", 2))

	var createdAvailable, createdReady *api.ResourceCondition
	for i := range createdConds {
		switch createdConds[i].Type {
		case api.ConditionTypeAvailable:
			createdAvailable = &createdConds[i]
		case api.ConditionTypeReady:
			createdReady = &createdConds[i]
		}
	}
	g.Expect(createdAvailable).ToNot(BeNil())
	g.Expect(createdReady).ToNot(BeNil())
	g.Expect(createdAvailable.CreatedTime).To(Equal(fixedNow))
	g.Expect(createdAvailable.LastTransitionTime).To(Equal(fixedNow))
	g.Expect(createdAvailable.LastUpdatedTime).To(Equal(fixedNow))
	g.Expect(createdReady.CreatedTime).To(Equal(fixedNow))
	g.Expect(createdReady.LastTransitionTime).To(Equal(fixedNow))
	g.Expect(createdReady.LastUpdatedTime).To(Equal(fixedNow))

	updated, err := service.UpdateClusterStatusFromAdapters(ctx, clusterID)
	g.Expect(err).To(BeNil())

	var updatedConds []api.ResourceCondition
	g.Expect(json.Unmarshal(updated.StatusConditions, &updatedConds)).To(Succeed())
	g.Expect(len(updatedConds)).To(BeNumerically(">=", 2))

	var updatedAvailable, updatedReady *api.ResourceCondition
	for i := range updatedConds {
		switch updatedConds[i].Type {
		case api.ConditionTypeAvailable:
			updatedAvailable = &updatedConds[i]
		case api.ConditionTypeReady:
			updatedReady = &updatedConds[i]
		}
	}
	g.Expect(updatedAvailable).ToNot(BeNil())
	g.Expect(updatedReady).ToNot(BeNil())
	g.Expect(updatedAvailable.CreatedTime).To(Equal(fixedNow))
	g.Expect(updatedAvailable.LastTransitionTime).To(Equal(fixedNow))
	g.Expect(updatedAvailable.LastUpdatedTime).To(Equal(fixedNow))
	g.Expect(updatedReady.CreatedTime).To(Equal(fixedNow))
	g.Expect(updatedReady.LastTransitionTime).To(Equal(fixedNow))
	g.Expect(updatedReady.LastUpdatedTime).To(Equal(fixedNow))
}

// TestProcessAdapterStatus_MissingMandatoryCondition_Available tests that updates missing Available are rejected
func TestProcessAdapterStatus_MissingMandatoryCondition_Available(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()
	config := testAdapterConfig()
	service := NewClusterService(clusterDao, newMockNodePoolDao(), adapterStatusDao, config)

	ctx := context.Background()
	clusterID := testClusterID

	// Create cluster first
	cluster := &api.Cluster{Generation: 1}
	cluster.ID = clusterID
	_, svcErr := service.Create(ctx, cluster)
	g.Expect(svcErr).To(BeNil())

	// First, send a valid status
	validConditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	validConditionsJSON, _ := json.Marshal(validConditions)
	now := time.Now()
	validStatus := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         clusterID,
		Adapter:            "test-adapter",
		Conditions:         validConditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}
	result, err := service.ProcessAdapterStatus(ctx, clusterID, validStatus)
	g.Expect(err).To(BeNil())
	g.Expect(result).ToNot(BeNil())

	// Now send an update without Available condition
	invalidConditions := []api.AdapterCondition{
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: "CustomCondition", Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	invalidConditionsJSON, _ := json.Marshal(invalidConditions)
	invalidStatus := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         clusterID,
		Adapter:            "test-adapter",
		Conditions:         invalidConditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}

	result, err = service.ProcessAdapterStatus(ctx, clusterID, invalidStatus)

	// Should be rejected with validation error
	g.Expect(err).ToNot(BeNil())
	g.Expect(err.HTTPCode).To(Equal(http.StatusBadRequest))
	g.Expect(err.Reason).To(ContainSubstring("missing mandatory condition"))
	g.Expect(result).To(BeNil(), "Update missing Available condition should be rejected")

	// Verify old status is preserved
	storedStatus, _ := adapterStatusDao.FindByResourceAndAdapter(ctx, "Cluster", clusterID, "test-adapter")
	g.Expect(storedStatus).ToNot(BeNil())
	g.Expect(storedStatus.ObservedGeneration).To(Equal(int32(1)), "Old status should be preserved")

	var storedConditions []api.AdapterCondition
	unmarshalErr := json.Unmarshal(storedStatus.Conditions, &storedConditions)
	g.Expect(unmarshalErr).To(BeNil())
	g.Expect(len(storedConditions)).To(Equal(3))
	// Verify Available is still there
	hasAvailable := false
	for _, cond := range storedConditions {
		if cond.Type == api.ConditionTypeAvailable {
			hasAvailable = true
			break
		}
	}
	g.Expect(hasAvailable).To(BeTrue(), "Available condition should be preserved")
}

// TestProcessAdapterStatus_AllMandatoryConditions_WithCustom tests that valid
// updates with all mandatory conditions are accepted
func TestProcessAdapterStatus_AllMandatoryConditions_WithCustom(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()
	config := testAdapterConfig()
	service := NewClusterService(clusterDao, newMockNodePoolDao(), adapterStatusDao, config)

	ctx := context.Background()
	clusterID := testClusterID

	// Create cluster first
	cluster := &api.Cluster{Generation: 1}
	cluster.ID = clusterID
	_, svcErr := service.Create(ctx, cluster)
	g.Expect(svcErr).To(BeNil())

	// Send status with all mandatory conditions + custom condition
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: "CustomCondition", Status: api.AdapterConditionFalse, LastTransitionTime: time.Now()},
	}
	conditionsJSON, _ := json.Marshal(conditions)
	now := time.Now()
	adapterStatus := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         clusterID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}

	result, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)

	// Should be accepted
	g.Expect(err).To(BeNil())
	g.Expect(result).ToNot(BeNil(), "Update with all mandatory conditions should be accepted")

	// Verify status was stored with all 4 conditions
	var storedConditions []api.AdapterCondition
	unmarshalErr := json.Unmarshal(result.Conditions, &storedConditions)
	g.Expect(unmarshalErr).To(BeNil())
	g.Expect(len(storedConditions)).To(Equal(4), "All 4 conditions should be stored")

	// Verify all conditions are present
	conditionTypes := make(map[string]bool)
	for _, cond := range storedConditions {
		conditionTypes[cond.Type] = true
	}
	g.Expect(conditionTypes[api.ConditionTypeAvailable]).To(BeTrue())
	g.Expect(conditionTypes[api.ConditionTypeApplied]).To(BeTrue())
	g.Expect(conditionTypes[api.ConditionTypeHealth]).To(BeTrue())
	g.Expect(conditionTypes["CustomCondition"]).To(BeTrue())
}

// TestProcessAdapterStatus_CustomConditionRemoval tests that custom conditions can be removed
func TestProcessAdapterStatus_CustomConditionRemoval(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()
	config := testAdapterConfig()
	service := NewClusterService(clusterDao, newMockNodePoolDao(), adapterStatusDao, config)

	ctx := context.Background()
	clusterID := testClusterID

	// Create cluster first
	cluster := &api.Cluster{Generation: 1}
	cluster.ID = clusterID
	_, svcErr := service.Create(ctx, cluster)
	g.Expect(svcErr).To(BeNil())

	// First update: send all mandatory + custom condition
	conditions1 := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: "CustomCondition", Status: api.AdapterConditionFalse, LastTransitionTime: time.Now()},
	}
	conditionsJSON1, _ := json.Marshal(conditions1)
	now := time.Now()
	adapterStatus1 := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         clusterID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON1,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}
	result1, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus1)
	g.Expect(err).To(BeNil())
	g.Expect(result1).ToNot(BeNil())

	var storedConditions1 []api.AdapterCondition
	unmarshalErr := json.Unmarshal(result1.Conditions, &storedConditions1)
	g.Expect(unmarshalErr).To(BeNil())
	g.Expect(len(storedConditions1)).To(Equal(4))

	// Allow observed_generation=2 on the adapter report while the cluster is still at generation 1.
	clusterDao.clusters[clusterID].Generation = 2

	// Second update: remove custom condition (only send mandatory conditions)
	conditions2 := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionFalse, LastTransitionTime: time.Now()},
	}
	conditionsJSON2, _ := json.Marshal(conditions2)
	adapterStatus2 := &api.AdapterStatus{
		ResourceType:       "Cluster",
		ResourceID:         clusterID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON2,
		ObservedGeneration: 2,
		CreatedTime:        now,
		LastReportTime:     now,
	}
	result2, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus2)
	g.Expect(err).To(BeNil())
	g.Expect(result2).ToNot(BeNil())

	// Verify CustomCondition was removed
	var storedConditions2 []api.AdapterCondition
	unmarshalErr = json.Unmarshal(result2.Conditions, &storedConditions2)
	g.Expect(unmarshalErr).To(BeNil())
	g.Expect(len(storedConditions2)).To(Equal(3), "CustomCondition should be removed")

	conditionTypes := make(map[string]bool)
	for _, cond := range storedConditions2 {
		conditionTypes[cond.Type] = true
	}
	g.Expect(conditionTypes[api.ConditionTypeAvailable]).To(BeTrue())
	g.Expect(conditionTypes[api.ConditionTypeApplied]).To(BeTrue())
	g.Expect(conditionTypes[api.ConditionTypeHealth]).To(BeTrue())
	g.Expect(conditionTypes["CustomCondition"]).To(BeFalse(), "CustomCondition should not be present")
}

func TestClusterSoftDelete(t *testing.T) {
	t.Run("given a live cluster, when soft-deleted, then deleted_time/deleted_by/generation are set", func(t *testing.T) {
		g := NewWithT(t)
		// Given:
		clusterDao := newMockClusterDao()
		nodePoolDao := newMockNodePoolDao()
		adapterStatusDao := newMockAdapterStatusDao()
		service := NewClusterService(clusterDao, nodePoolDao, adapterStatusDao, testAdapterConfig())
		ctx := context.Background()
		clusterID := "live-cluster"
		clusterDao.clusters[clusterID] = &api.Cluster{
			Meta:       api.Meta{ID: clusterID},
			Generation: 1,
		}
		// When:
		cluster, svcErr := service.SoftDelete(ctx, clusterID)
		// Then:
		g.Expect(svcErr).To(BeNil())
		g.Expect(cluster.DeletedTime).NotTo(BeNil())
		g.Expect(cluster.DeletedBy).NotTo(BeNil())
		g.Expect(*cluster.DeletedBy).To(Equal(systemActor))
		g.Expect(cluster.Generation).To(Equal(int32(2)))
	})

	t.Run("given a cluster with child nodepools, when soft-deleted, then all nodepools are also soft-deleted", func(t *testing.T) { //nolint:lll
		g := NewWithT(t)
		// Given:
		clusterDao := newMockClusterDao()
		nodePoolDao := newMockNodePoolDao()
		adapterStatusDao := newMockAdapterStatusDao()
		service := NewClusterService(clusterDao, nodePoolDao, adapterStatusDao, testAdapterConfig())
		ctx := context.Background()
		clusterID := "cluster-with-pools"
		clusterDao.clusters[clusterID] = &api.Cluster{Meta: api.Meta{ID: clusterID}, Generation: 1}
		nodePoolDao.nodePools["np-1"] = &api.NodePool{Meta: api.Meta{ID: "np-1"}, OwnerID: clusterID, Generation: 1}
		nodePoolDao.nodePools["np-2"] = &api.NodePool{Meta: api.Meta{ID: "np-2"}, OwnerID: clusterID, Generation: 1}
		// When:
		cluster, svcErr := service.SoftDelete(ctx, clusterID)
		// Then:
		g.Expect(svcErr).To(BeNil())
		g.Expect(cluster.DeletedTime).NotTo(BeNil())
		g.Expect(nodePoolDao.nodePools["np-1"].DeletedTime).NotTo(BeNil())
		g.Expect(nodePoolDao.nodePools["np-2"].DeletedTime).NotTo(BeNil())
	})

	t.Run("given an already-deleted cluster, when soft-deleted again, then deleted_time and generation are unchanged", func(t *testing.T) { //nolint:lll
		g := NewWithT(t)
		// Given:
		clusterDao := newMockClusterDao()
		nodePoolDao := newMockNodePoolDao()
		adapterStatusDao := newMockAdapterStatusDao()
		service := NewClusterService(clusterDao, nodePoolDao, adapterStatusDao, testAdapterConfig())
		ctx := context.Background()
		clusterID := "already-deleted"
		originalTime := time.Now().Add(-time.Hour)
		clusterDao.clusters[clusterID] = &api.Cluster{
			Meta: api.Meta{ID: clusterID}, DeletedTime: &originalTime, Generation: 2,
		}
		nodePoolDao.nodePools["np-1"] = &api.NodePool{
			Meta: api.Meta{ID: "np-1"}, OwnerID: clusterID, DeletedTime: &originalTime, Generation: 2,
		}
		// When:
		cluster, svcErr := service.SoftDelete(ctx, clusterID)
		// Then:
		g.Expect(svcErr).To(BeNil())
		g.Expect(cluster.DeletedTime.Equal(originalTime)).To(BeTrue())
		g.Expect(cluster.Generation).To(Equal(int32(2)))
		g.Expect(nodePoolDao.nodePools["np-1"].DeletedTime.Equal(originalTime)).To(BeTrue())
	})

	t.Run("given a non-existent cluster ID, when soft-deleted, then returns 404 service error", func(t *testing.T) {
		g := NewWithT(t)
		// Given:
		clusterDao := newMockClusterDao()
		nodePoolDao := newMockNodePoolDao()
		adapterStatusDao := newMockAdapterStatusDao()
		service := NewClusterService(clusterDao, nodePoolDao, adapterStatusDao, testAdapterConfig())
		ctx := context.Background()
		// When:
		_, svcErr := service.SoftDelete(ctx, "nonexistent")
		// Then:
		g.Expect(svcErr).NotTo(BeNil())
		g.Expect(svcErr.HTTPCode).To(Equal(404))
	})

	t.Run("given a cluster with Ready=True, when soft-deleted, then Ready flips to False due to generation bump", func(t *testing.T) { //nolint:lll
		g := NewWithT(t)
		// Given:
		clusterDao := newMockClusterDao()
		nodePoolDao := newMockNodePoolDao()
		adapterStatusDao := newMockAdapterStatusDao()
		adapterConfig := testAdapterConfig()
		adapterConfig.Required.Cluster = []string{"validation"}
		service := NewClusterService(clusterDao, nodePoolDao, adapterStatusDao, adapterConfig)
		ctx := context.Background()
		clusterID := "ready-cluster"

		clusterDao.clusters[clusterID] = &api.Cluster{Meta: api.Meta{ID: clusterID}, Generation: 1}
		conditions := []api.AdapterCondition{
			{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
			{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
			{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		}
		condJSON, _ := json.Marshal(conditions)
		now := time.Now()
		_, svcErr := service.ProcessAdapterStatus(ctx, clusterID, &api.AdapterStatus{
			ResourceType: "Cluster", ResourceID: clusterID, Adapter: "validation",
			ObservedGeneration: 1, Conditions: condJSON, CreatedTime: now, LastReportTime: now,
		})
		g.Expect(svcErr).To(BeNil())

		// Pre-condition: Ready=True before soft-delete
		stored, _ := clusterDao.Get(ctx, clusterID)
		var preConds []api.ResourceCondition
		g.Expect(json.Unmarshal(stored.StatusConditions, &preConds)).To(Succeed())
		var preReady *api.ResourceCondition
		for i := range preConds {
			if preConds[i].Type == api.ConditionTypeReady {
				preReady = &preConds[i]
			}
		}
		g.Expect(preReady).NotTo(BeNil())
		g.Expect(preReady.Status).To(Equal(api.ConditionTrue))

		// When:
		_, svcErr = service.SoftDelete(ctx, clusterID)
		g.Expect(svcErr).To(BeNil())

		// Then: generation bumped to 2, Ready must flip to False
		stored, _ = clusterDao.Get(ctx, clusterID)
		g.Expect(stored.Generation).To(Equal(int32(2)))
		var postConds []api.ResourceCondition
		g.Expect(json.Unmarshal(stored.StatusConditions, &postConds)).To(Succeed())
		var postReady *api.ResourceCondition
		for i := range postConds {
			if postConds[i].Type == api.ConditionTypeReady {
				postReady = &postConds[i]
			}
		}
		g.Expect(postReady).NotTo(BeNil())
		g.Expect(postReady.Status).To(Equal(api.ConditionFalse))
		g.Expect(postReady.ObservedGeneration).To(Equal(int32(2)))
	})
}
