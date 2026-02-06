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
	testNodePoolID = "test-nodepool-id"
)

// testNodePoolAdapterConfig creates a test adapter config with default values
func testNodePoolAdapterConfig() *config.AdapterRequirementsConfig {
	return &config.AdapterRequirementsConfig{
		RequiredClusterAdapters:  []string{"validation", "dns", "pullsecret", "hypershift"},
		RequiredNodePoolAdapters: []string{"validation", "hypershift"},
	}
}

// Mock implementations for testing NodePool ProcessAdapterStatus

type mockNodePoolDao struct {
	nodePools map[string]*api.NodePool
}

func newMockNodePoolDao() *mockNodePoolDao {
	return &mockNodePoolDao{
		nodePools: make(map[string]*api.NodePool),
	}
}

func (d *mockNodePoolDao) Get(ctx context.Context, id string) (*api.NodePool, error) {
	if np, ok := d.nodePools[id]; ok {
		return np, nil
	}
	return nil, errors.NotFound("NodePool").AsError()
}

func (d *mockNodePoolDao) Create(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, error) {
	d.nodePools[nodePool.ID] = nodePool
	return nodePool, nil
}

func (d *mockNodePoolDao) Replace(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, error) {
	d.nodePools[nodePool.ID] = nodePool
	return nodePool, nil
}

func (d *mockNodePoolDao) Delete(ctx context.Context, id string) error {
	delete(d.nodePools, id)
	return nil
}

func (d *mockNodePoolDao) FindByIDs(ctx context.Context, ids []string) (api.NodePoolList, error) {
	var result api.NodePoolList
	for _, id := range ids {
		if np, ok := d.nodePools[id]; ok {
			result = append(result, np)
		}
	}
	return result, nil
}

func (d *mockNodePoolDao) All(ctx context.Context) (api.NodePoolList, error) {
	var result api.NodePoolList
	for _, np := range d.nodePools {
		result = append(result, np)
	}
	return result, nil
}

var _ dao.NodePoolDao = &mockNodePoolDao{}

// TestNodePoolProcessAdapterStatus_UnknownCondition tests that Unknown Available condition returns nil (no-op)
func TestNodePoolProcessAdapterStatus_UnknownCondition(t *testing.T) {
	RegisterTestingT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := testNodePoolAdapterConfig()
	service := NewNodePoolService(nodePoolDao, adapterStatusDao, config)

	ctx := context.Background()
	nodePoolID := testNodePoolID

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
		ResourceType: "NodePool",
		ResourceID:   nodePoolID,
		Adapter:      "test-adapter",
		Conditions:   conditionsJSON,
	}

	result, err := service.ProcessAdapterStatus(ctx, nodePoolID, adapterStatus)

	Expect(err).To(BeNil())
	Expect(result).To(BeNil(), "ProcessAdapterStatus should return nil for Unknown status")

	// Verify nothing was stored
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, "NodePool", nodePoolID)
	Expect(len(storedStatuses)).To(Equal(0), "No status should be stored for Unknown")
}

// TestNodePoolProcessAdapterStatus_TrueCondition tests that True Available condition upserts and aggregates
func TestNodePoolProcessAdapterStatus_TrueCondition(t *testing.T) {
	RegisterTestingT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := testNodePoolAdapterConfig()
	service := NewNodePoolService(nodePoolDao, adapterStatusDao, config)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	// Create the nodepool first
	nodePool := &api.NodePool{
		Generation: 1,
	}
	nodePool.ID = nodePoolID
	_, svcErr := service.Create(ctx, nodePool)
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
		ResourceType: "NodePool",
		ResourceID:   nodePoolID,
		Adapter:      "test-adapter",
		Conditions:   conditionsJSON,
		CreatedTime:  &now,
	}

	result, err := service.ProcessAdapterStatus(ctx, nodePoolID, adapterStatus)

	Expect(err).To(BeNil())
	Expect(result).ToNot(BeNil(), "ProcessAdapterStatus should return the upserted status")
	Expect(result.Adapter).To(Equal("test-adapter"))

	// Verify the status was stored
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, "NodePool", nodePoolID)
	Expect(len(storedStatuses)).To(Equal(1), "Status should be stored for True condition")
}

