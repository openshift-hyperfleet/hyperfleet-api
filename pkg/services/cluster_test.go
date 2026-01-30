package services

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

const (
	testClusterID = "test-cluster-id"
)

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
	return nil, errors.NotFound("Cluster").AsError()
}

func (d *mockClusterDao) Create(ctx context.Context, cluster *api.Cluster) (*api.Cluster, error) {
	d.clusters[cluster.ID] = cluster
	return cluster, nil
}

func (d *mockClusterDao) Replace(ctx context.Context, cluster *api.Cluster) (*api.Cluster, error) {
	d.clusters[cluster.ID] = cluster
	return cluster, nil
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
	status.ID = key
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

// TestProcessAdapterStatus_UnknownCondition tests that Unknown Available condition returns nil (no-op)
func TestProcessAdapterStatus_UnknownCondition(t *testing.T) {
	RegisterTestingT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := config.NewAdapterRequirementsConfig()
	service := NewClusterService(clusterDao, adapterStatusDao, config)

	ctx := context.Background()
	clusterID := testClusterID

	// Create adapter status with Available=Unknown
	conditions := []api.AdapterCondition{
		{
			Type:               conditionTypeAvailable,
			Status:             api.AdapterConditionUnknown,
			LastTransitionTime: time.Now(),
		},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	adapterStatus := &api.AdapterStatus{
		ResourceType: "Cluster",
		ResourceID:   clusterID,
		Adapter:      "test-adapter",
		Conditions:   conditionsJSON,
	}

	result, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)

	Expect(err).To(BeNil())
	Expect(result).To(BeNil(), "ProcessAdapterStatus should return nil for Unknown status")

	// Verify nothing was stored
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, "Cluster", clusterID)
	Expect(len(storedStatuses)).To(Equal(0), "No status should be stored for Unknown")
}

// TestProcessAdapterStatus_TrueCondition tests that True Available condition upserts and aggregates
func TestProcessAdapterStatus_TrueCondition(t *testing.T) {
	RegisterTestingT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := config.NewAdapterRequirementsConfig()
	service := NewClusterService(clusterDao, adapterStatusDao, config)

	ctx := context.Background()
	clusterID := testClusterID

	// Create the cluster first
	cluster := &api.Cluster{
		Generation: 1,
	}
	cluster.ID = clusterID
	_, svcErr := service.Create(ctx, cluster)
	Expect(svcErr).To(BeNil())

	// Create adapter status with Available=True
	conditions := []api.AdapterCondition{
		{
			Type:               conditionTypeAvailable,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	now := time.Now()
	adapterStatus := &api.AdapterStatus{
		ResourceType: "Cluster",
		ResourceID:   clusterID,
		Adapter:      "test-adapter",
		Conditions:   conditionsJSON,
		CreatedTime:  &now,
	}

	result, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)

	Expect(err).To(BeNil())
	Expect(result).ToNot(BeNil(), "ProcessAdapterStatus should return the upserted status")
	Expect(result.Adapter).To(Equal("test-adapter"))

	// Verify the status was stored
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, "Cluster", clusterID)
	Expect(len(storedStatuses)).To(Equal(1), "Status should be stored for True condition")
}

// TestProcessAdapterStatus_FalseCondition tests that False Available condition upserts and aggregates
func TestProcessAdapterStatus_FalseCondition(t *testing.T) {
	RegisterTestingT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := config.NewAdapterRequirementsConfig()
	service := NewClusterService(clusterDao, adapterStatusDao, config)

	ctx := context.Background()
	clusterID := testClusterID

	// Create the cluster first
	cluster := &api.Cluster{
		Generation: 1,
	}
	cluster.ID = clusterID
	_, svcErr := service.Create(ctx, cluster)
	Expect(svcErr).To(BeNil())

	// Create adapter status with Available=False
	conditions := []api.AdapterCondition{
		{
			Type:               conditionTypeAvailable,
			Status:             api.AdapterConditionFalse,
			LastTransitionTime: time.Now(),
		},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	now := time.Now()
	adapterStatus := &api.AdapterStatus{
		ResourceType: "Cluster",
		ResourceID:   clusterID,
		Adapter:      "test-adapter",
		Conditions:   conditionsJSON,
		CreatedTime:  &now,
	}

	result, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)

	Expect(err).To(BeNil())
	Expect(result).ToNot(BeNil(), "ProcessAdapterStatus should return the upserted status")

	// Verify the status was stored
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, "Cluster", clusterID)
	Expect(len(storedStatuses)).To(Equal(1), "Status should be stored for False condition")
}

