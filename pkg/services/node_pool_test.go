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
	testNodePoolID = "test-nodepool-id"
)

// testNodePoolAdapterConfig creates a test adapter config with default values
func testNodePoolAdapterConfig() *config.AdapterRequirementsConfig {
	return &config.AdapterRequirementsConfig{
		Required: config.RequiredAdapters{
			Cluster:  []string{"validation", "dns", "pullsecret", "hypershift"},
			Nodepool: []string{"validation", "hypershift"},
		},
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
	if nodePool.CreatedTime.IsZero() {
		now := time.Now()
		nodePool.CreatedTime = now
	}
	if nodePool.UpdatedTime.IsZero() {
		nodePool.UpdatedTime = nodePool.CreatedTime
	}
	d.nodePools[nodePool.ID] = nodePool
	return nodePool, nil
}

func (d *mockNodePoolDao) Replace(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, error) {
	d.nodePools[nodePool.ID] = nodePool
	return nodePool, nil
}

func (d *mockNodePoolDao) RequestDeletion(ctx context.Context, id string) (*api.NodePool, error) {
	np, ok := d.nodePools[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	if np.DeletedAt != nil {
		return np, nil
	}
	t := time.Now()
	np.DeletedAt = &t
	np.Generation++
	d.nodePools[id] = np
	return np, nil
}

func (d *mockNodePoolDao) Delete(ctx context.Context, id string) error {
	delete(d.nodePools, id)
	return nil
}

func (d *mockNodePoolDao) FindByOwnerID(ctx context.Context, ownerID string) (api.NodePoolList, error) {
	var result api.NodePoolList
	for _, np := range d.nodePools {
		if np.OwnerID == ownerID {
			cp := *np
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (d *mockNodePoolDao) RequestDeletionByOwner(ctx context.Context, ownerID string, t time.Time) error {
	for id, np := range d.nodePools {
		if np.OwnerID == ownerID && np.DeletedAt == nil {
			np.DeletedAt = &t
			np.Generation++
			d.nodePools[id] = np
		}
	}
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

// TestNodePoolProcessAdapterStatus_FirstUnknownCondition tests that the first Unknown Available condition is stored
func TestNodePoolProcessAdapterStatus_FirstUnknownCondition(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := testNodePoolAdapterConfig()
	service := NewNodePoolService(nodePoolDao, adapterStatusDao, config)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	now := time.Now()
	nodePoolDao.nodePools[nodePoolID] = &api.NodePool{
		Meta:       api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
		Generation: 1,
	}

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
	}
	conditionsJSON, _ := json.Marshal(conditions)

	adapterStatus := &api.AdapterStatus{
		ResourceType:       "NodePool",
		ResourceID:         nodePoolID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}

	result, err := service.ProcessAdapterStatus(ctx, nodePoolID, adapterStatus)

	g.Expect(err).To(BeNil())
	g.Expect(result).ToNot(BeNil(), "First report with Available=Unknown should be accepted")
	g.Expect(result.Adapter).To(Equal("test-adapter"))

	// Verify the status was stored
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, "NodePool", nodePoolID)
	g.Expect(len(storedStatuses)).To(Equal(1), "First Unknown status should be stored")
}

// TestNodePoolProcessAdapterStatus_SubsequentUnknownCondition tests that subsequent Unknown conditions are discarded
func TestNodePoolProcessAdapterStatus_SubsequentUnknownCondition(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := testNodePoolAdapterConfig()
	service := NewNodePoolService(nodePoolDao, adapterStatusDao, config)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	now := time.Now()
	nodePoolDao.nodePools[nodePoolID] = &api.NodePool{
		Meta:       api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
		Generation: 1,
	}

	// Pre-populate an existing adapter status
	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	existingStatus := &api.AdapterStatus{
		ResourceType:       "NodePool",
		ResourceID:         nodePoolID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}
	_, _ = adapterStatusDao.Upsert(ctx, existingStatus)

	// Now send another Unknown status report
	newAdapterStatus := &api.AdapterStatus{
		ResourceType:       "NodePool",
		ResourceID:         nodePoolID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}

	result, err := service.ProcessAdapterStatus(ctx, nodePoolID, newAdapterStatus)

	g.Expect(err).To(BeNil())
	g.Expect(result).To(BeNil(), "Subsequent Unknown status should be discarded")
}

// TestNodePoolProcessAdapterStatus_InvalidStatusReturnsValidationError tests that a non-True/False/Unknown
// Available status is rejected with a validation error.
func TestNodePoolProcessAdapterStatus_InvalidStatusReturnsValidationError(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()
	config := testNodePoolAdapterConfig()
	service := NewNodePoolService(nodePoolDao, adapterStatusDao, config)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	now := time.Now()
	nodePoolDao.nodePools[nodePoolID] = &api.NodePool{
		Meta:       api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
		Generation: 1,
	}

	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: "Pending", LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	conditionsJSON, _ := json.Marshal(conditions)
	adapterStatus := &api.AdapterStatus{
		ResourceType:       "NodePool",
		ResourceID:         nodePoolID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}

	result, err := service.ProcessAdapterStatus(ctx, nodePoolID, adapterStatus)

	g.Expect(err).ToNot(BeNil(), "Invalid status should return a validation error")
	g.Expect(err.HTTPCode).To(Equal(http.StatusBadRequest))
	g.Expect(result).To(BeNil())
}

// TestNodePoolProcessAdapterStatus_EmptyStatusReturnsValidationError tests that an empty Available
// status is rejected with a validation error.
func TestNodePoolProcessAdapterStatus_EmptyStatusReturnsValidationError(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()
	config := testNodePoolAdapterConfig()
	service := NewNodePoolService(nodePoolDao, adapterStatusDao, config)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	now := time.Now()
	nodePoolDao.nodePools[nodePoolID] = &api.NodePool{
		Meta:       api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
		Generation: 1,
	}

	conditions := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: "", LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	conditionsJSON, _ := json.Marshal(conditions)
	adapterStatus := &api.AdapterStatus{
		ResourceType:       "NodePool",
		ResourceID:         nodePoolID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}

	result, err := service.ProcessAdapterStatus(ctx, nodePoolID, adapterStatus)

	g.Expect(err).ToNot(BeNil(), "Empty status should return a validation error")
	g.Expect(err.HTTPCode).To(Equal(http.StatusBadRequest))
	g.Expect(result).To(BeNil())
}

// TestNodePoolProcessAdapterStatus_TrueCondition tests that True Available condition upserts and aggregates
func TestNodePoolProcessAdapterStatus_TrueCondition(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

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
		ResourceType:   "NodePool",
		ResourceID:     nodePoolID,
		Adapter:        "test-adapter",
		Conditions:     conditionsJSON,
		CreatedTime:    now,
		LastReportTime: now,
	}

	result, err := service.ProcessAdapterStatus(ctx, nodePoolID, adapterStatus)

	g.Expect(err).To(BeNil())
	g.Expect(result).ToNot(BeNil(), "ProcessAdapterStatus should return the upserted status")
	g.Expect(result.Adapter).To(Equal("test-adapter"))

	// Verify the status was stored
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, "NodePool", nodePoolID)
	g.Expect(len(storedStatuses)).To(Equal(1), "Status should be stored for True condition")
}

// TestNodePoolProcessAdapterStatus_FirstMultipleConditions_AvailableUnknown tests that first reports
// with Available=Unknown are accepted even when other conditions are present
func TestNodePoolProcessAdapterStatus_FirstMultipleConditions_AvailableUnknown(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := testNodePoolAdapterConfig()
	service := NewNodePoolService(nodePoolDao, adapterStatusDao, config)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	now := time.Now()
	nodePoolDao.nodePools[nodePoolID] = &api.NodePool{
		Meta:       api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
		Generation: 1,
	}

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

	adapterStatus := &api.AdapterStatus{
		ResourceType:       "NodePool",
		ResourceID:         nodePoolID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}

	result, err := service.ProcessAdapterStatus(ctx, nodePoolID, adapterStatus)

	g.Expect(err).To(BeNil())
	g.Expect(result).ToNot(BeNil(), "First report with Available=Unknown should be accepted")

	// Verify the status was stored
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, "NodePool", nodePoolID)
	g.Expect(len(storedStatuses)).To(Equal(1), "First status with Available=Unknown should be stored")
}

// TestNodePoolProcessAdapterStatus_SubsequentMultipleConditions_AvailableUnknown tests that subsequent
// reports with multiple conditions including Available=Unknown are discarded
func TestNodePoolProcessAdapterStatus_SubsequentMultipleConditions_AvailableUnknown(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := testNodePoolAdapterConfig()
	service := NewNodePoolService(nodePoolDao, adapterStatusDao, config)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	now := time.Now()
	nodePoolDao.nodePools[nodePoolID] = &api.NodePool{
		Meta:       api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
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
		ResourceType:       "NodePool",
		ResourceID:         nodePoolID,
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
		ResourceType:       "NodePool",
		ResourceID:         nodePoolID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}

	result, err := service.ProcessAdapterStatus(ctx, nodePoolID, adapterStatus)

	g.Expect(err).To(BeNil())
	g.Expect(result).To(BeNil(), "Subsequent Available=Unknown should be discarded")
}

func TestNodePoolAvailableReadyTransitions(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()

	adapterConfig := testNodePoolAdapterConfig()
	adapterConfig.Required.Nodepool = []string{"validation", "hypershift"}

	service := NewNodePoolService(nodePoolDao, adapterStatusDao, adapterConfig)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	nodePool := &api.NodePool{Generation: 1}
	nodePool.ID = nodePoolID
	_, svcErr := service.Create(ctx, nodePool)
	g.Expect(svcErr).To(BeNil())

	getSynth := func() (api.ResourceCondition, api.ResourceCondition) {
		stored, getErr := nodePoolDao.Get(ctx, nodePoolID)
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
			ResourceType:       "NodePool",
			ResourceID:         nodePoolID,
			Adapter:            adapter,
			ObservedGeneration: observedGen,
			Conditions:         conditionsJSON,
			CreatedTime:        now,
			LastReportTime:     now,
		}

		_, err := service.ProcessAdapterStatus(ctx, nodePoolID, adapterStatus)
		g.Expect(err).To(BeNil())
	}

	// No adapter statuses yet.
	_, err := service.UpdateNodePoolStatusFromAdapters(ctx, nodePoolID)
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
	upsert("hypershift", api.AdapterConditionTrue, 1)
	avail, ready = getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionTrue))
	g.Expect(avail.ObservedGeneration).To(Equal(int32(1)))
	g.Expect(ready.Status).To(Equal(api.ConditionTrue))

	// Bump resource generation => Ready flips to False; Available remains True.
	nodePoolDao.nodePools[nodePoolID].Generation = 2
	_, err = service.UpdateNodePoolStatusFromAdapters(ctx, nodePoolID)
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

	// All required adapters at gen=2 => Ready becomes True, Available minObservedGeneration becomes 2.
	upsert("hypershift", api.AdapterConditionTrue, 2)
	avail, ready = getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionTrue))
	g.Expect(avail.ObservedGeneration).To(Equal(int32(2)))
	g.Expect(ready.Status).To(Equal(api.ConditionTrue))

	// One required adapter goes False => both Available and Ready become False.
	upsert("hypershift", api.AdapterConditionFalse, 2)
	avail, ready = getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionFalse))
	g.Expect(avail.ObservedGeneration).To(Equal(int32(2)))
	g.Expect(ready.Status).To(Equal(api.ConditionFalse))

	// Adapter status missing mandatory conditions should be rejected and not overwrite synthetic conditions.
	prevStatus := api.NodePool{}.StatusConditions
	prevStatus = append(prevStatus, nodePoolDao.nodePools[nodePoolID].StatusConditions...)
	nonAvailableConds := []api.AdapterCondition{
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	nonAvailableJSON, _ := json.Marshal(nonAvailableConds)
	naNow := time.Now()
	nonAvailableStatus := &api.AdapterStatus{
		ResourceType:       "NodePool",
		ResourceID:         nodePoolID,
		Adapter:            "hypershift",
		ObservedGeneration: 2,
		Conditions:         nonAvailableJSON,
		CreatedTime:        naNow,
		LastReportTime:     naNow,
	}
	result, svcErr := service.ProcessAdapterStatus(ctx, nodePoolID, nonAvailableStatus)
	g.Expect(svcErr).ToNot(BeNil())
	g.Expect(svcErr.HTTPCode).To(Equal(http.StatusBadRequest))
	g.Expect(svcErr.Reason).To(ContainSubstring("missing mandatory condition"))
	g.Expect(result).To(BeNil(), "Update missing mandatory conditions should be rejected")
	g.Expect(nodePoolDao.nodePools[nodePoolID].StatusConditions).To(Equal(prevStatus))

	// Available=Unknown is a no-op (does not store, does not overwrite nodepool conditions).
	prevStatus = api.NodePool{}.StatusConditions
	prevStatus = append(prevStatus, nodePoolDao.nodePools[nodePoolID].StatusConditions...)
	unknownConds := []api.AdapterCondition{
		{Type: api.ConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.ConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	unknownJSON, _ := json.Marshal(unknownConds)
	unknownNow := time.Now()
	unknownStatus := &api.AdapterStatus{
		ResourceType:       "NodePool",
		ResourceID:         nodePoolID,
		Adapter:            "hypershift",
		Conditions:         unknownJSON,
		ObservedGeneration: 2,
		CreatedTime:        unknownNow,
		LastReportTime:     unknownNow,
	}
	result, svcErr = service.ProcessAdapterStatus(ctx, nodePoolID, unknownStatus)
	g.Expect(svcErr).To(BeNil())
	g.Expect(result).To(BeNil())
	g.Expect(nodePoolDao.nodePools[nodePoolID].StatusConditions).To(Equal(prevStatus))
}

func TestNodePoolStaleAdapterStatusUpdatePolicy(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()

	adapterConfig := testNodePoolAdapterConfig()
	adapterConfig.Required.Nodepool = []string{"validation", "hypershift"}

	service := NewNodePoolService(nodePoolDao, adapterStatusDao, adapterConfig)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	nodePool := &api.NodePool{Generation: 2}
	nodePool.ID = nodePoolID
	_, svcErr := service.Create(ctx, nodePool)
	g.Expect(svcErr).To(BeNil())

	getAvailable := func() api.ResourceCondition {
		stored, getErr := nodePoolDao.Get(ctx, nodePoolID)
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
			ResourceType:       "NodePool",
			ResourceID:         nodePoolID,
			Adapter:            adapter,
			ObservedGeneration: observedGen,
			Conditions:         conditionsJSON,
			CreatedTime:        now,
			LastReportTime:     now,
		}

		_, err := service.ProcessAdapterStatus(ctx, nodePoolID, adapterStatus)
		g.Expect(err).To(BeNil())
	}

	// Current generation statuses => Available=True at observed_generation=2.
	upsert("validation", api.AdapterConditionTrue, 2)
	upsert("hypershift", api.AdapterConditionTrue, 2)
	available := getAvailable()
	g.Expect(available.Status).To(Equal(api.ConditionTrue))
	g.Expect(available.ObservedGeneration).To(Equal(int32(2)))

	// Stale True should not override newer True.
	upsert("validation", api.AdapterConditionTrue, 1)
	available = getAvailable()
	g.Expect(available.Status).To(Equal(api.ConditionTrue))
	g.Expect(available.ObservedGeneration).To(Equal(int32(2)))

	// Stale False is more restrictive and should override but we do not override newer generation responses
	upsert("validation", api.AdapterConditionFalse, 1)
	available = getAvailable()
	g.Expect(available.Status).To(Equal(api.ConditionTrue))
	g.Expect(available.ObservedGeneration).To(Equal(int32(2)))
}

func TestNodePoolSyntheticTimestampsStableWithoutAdapterStatus(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()

	adapterConfig := testNodePoolAdapterConfig()
	adapterConfig.Required.Nodepool = []string{"validation"}

	service := NewNodePoolService(nodePoolDao, adapterStatusDao, adapterConfig)

	ctx := context.Background()
	nodePoolID := testNodePoolID

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

	nodePool := &api.NodePool{
		Generation:       1,
		StatusConditions: initialConditionsJSON,
	}
	nodePool.ID = nodePoolID
	nodePool.CreatedTime = fixedNow
	nodePool.UpdatedTime = fixedNow
	created, svcErr := service.Create(ctx, nodePool)
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

	updated, err := service.UpdateNodePoolStatusFromAdapters(ctx, nodePoolID)
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

func TestNodePool_RequestDeletion_CascadesToAdapterStatuses(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()
	cfg := testNodePoolAdapterConfig()
	service := NewNodePoolService(nodePoolDao, adapterStatusDao, cfg)

	ctx := context.Background()
	nodePoolID := "np-cascade"

	nodePoolDao.nodePools[nodePoolID] = &api.NodePool{
		Meta:       api.Meta{ID: nodePoolID},
		Generation: 1,
	}

	adapterStatusDao.statuses["as-1"] = &api.AdapterStatus{
		Meta:         api.Meta{ID: "as-1"},
		ResourceType: "NodePool",
		ResourceID:   nodePoolID,
		Adapter:      "validation",
	}
	adapterStatusDao.statuses["as-2"] = &api.AdapterStatus{
		Meta:         api.Meta{ID: "as-2"},
		ResourceType: "NodePool",
		ResourceID:   nodePoolID,
		Adapter:      "hypershift",
	}

	np, svcErr := service.RequestDeletion(ctx, nodePoolID)
	g.Expect(svcErr).To(BeNil())
	g.Expect(np.DeletedAt).ToNot(BeNil())
	g.Expect(np.Generation).To(Equal(int32(2)))

	// Verify adapter statuses were soft-deleted
	g.Expect(adapterStatusDao.statuses["as-1"].DeletedAt).ToNot(BeNil())
	g.Expect(adapterStatusDao.statuses["as-2"].DeletedAt).ToNot(BeNil())
}

func TestNodePool_RequestDeletion_AlreadyDeleted_IdempotentCascade(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()
	cfg := testNodePoolAdapterConfig()
	service := NewNodePoolService(nodePoolDao, adapterStatusDao, cfg)

	ctx := context.Background()
	nodePoolID := "np-already-deleted"

	deletedAt := time.Now().Add(-time.Hour)
	nodePoolDao.nodePools[nodePoolID] = &api.NodePool{
		Meta:       api.Meta{ID: nodePoolID, DeletedAt: &deletedAt},
		Generation: 2,
	}

	// Adapter status already soft-deleted from the previous cascade
	adapterStatusDao.statuses["as-1"] = &api.AdapterStatus{
		Meta:         api.Meta{ID: "as-1", DeletedAt: &deletedAt},
		ResourceType: "NodePool",
		ResourceID:   nodePoolID,
		Adapter:      "validation",
	}

	np, svcErr := service.RequestDeletion(ctx, nodePoolID)
	g.Expect(svcErr).To(BeNil())
	g.Expect(np.DeletedAt).ToNot(BeNil())
	g.Expect(np.DeletedAt.Equal(deletedAt)).To(BeTrue())

	// Already-deleted adapter status retains its original DeletedAt
	g.Expect(adapterStatusDao.statuses["as-1"].DeletedAt.Equal(deletedAt)).To(BeTrue())
}

func TestNodePool_RequestDeletion_NotFound(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()
	cfg := testNodePoolAdapterConfig()
	service := NewNodePoolService(nodePoolDao, adapterStatusDao, cfg)

	ctx := context.Background()

	_, svcErr := service.RequestDeletion(ctx, "nonexistent")
	g.Expect(svcErr).ToNot(BeNil())
	g.Expect(svcErr.HTTPCode).To(Equal(404))
}