// TestNodePoolProcessAdapterStatus_MultipleConditions_AvailableUnknown tests multiple conditions with Available=Unknown
func TestNodePoolProcessAdapterStatus_MultipleConditions_AvailableUnknown(t *testing.T) {
	RegisterTestingT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := testNodePoolAdapterConfig()
	service := NewNodePoolService(nodePoolDao, adapterStatusDao, config)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	// Create adapter status with multiple conditions including Available=Unknown
	conditions := []api.AdapterCondition{
		{
			Type:               conditionTypeReady,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               conditionTypeAvailable,
			Status:             api.AdapterConditionUnknown,
			LastTransitionTime: time.Now(),
		},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	adapterStatus := &api.AdapterStatus{
		ResourceType: "NodePool",
		ResourceID:   nodePoolID,
		Adapter:      "test-adapter",
		Conditions:   conditionsJSON,
	}

	result, err := service.ProcessAdapterStatus(ctx, nodePoolID, adapterStatus)

	Expect(err).To(BeNil())
	Expect(result).To(BeNil(), "ProcessAdapterStatus should return nil when Available=Unknown")

	// Verify nothing was stored
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, "NodePool", nodePoolID)
	Expect(len(storedStatuses)).To(Equal(0), "No status should be stored for Unknown")
}