// TestProcessAdapterStatus_NoAvailableCondition tests when there's no Available condition
func TestProcessAdapterStatus_NoAvailableCondition(t *testing.T) {
	RegisterTestingT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := config.NewAdapterRequirementsConfig()
	service := NewClusterService(clusterDao, adapterStatusDao, config)

	ctx := context.Background()
	clusterID := testClusterID

	// Create the cluster first
	fixedNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	initialConditions := []api.ResourceCondition{
		{
			Type:               conditionTypeAvailable,
			Status:             api.ConditionFalse,
			ObservedGeneration: 1,
			LastTransitionTime: fixedNow,
			CreatedTime:        fixedNow,
			LastUpdatedTime:    fixedNow,
		},
		{
			Type:               "Ready",
			Status:             api.ConditionFalse,
			ObservedGeneration: 7,
			LastTransitionTime: fixedNow,
			CreatedTime:        fixedNow,
			LastUpdatedTime:    fixedNow,
		},
	}
	initialConditionsJSON, _ := json.Marshal(initialConditions)

	cluster := &api.Cluster{
		Generation:       7,
		StatusConditions: initialConditionsJSON,
	}
	cluster.ID = clusterID
	_, svcErr := service.Create(ctx, cluster)
	Expect(svcErr).To(BeNil())
	initialClusterStatusConditions := api.Cluster{}.StatusConditions
	initialClusterStatusConditions = append(initialClusterStatusConditions, cluster.StatusConditions...)

	// Create adapter status with Health condition (no Available)
	conditions := []api.AdapterCondition{
		{
			Type:               "Health",
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	now := time.Now()
	adapterStatus := &api.AdapterStatus{
		ResourceType: "Cluster",
		ResourceID:   clusterID,
		Adapter:      "test-adapter",
		Conditions:   conditionsJSON,
		CreatedTime:  &now,
	}

	result, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)

	Expect(err).To(BeNil())
	Expect(result).ToNot(BeNil(), "ProcessAdapterStatus should proceed when no Available condition")

	// Verify the status was stored
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, "Cluster", clusterID)
	Expect(len(storedStatuses)).To(Equal(1), "Status should be stored when no Available condition")

	// Verify that saving a non-Available condition did not overwrite cluster Available/Ready
	storedCluster, _ := clusterDao.Get(ctx, clusterID)
	Expect(storedCluster.StatusConditions).To(Equal(initialClusterStatusConditions),
		"Cluster status conditions should not be overwritten when adapter status lacks Available")
}

// TestProcessAdapterStatus_MultipleConditions_AvailableUnknown tests multiple conditions with Available=Unknown
func TestProcessAdapterStatus_MultipleConditions_AvailableUnknown(t *testing.T) {
	RegisterTestingT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := config.NewAdapterRequirementsConfig()
	service := NewClusterService(clusterDao, adapterStatusDao, config)

	ctx := context.Background()
	clusterID := testClusterID

	// Create adapter status with multiple conditions including Available=Unknown
	conditions := []api.AdapterCondition{
		{
			Type:               "Ready",
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               conditionTypeAvailable,
			Status:             api.AdapterConditionUnknown,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               "Progressing",
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	adapterStatus := &api.AdapterStatus{
		ResourceType: "Cluster",
		ResourceID:   clusterID,
		Adapter:      "test-adapter",
		Conditions:   conditionsJSON,
	}

	result, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)

	Expect(err).To(BeNil())
	Expect(result).To(BeNil(), "ProcessAdapterStatus should return nil when Available=Unknown")

	// Verify nothing was stored
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, "Cluster", clusterID)
	Expect(len(storedStatuses)).To(Equal(0), "No status should be stored for Unknown")
}

func TestClusterAvailableReadyTransitions(t *testing.T) {
	RegisterTestingT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()

	adapterConfig := config.NewAdapterRequirementsConfig()
	// Keep this small so we can cover transitions succinctly.
	adapterConfig.RequiredClusterAdapters = []string{"validation", "dns"}

	service := NewClusterService(clusterDao, adapterStatusDao, adapterConfig)

	ctx := context.Background()
	clusterID := testClusterID

	cluster := &api.Cluster{Generation: 1}
	cluster.ID = clusterID
	_, svcErr := service.Create(ctx, cluster)
	Expect(svcErr).To(BeNil())

	getSynth := func() (api.ResourceCondition, api.ResourceCondition) {
		stored, getErr := clusterDao.Get(ctx, clusterID)
		Expect(getErr).To(BeNil())

		var conds []api.ResourceCondition
		Expect(json.Unmarshal(stored.StatusConditions, &conds)).To(Succeed())
		Expect(len(conds)).To(BeNumerically(">=", 2))

		var available, ready *api.ResourceCondition
		for i := range conds {
			switch conds[i].Type {
			case conditionTypeAvailable:
				available = &conds[i]
			case conditionTypeReady:
				ready = &conds[i]
			}
		}
		Expect(available).ToNot(BeNil())
		Expect(ready).ToNot(BeNil())
		return *available, *ready
	}

	upsert := func(adapter string, available api.AdapterConditionStatus, observedGen int32) {
		conditions := []api.AdapterCondition{
			{Type: conditionTypeAvailable, Status: available, LastTransitionTime: time.Now()},
		}
		conditionsJSON, _ := json.Marshal(conditions)
		now := time.Now()

		adapterStatus := &api.AdapterStatus{
			ResourceType:       "Cluster",
			ResourceID:         clusterID,
			Adapter:            adapter,
			ObservedGeneration: observedGen,
			Conditions:         conditionsJSON,
			CreatedTime:        &now,
			LastReportTime:     &now,
		}

		_, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)
		Expect(err).To(BeNil())
	}

	// No adapter statuses yet.
	_, err := service.UpdateClusterStatusFromAdapters(ctx, clusterID)
	Expect(err).To(BeNil())
	avail, ready := getSynth()
	Expect(avail.Status).To(Equal(api.ConditionFalse))
	Expect(avail.ObservedGeneration).To(Equal(int32(1)))
	Expect(ready.Status).To(Equal(api.ConditionFalse))
	Expect(ready.ObservedGeneration).To(Equal(int32(1)))

	// Partial adapters: still not Available/Ready.
	upsert("validation", api.AdapterConditionTrue, 1)
	avail, ready = getSynth()
	Expect(avail.Status).To(Equal(api.ConditionFalse))
	Expect(ready.Status).To(Equal(api.ConditionFalse))

	// All required adapters available at gen=1 => Available=True, Ready=True.
	upsert("dns", api.AdapterConditionTrue, 1)
	avail, ready = getSynth()
	Expect(avail.Status).To(Equal(api.ConditionTrue))
	Expect(avail.ObservedGeneration).To(Equal(int32(1)))
	Expect(ready.Status).To(Equal(api.ConditionTrue))
	Expect(ready.ObservedGeneration).To(Equal(int32(1)))

	// Bump resource generation => Ready flips to False; Available remains True.
	clusterDao.clusters[clusterID].Generation = 2
	_, err = service.UpdateClusterStatusFromAdapters(ctx, clusterID)
	Expect(err).To(BeNil())
	avail, ready = getSynth()
	Expect(avail.Status).To(Equal(api.ConditionTrue))
	Expect(avail.ObservedGeneration).To(Equal(int32(1)))
	Expect(ready.Status).To(Equal(api.ConditionFalse))
	Expect(ready.ObservedGeneration).To(Equal(int32(2)))

	// One adapter updates to gen=2 => Ready still False; Available still True (minObservedGeneration still 1).
	upsert("validation", api.AdapterConditionTrue, 2)
	avail, ready = getSynth()
	Expect(avail.Status).To(Equal(api.ConditionTrue))
	Expect(avail.ObservedGeneration).To(Equal(int32(1)))
	Expect(ready.Status).To(Equal(api.ConditionFalse))

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
	Expect(avail.Status).To(Equal(api.ConditionTrue)) // <-- this is the edge case
	Expect(avail.ObservedGeneration).To(Equal(int32(1)))
	Expect(ready.Status).To(Equal(api.ConditionFalse))

	// All required adapters at gen=2 => Ready becomes True, Available minObservedGeneration becomes 2.
	upsert("dns", api.AdapterConditionTrue, 2)
	avail, ready = getSynth()
	Expect(avail.Status).To(Equal(api.ConditionTrue))
	Expect(avail.ObservedGeneration).To(Equal(int32(2)))
	Expect(ready.Status).To(Equal(api.ConditionTrue))

	// One required adapter goes False => both Available and Ready become False.
	upsert("dns", api.AdapterConditionFalse, 2)
	avail, ready = getSynth()
	Expect(avail.Status).To(Equal(api.ConditionFalse))
	Expect(avail.ObservedGeneration).To(Equal(int32(0)))
	Expect(ready.Status).To(Equal(api.ConditionFalse))

	// Available=Unknown is a no-op (does not store, does not overwrite cluster conditions).
	prevStatus := api.Cluster{}.StatusConditions
	prevStatus = append(prevStatus, clusterDao.clusters[clusterID].StatusConditions...)
	unknownConds := []api.AdapterCondition{
		{Type: conditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
	}
	unknownJSON, _ := json.Marshal(unknownConds)
	unknownStatus := &api.AdapterStatus{
		ResourceType: "Cluster",
		ResourceID:   clusterID,
		Adapter:      "dns",
		Conditions:   unknownJSON,
	}
	result, svcErr := service.ProcessAdapterStatus(ctx, clusterID, unknownStatus)
	Expect(svcErr).To(BeNil())
	Expect(result).To(BeNil())
	Expect(clusterDao.clusters[clusterID].StatusConditions).To(Equal(prevStatus))
}