func TestNodePoolAvailableReadyTransitions(t *testing.T) {
	RegisterTestingT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()

	adapterConfig := testNodePoolAdapterConfig()
	adapterConfig.RequiredNodePoolAdapters = []string{"validation", "hypershift"}

	service := NewNodePoolService(nodePoolDao, adapterStatusDao, adapterConfig)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	nodePool := &api.NodePool{Generation: 1}
	nodePool.ID = nodePoolID
	_, svcErr := service.Create(ctx, nodePool)
	Expect(svcErr).To(BeNil())

	getSynth := func() (api.ResourceCondition, api.ResourceCondition) {
		stored, getErr := nodePoolDao.Get(ctx, nodePoolID)
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
			ResourceType:       "NodePool",
			ResourceID:         nodePoolID,
			Adapter:            adapter,
			ObservedGeneration: observedGen,
			Conditions:         conditionsJSON,
			CreatedTime:        &now,
			LastReportTime:     &now,
		}

		_, err := service.ProcessAdapterStatus(ctx, nodePoolID, adapterStatus)
		Expect(err).To(BeNil())
	}

	// No adapter statuses yet.
	_, err := service.UpdateNodePoolStatusFromAdapters(ctx, nodePoolID)
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
	upsert("hypershift", api.AdapterConditionTrue, 1)
	avail, ready = getSynth()
	Expect(avail.Status).To(Equal(api.ConditionTrue))
	Expect(avail.ObservedGeneration).To(Equal(int32(1)))
	Expect(ready.Status).To(Equal(api.ConditionTrue))

	// Bump resource generation => Ready flips to False; Available remains True.
	nodePoolDao.nodePools[nodePoolID].Generation = 2
	_, err = service.UpdateNodePoolStatusFromAdapters(ctx, nodePoolID)
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

	// All required adapters at gen=2 => Ready becomes True, Available minObservedGeneration becomes 2.
	upsert("hypershift", api.AdapterConditionTrue, 2)
	avail, ready = getSynth()
	Expect(avail.Status).To(Equal(api.ConditionTrue))
	Expect(avail.ObservedGeneration).To(Equal(int32(2)))
	Expect(ready.Status).To(Equal(api.ConditionTrue))

	// One required adapter goes False => both Available and Ready become False.
	upsert("hypershift", api.AdapterConditionFalse, 2)
	avail, ready = getSynth()
	Expect(avail.Status).To(Equal(api.ConditionFalse))
	Expect(avail.ObservedGeneration).To(Equal(int32(0)))
	Expect(ready.Status).To(Equal(api.ConditionFalse))

	// Adapter status with no Available condition should not overwrite synthetic conditions.
	prevStatus := api.NodePool{}.StatusConditions
	prevStatus = append(prevStatus, nodePoolDao.nodePools[nodePoolID].StatusConditions...)
	nonAvailableConds := []api.AdapterCondition{
		{Type: "Health", Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	nonAvailableJSON, _ := json.Marshal(nonAvailableConds)
	nonAvailableStatus := &api.AdapterStatus{
		ResourceType:       "NodePool",
		ResourceID:         nodePoolID,
		Adapter:            "hypershift",
		ObservedGeneration: 2,
		Conditions:         nonAvailableJSON,
	}
	result, svcErr := service.ProcessAdapterStatus(ctx, nodePoolID, nonAvailableStatus)
	Expect(svcErr).To(BeNil())
	Expect(result).ToNot(BeNil())
	Expect(nodePoolDao.nodePools[nodePoolID].StatusConditions).To(Equal(prevStatus))

	// Available=Unknown is a no-op (does not store, does not overwrite nodepool conditions).
	prevStatus = api.NodePool{}.StatusConditions
	prevStatus = append(prevStatus, nodePoolDao.nodePools[nodePoolID].StatusConditions...)
	unknownConds := []api.AdapterCondition{
		{Type: conditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
	}
	unknownJSON, _ := json.Marshal(unknownConds)
	unknownStatus := &api.AdapterStatus{
		ResourceType: "NodePool",
		ResourceID:   nodePoolID,
		Adapter:      "hypershift",
		Conditions:   unknownJSON,
	}
	result, svcErr = service.ProcessAdapterStatus(ctx, nodePoolID, unknownStatus)
	Expect(svcErr).To(BeNil())
	Expect(result).To(BeNil())
	Expect(nodePoolDao.nodePools[nodePoolID].StatusConditions).To(Equal(prevStatus))
}

func TestNodePoolStaleAdapterStatusUpdatePolicy(t *testing.T) {
	RegisterTestingT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()

	adapterConfig := testNodePoolAdapterConfig()
	adapterConfig.RequiredNodePoolAdapters = []string{"validation", "hypershift"}

	service := NewNodePoolService(nodePoolDao, adapterStatusDao, adapterConfig)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	nodePool := &api.NodePool{Generation: 2}
	nodePool.ID = nodePoolID
	_, svcErr := service.Create(ctx, nodePool)
	Expect(svcErr).To(BeNil())

	getAvailable := func() api.ResourceCondition {
		stored, getErr := nodePoolDao.Get(ctx, nodePoolID)
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
			ResourceType:       "NodePool",
			ResourceID:         nodePoolID,
			Adapter:            adapter,
			ObservedGeneration: observedGen,
			Conditions:         conditionsJSON,
			CreatedTime:        &now,
			LastReportTime:     &now,
		}

		_, err := service.ProcessAdapterStatus(ctx, nodePoolID, adapterStatus)
		Expect(err).To(BeNil())
	}

	// Current generation statuses => Available=True at observed_generation=2.
	upsert("validation", api.AdapterConditionTrue, 2)
	upsert("hypershift", api.AdapterConditionTrue, 2)
	available := getAvailable()
	Expect(available.Status).To(Equal(api.ConditionTrue))
	Expect(available.ObservedGeneration).To(Equal(int32(2)))

	// Stale True should not override newer True.
	upsert("validation", api.AdapterConditionTrue, 1)
	available = getAvailable()
	Expect(available.Status).To(Equal(api.ConditionTrue))
	Expect(available.ObservedGeneration).To(Equal(int32(2)))

	// Stale False is more restrictive and should override but we do not override newer generation responses
	upsert("validation", api.AdapterConditionFalse, 1)
	available = getAvailable()
	Expect(available.Status).To(Equal(api.ConditionTrue))
	Expect(available.ObservedGeneration).To(Equal(int32(2)))
}

func TestNodePoolSyntheticTimestampsStableWithoutAdapterStatus(t *testing.T) {
	RegisterTestingT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()

	adapterConfig := testNodePoolAdapterConfig()
	adapterConfig.RequiredNodePoolAdapters = []string{"validation"}

	service := NewNodePoolService(nodePoolDao, adapterStatusDao, adapterConfig)

	ctx := context.Background()
	nodePoolID := testNodePoolID

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
			Type:               conditionTypeReady,
			Status:             api.ConditionFalse,
			ObservedGeneration: 1,
			LastTransitionTime: fixedNow,
			CreatedTime:        fixedNow,
			LastUpdatedTime:    fixedNow,
		},
	}
	initialConditionsJSON, _ := json.Marshal(initialConditions)

	nodePool := &api.NodePool{
		Generation:       1,
		StatusConditions: initialConditionsJSON,
	}
	nodePool.ID = nodePoolID
	created, svcErr := service.Create(ctx, nodePool)
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

	updated, err := service.UpdateNodePoolStatusFromAdapters(ctx, nodePoolID)
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