func TestClusterStaleAdapterStatusUpdatePolicy(t *testing.T) {
	RegisterTestingT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()

	adapterConfig := config.NewAdapterRequirementsConfig()
	adapterConfig.RequiredClusterAdapters = []string{"validation", "dns"}

	service := NewClusterService(clusterDao, adapterStatusDao, adapterConfig)

	ctx := context.Background()
	clusterID := testClusterID

	cluster := &api.Cluster{Generation: 2}
	cluster.ID = clusterID
	_, svcErr := service.Create(ctx, cluster)
	Expect(svcErr).To(BeNil())

	getAvailable := func() api.ResourceCondition {
		stored, getErr := clusterDao.Get(ctx, clusterID)
		Expect(getErr).To(BeNil())

		var conds []api.ResourceCondition
		Expect(json.Unmarshal(stored.StatusConditions, &conds)).To(Succeed())
		for i := range conds {
			if conds[i].Type == conditionTypeAvailable {
				return conds[i]
			}
		}
		Expect(true).To(BeFalse(), "Available condition not found")
		return api.ResourceCondition{}
	}

	upsert := func(adapter string, available api.AdapterConditionStatus, observedGen int32) {
		conditions := []api.AdapterCondition{
			{Type: conditionTypeAvailable, Status: available, LastTransitionTime: time.Now()},
		}
		conditionsJSON, _ := json.Marshal(conditions)
		now := time.Now()

		adapterStatus := &api.AdapterStatus{
			ResourceType:       "Cluster",
			ResourceID:         clusterID,
			Adapter:            adapter,
			ObservedGeneration: observedGen,
			Conditions:         conditionsJSON,
			CreatedTime:        &now,
			LastReportTime:     &now,
		}

		_, err := service.ProcessAdapterStatus(ctx, clusterID, adapterStatus)
		Expect(err).To(BeNil())
	}

	// Current generation statuses => Available=True at observed_generation=2.
	upsert("validation", api.AdapterConditionTrue, 2)
	upsert("dns", api.AdapterConditionTrue, 2)
	available := getAvailable()
	Expect(available.Status).To(Equal(api.ConditionTrue))
	Expect(available.ObservedGeneration).To(Equal(int32(2)))

	// Stale True should not override newer True.
	upsert("validation", api.AdapterConditionTrue, 1)
	available = getAvailable()
	Expect(available.Status).To(Equal(api.ConditionTrue))
	Expect(available.ObservedGeneration).To(Equal(int32(2)))

	// Stale False is more restrictive and should override.
	upsert("validation", api.AdapterConditionFalse, 1)
	available = getAvailable()
	Expect(available.Status).To(Equal(api.ConditionTrue))
	Expect(available.ObservedGeneration).To(Equal(int32(2)))
}

func TestClusterSyntheticTimestampsStableWithoutAdapterStatus(t *testing.T) {
	RegisterTestingT(t)

	clusterDao := newMockClusterDao()
	adapterStatusDao := newMockAdapterStatusDao()

	adapterConfig := config.NewAdapterRequirementsConfig()
	adapterConfig.RequiredClusterAdapters = []string{"validation"}

	service := NewClusterService(clusterDao, adapterStatusDao, adapterConfig)

	ctx := context.Background()
	clusterID := testClusterID

	fixedNow := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	initialConditions := []api.ResourceCondition{
		{
			Type:               conditionTypeAvailable,
			Status:             api.ConditionFalse,
			ObservedGeneration: 1,
			LastTransitionTime: fixedNow,
			CreatedTime:        fixedNow,
			LastUpdatedTime:    fixedNow,
		},
		{
			Type:               "Ready",
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
	created, svcErr := service.Create(ctx, cluster)
	Expect(svcErr).To(BeNil())

	var createdConds []api.ResourceCondition
	Expect(json.Unmarshal(created.StatusConditions, &createdConds)).To(Succeed())
	Expect(len(createdConds)).To(BeNumerically(">=", 2))

	var createdAvailable, createdReady *api.ResourceCondition
	for i := range createdConds {
		switch createdConds[i].Type {
		case conditionTypeAvailable:
			createdAvailable = &createdConds[i]
		case conditionTypeReady:
			createdReady = &createdConds[i]
		}
	}
	Expect(createdAvailable).ToNot(BeNil())
	Expect(createdReady).ToNot(BeNil())
	Expect(createdAvailable.CreatedTime).To(Equal(fixedNow))
	Expect(createdAvailable.LastTransitionTime).To(Equal(fixedNow))
	Expect(createdAvailable.LastUpdatedTime).To(Equal(fixedNow))
	Expect(createdReady.CreatedTime).To(Equal(fixedNow))
	Expect(createdReady.LastTransitionTime).To(Equal(fixedNow))
	Expect(createdReady.LastUpdatedTime).To(Equal(fixedNow))

	updated, err := service.UpdateClusterStatusFromAdapters(ctx, clusterID)
	Expect(err).To(BeNil())

	var updatedConds []api.ResourceCondition
	Expect(json.Unmarshal(updated.StatusConditions, &updatedConds)).To(Succeed())
	Expect(len(updatedConds)).To(BeNumerically(">=", 2))

	var updatedAvailable, updatedReady *api.ResourceCondition
	for i := range updatedConds {
		switch updatedConds[i].Type {
		case conditionTypeAvailable:
			updatedAvailable = &updatedConds[i]
		case conditionTypeReady:
			updatedReady = &updatedConds[i]
		}
	}
	Expect(updatedAvailable).ToNot(BeNil())
	Expect(updatedReady).ToNot(BeNil())
	Expect(updatedAvailable.CreatedTime).To(Equal(fixedNow))
	Expect(updatedAvailable.LastTransitionTime).To(Equal(fixedNow))
	Expect(updatedAvailable.LastUpdatedTime).To(Equal(fixedNow))
	Expect(updatedReady.CreatedTime).To(Equal(fixedNow))
	Expect(updatedReady.LastTransitionTime).To(Equal(fixedNow))
	Expect(updatedReady.LastUpdatedTime).To(Equal(fixedNow))
}
