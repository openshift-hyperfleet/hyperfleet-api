package services

import (
	"context"
	"encoding/json"
	"fmt"
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
	nodePools      map[string]*api.NodePool
	deleteErr      error
	findByOwnerErr error
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
	return nil, gorm.ErrRecordNotFound
}

func (d *mockNodePoolDao) GetByIDAndOwner(ctx context.Context, id string, ownerID string) (*api.NodePool, error) {
	np, err := d.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if np.OwnerID != ownerID {
		return nil, gorm.ErrRecordNotFound
	}
	return np, nil
}

func (d *mockNodePoolDao) GetForUpdate(ctx context.Context, id string) (*api.NodePool, error) {
	return d.Get(ctx, id)
}

func (d *mockNodePoolDao) SaveStatusConditions(ctx context.Context, id string, statusConditions []byte) error {
	if np, ok := d.nodePools[id]; ok {
		np.StatusConditions = statusConditions
		return nil
	}
	return gorm.ErrRecordNotFound
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

func (d *mockNodePoolDao) Save(ctx context.Context, nodePool *api.NodePool) error {
	d.nodePools[nodePool.ID] = nodePool
	return nil
}

func (d *mockNodePoolDao) Delete(ctx context.Context, id string) error {
	if d.deleteErr != nil {
		return d.deleteErr
	}
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

func (d *mockNodePoolDao) FindByOwner(ctx context.Context, ownerID string) (api.NodePoolList, error) {
	if d.findByOwnerErr != nil {
		return nil, d.findByOwnerErr
	}
	var result api.NodePoolList
	for _, np := range d.nodePools {
		if np.OwnerID == ownerID {
			result = append(result, np)
		}
	}
	return result, nil
}

func (d *mockNodePoolDao) SaveAll(ctx context.Context, nodePools api.NodePoolList) error {
	for _, np := range nodePools {
		d.nodePools[np.ID] = np
	}
	return nil
}

func (d *mockNodePoolDao) ExistsByOwner(ctx context.Context, ownerID string) (bool, error) {
	for _, np := range d.nodePools {
		if np.OwnerID == ownerID {
			return true, nil
		}
	}
	return false, nil
}

func (d *mockNodePoolDao) All(ctx context.Context) (api.NodePoolList, error) {
	var result api.NodePoolList
	for _, np := range d.nodePools {
		result = append(result, np)
	}
	return result, nil
}

var _ dao.NodePoolDao = &mockNodePoolDao{}

type mockGenericService struct {
	err       *errors.ServiceError
	nodePools []api.NodePool
}

func (m *mockGenericService) List(
	_ context.Context, _ *ListArguments, resourceList interface{},
) (*api.PagingMeta, *errors.ServiceError) {
	if m.err != nil {
		return nil, m.err
	}
	target := resourceList.(*[]api.NodePool)
	*target = m.nodePools
	return &api.PagingMeta{Page: 1, Size: int64(len(m.nodePools)), Total: int64(len(m.nodePools))}, nil
}

var _ GenericService = &mockGenericService{}

// TestNodePoolProcessAdapterStatus_FirstUnknownCondition tests that the first Unknown Available condition is stored
func TestNodePoolProcessAdapterStatus_FirstUnknownCondition(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := testNodePoolAdapterConfig()
	service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, config, nil)

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
			Type:               api.AdapterConditionTypeAvailable,
			Status:             api.AdapterConditionUnknown,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               api.AdapterConditionTypeApplied,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               api.AdapterConditionTypeHealth,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	adapterStatus := &api.AdapterStatus{
		ResourceType:       api.ResourceTypeNodePool,
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
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, api.ResourceTypeNodePool, nodePoolID)
	g.Expect(len(storedStatuses)).To(Equal(1), "First Unknown status should be stored")
}

// TestNodePoolProcessAdapterStatus_SubsequentUnknownCondition tests that subsequent Unknown conditions are discarded
func TestNodePoolProcessAdapterStatus_SubsequentUnknownCondition(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()

	config := testNodePoolAdapterConfig()
	service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, config, nil)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	now := time.Now()
	nodePoolDao.nodePools[nodePoolID] = &api.NodePool{
		Meta:       api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
		Generation: 1,
	}

	// Pre-populate an existing adapter status
	conditions := []api.AdapterCondition{
		{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
		{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	existingStatus := &api.AdapterStatus{
		ResourceType:       api.ResourceTypeNodePool,
		ResourceID:         nodePoolID,
		Adapter:            "test-adapter",
		Conditions:         conditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}
	_, _ = adapterStatusDao.Upsert(ctx, existingStatus, nil)

	// Now send another Unknown status report
	newAdapterStatus := &api.AdapterStatus{
		ResourceType:       api.ResourceTypeNodePool,
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
	service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, config, nil)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	now := time.Now()
	nodePoolDao.nodePools[nodePoolID] = &api.NodePool{
		Meta:       api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
		Generation: 1,
	}

	conditions := []api.AdapterCondition{
		{Type: api.AdapterConditionTypeAvailable, Status: "Pending", LastTransitionTime: time.Now()},
		{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	conditionsJSON, _ := json.Marshal(conditions)
	adapterStatus := &api.AdapterStatus{
		ResourceType:       api.ResourceTypeNodePool,
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
	service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, config, nil)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	now := time.Now()
	nodePoolDao.nodePools[nodePoolID] = &api.NodePool{
		Meta:       api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
		Generation: 1,
	}

	conditions := []api.AdapterCondition{
		{Type: api.AdapterConditionTypeAvailable, Status: "", LastTransitionTime: time.Now()},
		{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	conditionsJSON, _ := json.Marshal(conditions)
	adapterStatus := &api.AdapterStatus{
		ResourceType:       api.ResourceTypeNodePool,
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
	service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, config, nil)

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
			Type:               api.AdapterConditionTypeAvailable,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               api.AdapterConditionTypeApplied,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               api.AdapterConditionTypeHealth,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	now := time.Now()
	adapterStatus := &api.AdapterStatus{
		ResourceType:   api.ResourceTypeNodePool,
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
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, api.ResourceTypeNodePool, nodePoolID)
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
	service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, config, nil)

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
			Type:               api.AdapterConditionTypeAvailable,
			Status:             api.AdapterConditionUnknown,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               api.AdapterConditionTypeApplied,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               api.AdapterConditionTypeHealth,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
		{
			Type:               api.AdapterConditionTypeReconciled,
			Status:             api.AdapterConditionTrue,
			LastTransitionTime: time.Now(),
		},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	adapterStatus := &api.AdapterStatus{
		ResourceType:       api.ResourceTypeNodePool,
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
	storedStatuses, _ := adapterStatusDao.FindByResource(ctx, api.ResourceTypeNodePool, nodePoolID)
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
	service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, config, nil)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	now := time.Now()
	nodePoolDao.nodePools[nodePoolID] = &api.NodePool{
		Meta:       api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
		Generation: 1,
	}

	// Pre-populate an existing adapter status
	existingConditions := []api.AdapterCondition{
		{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
		{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	existingConditionsJSON, _ := json.Marshal(existingConditions)

	existingStatus := &api.AdapterStatus{
		ResourceType:       api.ResourceTypeNodePool,
		ResourceID:         nodePoolID,
		Adapter:            "test-adapter",
		Conditions:         existingConditionsJSON,
		ObservedGeneration: 1,
		CreatedTime:        now,
		LastReportTime:     now,
	}
	_, _ = adapterStatusDao.Upsert(ctx, existingStatus, nil)

	// Now send another report with multiple conditions including Available=Unknown
	conditions := []api.AdapterCondition{
		{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
		{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.AdapterConditionTypeReconciled, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: "Progressing", Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	conditionsJSON, _ := json.Marshal(conditions)

	adapterStatus := &api.AdapterStatus{
		ResourceType:       api.ResourceTypeNodePool,
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

func TestNodePoolAvailableReconciledTransitions(t *testing.T) {
	t.Parallel()
	g := NewWithT(t)

	nodePoolDao := newMockNodePoolDao()
	adapterStatusDao := newMockAdapterStatusDao()

	adapterConfig := testNodePoolAdapterConfig()
	adapterConfig.Required.Nodepool = []string{"validation", "hypershift"}

	service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, adapterConfig, nil)

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

		var available, reconciled *api.ResourceCondition
		for i := range conds {
			switch conds[i].Type {
			case api.ResourceConditionTypeLastKnownReconciled:
				available = &conds[i]
			case api.ResourceConditionTypeReconciled:
				reconciled = &conds[i]
			}
		}
		g.Expect(reconciled).ToNot(BeNil())
		g.Expect(available).ToNot(BeNil())
		return *available, *reconciled
	}

	upsert := func(adapter string, available api.AdapterConditionStatus, observedGen int32) {
		conditions := []api.AdapterCondition{
			{Type: api.AdapterConditionTypeAvailable, Status: available, LastTransitionTime: time.Now()},
			{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
			{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		}
		conditionsJSON, _ := json.Marshal(conditions)
		now := time.Now()

		adapterStatus := &api.AdapterStatus{
			ResourceType:       api.ResourceTypeNodePool,
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
	avail, reconciled := getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionFalse))
	g.Expect(avail.ObservedGeneration).To(Equal(int32(1)))
	g.Expect(reconciled.Status).To(Equal(api.ConditionFalse))
	g.Expect(reconciled.ObservedGeneration).To(Equal(int32(1)))

	// Partial adapters: still not Available/Reconciled.
	upsert("validation", api.AdapterConditionTrue, 1)
	avail, reconciled = getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionFalse))
	g.Expect(reconciled.Status).To(Equal(api.ConditionFalse))

	// All required adapters available at gen=1 => Available=True, Reconciled=True.
	upsert("hypershift", api.AdapterConditionTrue, 1)
	avail, reconciled = getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionTrue))
	g.Expect(avail.ObservedGeneration).To(Equal(int32(1)))
	g.Expect(reconciled.Status).To(Equal(api.ConditionTrue))

	// Bump resource generation => Reconciled flips to False; Available remains True.
	nodePoolDao.nodePools[nodePoolID].Generation = 2
	_, err = service.UpdateNodePoolStatusFromAdapters(ctx, nodePoolID)
	g.Expect(err).To(BeNil())
	avail, reconciled = getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionTrue))
	g.Expect(avail.ObservedGeneration).To(Equal(int32(1)))
	g.Expect(reconciled.Status).To(Equal(api.ConditionFalse))
	g.Expect(reconciled.ObservedGeneration).To(Equal(int32(2)))

	// One adapter updates to gen=2 => Reconciled still False; Available still True (minObservedGeneration still 1).
	upsert("validation", api.AdapterConditionTrue, 2)
	avail, reconciled = getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionTrue))
	g.Expect(avail.ObservedGeneration).To(Equal(int32(1)))
	g.Expect(reconciled.Status).To(Equal(api.ConditionFalse))

	// All required adapters at gen=2 => Reconciled becomes True, Available minObservedGeneration becomes 2.
	upsert("hypershift", api.AdapterConditionTrue, 2)
	avail, reconciled = getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionTrue))
	g.Expect(avail.ObservedGeneration).To(Equal(int32(2)))
	g.Expect(reconciled.Status).To(Equal(api.ConditionTrue))

	// One required adapter goes False => both Available and Reconciled become False.
	upsert("hypershift", api.AdapterConditionFalse, 2)
	avail, reconciled = getSynth()
	g.Expect(avail.Status).To(Equal(api.ConditionFalse))
	g.Expect(avail.ObservedGeneration).To(Equal(int32(2)))
	g.Expect(reconciled.Status).To(Equal(api.ConditionFalse))

	// Adapter status missing mandatory conditions should be rejected and not overwrite synthetic conditions.
	prevStatus := api.NodePool{}.StatusConditions
	prevStatus = append(prevStatus, nodePoolDao.nodePools[nodePoolID].StatusConditions...)
	nonAvailableConds := []api.AdapterCondition{
		{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	nonAvailableJSON, _ := json.Marshal(nonAvailableConds)
	naNow := time.Now()
	nonAvailableStatus := &api.AdapterStatus{
		ResourceType:       api.ResourceTypeNodePool,
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
		{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionUnknown, LastTransitionTime: time.Now()},
		{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
	}
	unknownJSON, _ := json.Marshal(unknownConds)
	unknownNow := time.Now()
	unknownStatus := &api.AdapterStatus{
		ResourceType:       api.ResourceTypeNodePool,
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

	service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, adapterConfig, nil)

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
			if conds[i].Type == api.ResourceConditionTypeLastKnownReconciled {
				return conds[i]
			}
		}
		g.Expect(true).To(BeFalse(), "Available condition not found")
		return api.ResourceCondition{}
	}

	upsert := func(adapter string, available api.AdapterConditionStatus, observedGen int32) {
		conditions := []api.AdapterCondition{
			{Type: api.AdapterConditionTypeAvailable, Status: available, LastTransitionTime: time.Now()},
			{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
			{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		}
		conditionsJSON, _ := json.Marshal(conditions)
		now := time.Now()

		adapterStatus := &api.AdapterStatus{
			ResourceType:       api.ResourceTypeNodePool,
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

	service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, adapterConfig, nil)

	ctx := context.Background()
	nodePoolID := testNodePoolID

	fixedNow := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	initialConditions := []api.ResourceCondition{
		{
			Type:               api.ResourceConditionTypeLastKnownReconciled,
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

	var createdAvailable, createdReconciled *api.ResourceCondition
	for i := range createdConds {
		switch createdConds[i].Type {
		case api.ResourceConditionTypeLastKnownReconciled:
			createdAvailable = &createdConds[i]
		case api.ResourceConditionTypeReconciled:
			createdReconciled = &createdConds[i]
		}
	}
	g.Expect(createdAvailable).ToNot(BeNil())
	g.Expect(createdReconciled).ToNot(BeNil())
	g.Expect(createdAvailable.CreatedTime).To(Equal(fixedNow))
	g.Expect(createdAvailable.LastTransitionTime).To(Equal(fixedNow))
	g.Expect(createdAvailable.LastUpdatedTime).To(Equal(fixedNow))
	g.Expect(createdReconciled.CreatedTime).To(Equal(fixedNow))
	g.Expect(createdReconciled.LastTransitionTime).To(Equal(fixedNow))
	g.Expect(createdReconciled.LastUpdatedTime).To(Equal(fixedNow))

	updated, err := service.UpdateNodePoolStatusFromAdapters(ctx, nodePoolID)
	g.Expect(err).To(BeNil())

	var updatedConds []api.ResourceCondition
	g.Expect(json.Unmarshal(updated.StatusConditions, &updatedConds)).To(Succeed())
	g.Expect(len(updatedConds)).To(BeNumerically(">=", 2))

	var updatedAvailable, updatedReconciled *api.ResourceCondition
	for i := range updatedConds {
		switch updatedConds[i].Type {
		case api.ResourceConditionTypeLastKnownReconciled:
			updatedAvailable = &updatedConds[i]
		case api.ResourceConditionTypeReconciled:
			updatedReconciled = &updatedConds[i]
		}
	}
	g.Expect(updatedAvailable).ToNot(BeNil())
	g.Expect(updatedReconciled).ToNot(BeNil())
	g.Expect(updatedAvailable.CreatedTime).To(Equal(fixedNow))
	g.Expect(updatedAvailable.LastTransitionTime).To(Equal(fixedNow))
	g.Expect(updatedAvailable.LastUpdatedTime).To(Equal(fixedNow))
	g.Expect(updatedReconciled.CreatedTime).To(Equal(fixedNow))
	g.Expect(updatedReconciled.LastTransitionTime).To(Equal(fixedNow))
	g.Expect(updatedReconciled.LastUpdatedTime).To(Equal(fixedNow))
}

func TestNodePoolSoftDelete(t *testing.T) {
	t.Run("given a live nodepool, when soft-deleted, then deleted_time/deleted_by/generation are set", func(t *testing.T) {
		g := NewWithT(t)
		// Given:
		nodePoolDao := newMockNodePoolDao()
		adapterStatusDao := newMockAdapterStatusDao()
		service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, testNodePoolAdapterConfig(), nil)
		ctx := context.Background()
		nodePoolID := testNodePoolID
		nodePoolDao.nodePools[nodePoolID] = &api.NodePool{
			Meta:       api.Meta{ID: nodePoolID},
			Generation: 1,
		}
		// When:
		nodePool, svcErr := service.SoftDelete(ctx, nodePoolID)
		// Then:
		g.Expect(svcErr).To(BeNil())
		g.Expect(nodePool.DeletedTime).NotTo(BeNil())
		g.Expect(nodePool.DeletedBy).NotTo(BeNil())
		g.Expect(*nodePool.DeletedBy).To(Equal(systemActor))
		g.Expect(nodePool.Generation).To(Equal(int32(2)))
	})

	t.Run("given an already-deleted nodepool, when soft-deleted again, then deleted_time and generation are unchanged", func(t *testing.T) { //nolint:lll
		g := NewWithT(t)
		// Given:
		nodePoolDao := newMockNodePoolDao()
		adapterStatusDao := newMockAdapterStatusDao()
		service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, testNodePoolAdapterConfig(), nil)
		ctx := context.Background()
		nodePoolID := testNodePoolID
		originalTime := time.Now().Add(-time.Hour)
		nodePoolDao.nodePools[nodePoolID] = &api.NodePool{
			Meta:        api.Meta{ID: nodePoolID},
			DeletedTime: &originalTime,
			Generation:  3,
		}
		// When:
		nodePool, svcErr := service.SoftDelete(ctx, nodePoolID)
		// Then:
		g.Expect(svcErr).To(BeNil())
		g.Expect(nodePool.DeletedTime.Equal(originalTime)).To(BeTrue())
		g.Expect(nodePool.Generation).To(Equal(int32(3)))
	})

	t.Run("given a non-existent nodepool ID, when soft-deleted, then returns 404 service error", func(t *testing.T) {
		g := NewWithT(t)
		// Given:
		nodePoolDao := newMockNodePoolDao()
		adapterStatusDao := newMockAdapterStatusDao()
		service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, testNodePoolAdapterConfig(), nil)
		ctx := context.Background()
		// When:
		_, svcErr := service.SoftDelete(ctx, "nonexistent")
		// Then:
		g.Expect(svcErr).NotTo(BeNil())
		g.Expect(svcErr.HTTPCode).To(Equal(404))
	})

	t.Run("given a nodepool with Reconciled=True, when soft-deleted, then Reconciled flips to False due to generation bump", func(t *testing.T) { //nolint:lll
		g := NewWithT(t)
		// Given:
		nodePoolDao := newMockNodePoolDao()
		adapterStatusDao := newMockAdapterStatusDao()
		adapterConfig := testNodePoolAdapterConfig()
		adapterConfig.Required.Nodepool = []string{"validation"}
		service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, adapterConfig, nil)
		ctx := context.Background()
		nodePoolID := "reconciled-nodepool"

		nodePoolDao.nodePools[nodePoolID] = &api.NodePool{Meta: api.Meta{ID: nodePoolID}, Generation: 1}
		conditions := []api.AdapterCondition{
			{Type: api.AdapterConditionTypeAvailable, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
			{Type: api.AdapterConditionTypeApplied, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
			{Type: api.AdapterConditionTypeHealth, Status: api.AdapterConditionTrue, LastTransitionTime: time.Now()},
		}
		condJSON, _ := json.Marshal(conditions)
		now := time.Now()
		_, svcErr := service.ProcessAdapterStatus(ctx, nodePoolID, &api.AdapterStatus{
			ResourceType: api.ResourceTypeNodePool, ResourceID: nodePoolID, Adapter: "validation",
			ObservedGeneration: 1, Conditions: condJSON, CreatedTime: now, LastReportTime: now,
		})
		g.Expect(svcErr).To(BeNil())

		// Pre-condition: Reconciled=True before soft-delete
		stored, _ := nodePoolDao.Get(ctx, nodePoolID)
		var preConds []api.ResourceCondition
		g.Expect(json.Unmarshal(stored.StatusConditions, &preConds)).To(Succeed())
		var preReconciled *api.ResourceCondition
		for i := range preConds {
			if preConds[i].Type == api.ResourceConditionTypeReconciled {
				preReconciled = &preConds[i]
			}
		}
		g.Expect(preReconciled).NotTo(BeNil())
		g.Expect(preReconciled.Status).To(Equal(api.ConditionTrue))

		// When:
		_, svcErr = service.SoftDelete(ctx, nodePoolID)
		g.Expect(svcErr).To(BeNil())

		// Then: generation bumped to 2, Reconciled must flip to False
		stored, _ = nodePoolDao.Get(ctx, nodePoolID)
		g.Expect(stored.Generation).To(Equal(int32(2)))
		var postConds []api.ResourceCondition
		g.Expect(json.Unmarshal(stored.StatusConditions, &postConds)).To(Succeed())
		var postReconciled *api.ResourceCondition
		for i := range postConds {
			if postConds[i].Type == api.ResourceConditionTypeReconciled {
				postReconciled = &postConds[i]
			}
		}
		g.Expect(postReconciled).NotTo(BeNil())
		g.Expect(postReconciled.Status).To(Equal(api.ConditionFalse))
		g.Expect(postReconciled.ObservedGeneration).To(Equal(int32(2)))
	})
}

type nodePoolTestEnv struct {
	nodePoolDao      *mockNodePoolDao
	adapterStatusDao *mockAdapterStatusDao
	service          NodePoolService
	ctx              context.Context
}

func newNodePoolTestEnv() *nodePoolTestEnv {
	npDao := newMockNodePoolDao()
	asDao := newMockAdapterStatusDao()
	return &nodePoolTestEnv{
		nodePoolDao:      npDao,
		adapterStatusDao: asDao,
		service:          NewNodePoolService(npDao, nil, asDao, testNodePoolAdapterConfig(), nil),
		ctx:              context.Background(),
	}
}

func TestNodePoolForceDelete(t *testing.T) {
	t.Parallel()
	deletedTime := time.Now().Add(-time.Hour)

	t.Run("nodepool not found returns 404", func(t *testing.T) {
		g := NewWithT(t)
		env := newNodePoolTestEnv()

		svcErr := env.service.ForceDelete(env.ctx, "nonexistent", "testing")

		g.Expect(svcErr).NotTo(BeNil())
		g.Expect(svcErr.HTTPCode).To(Equal(http.StatusNotFound))
	})

	t.Run("nodepool not in Finalizing state returns 409", func(t *testing.T) {
		g := NewWithT(t)
		env := newNodePoolTestEnv()
		env.nodePoolDao.nodePools[testNodePoolID] = &api.NodePool{
			Meta:       api.Meta{ID: testNodePoolID},
			Generation: 1,
		}

		svcErr := env.service.ForceDelete(env.ctx, testNodePoolID, "testing")

		g.Expect(svcErr).NotTo(BeNil())
		g.Expect(svcErr.HTTPCode).To(Equal(http.StatusConflict))
	})

	t.Run("soft-deleted nodepool is removed along with adapter statuses", func(t *testing.T) {
		g := NewWithT(t)
		env := newNodePoolTestEnv()
		env.nodePoolDao.nodePools[testNodePoolID] = &api.NodePool{
			Meta:        api.Meta{ID: testNodePoolID},
			Generation:  2,
			DeletedTime: &deletedTime,
		}
		env.adapterStatusDao.statuses["status-1"] = &api.AdapterStatus{
			Meta:         api.Meta{ID: "status-1"},
			ResourceType: api.ResourceTypeNodePool,
			ResourceID:   testNodePoolID,
			Adapter:      "validation",
		}

		svcErr := env.service.ForceDelete(env.ctx, testNodePoolID, "stuck in finalizing")

		g.Expect(svcErr).To(BeNil())
		_, err := env.nodePoolDao.Get(env.ctx, testNodePoolID)
		g.Expect(err).To(Equal(gorm.ErrRecordNotFound))
		g.Expect(env.adapterStatusDao.statuses).To(BeEmpty())
	})

	t.Run("adapter status fetch failure returns 500", func(t *testing.T) {
		g := NewWithT(t)
		env := newNodePoolTestEnv()
		env.adapterStatusDao.findByResourceErr = fmt.Errorf("db connection lost")
		env.nodePoolDao.nodePools[testNodePoolID] = &api.NodePool{
			Meta:        api.Meta{ID: testNodePoolID},
			DeletedTime: &deletedTime,
		}

		svcErr := env.service.ForceDelete(env.ctx, testNodePoolID, "testing")

		g.Expect(svcErr).NotTo(BeNil())
		g.Expect(svcErr.HTTPCode).To(Equal(http.StatusInternalServerError))
	})

	t.Run("adapter status deletion failure returns 500", func(t *testing.T) {
		g := NewWithT(t)
		env := newNodePoolTestEnv()
		env.adapterStatusDao.deleteByResourceErr = fmt.Errorf("db connection lost")
		env.nodePoolDao.nodePools[testNodePoolID] = &api.NodePool{
			Meta:        api.Meta{ID: testNodePoolID},
			DeletedTime: &deletedTime,
		}

		svcErr := env.service.ForceDelete(env.ctx, testNodePoolID, "testing")

		g.Expect(svcErr).NotTo(BeNil())
		g.Expect(svcErr.HTTPCode).To(Equal(http.StatusInternalServerError))
	})

	t.Run("nodepool deletion failure returns 500", func(t *testing.T) {
		g := NewWithT(t)
		env := newNodePoolTestEnv()
		env.nodePoolDao.deleteErr = fmt.Errorf("db connection lost")
		env.nodePoolDao.nodePools[testNodePoolID] = &api.NodePool{
			Meta:        api.Meta{ID: testNodePoolID},
			DeletedTime: &deletedTime,
		}

		svcErr := env.service.ForceDelete(env.ctx, testNodePoolID, "testing")

		g.Expect(svcErr).NotTo(BeNil())
		g.Expect(svcErr.HTTPCode).To(Equal(http.StatusInternalServerError))
	})
}

func TestNodePoolPatch(t *testing.T) {
	t.Parallel()
	t.Run("spec changed increments generation", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		nodePoolDao := newMockNodePoolDao()
		adapterStatusDao := newMockAdapterStatusDao()
		adapterConfig := testNodePoolAdapterConfig()
		adapterConfig.Required.Nodepool = []string{}
		service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, adapterConfig, nil)
		ctx := context.Background()

		nodePoolDao.nodePools["np1"] = &api.NodePool{
			Meta:       api.Meta{ID: "np1"},
			Spec:       []byte(`{"old":"spec"}`),
			Labels:     []byte(`{}`),
			Generation: 1,
		}

		newSpec := map[string]interface{}{"new": "spec"}
		result, svcErr := service.Patch(ctx, "np1", &api.NodePoolPatch{Spec: newSpec})

		g.Expect(svcErr).To(BeNil())
		g.Expect(result.Generation).To(Equal(int32(2)))
	})

	t.Run("spec unchanged keeps generation", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		nodePoolDao := newMockNodePoolDao()
		adapterStatusDao := newMockAdapterStatusDao()
		adapterConfig := testNodePoolAdapterConfig()
		adapterConfig.Required.Nodepool = []string{}
		service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, adapterConfig, nil)
		ctx := context.Background()

		nodePoolDao.nodePools["np1"] = &api.NodePool{
			Meta:       api.Meta{ID: "np1"},
			Spec:       []byte(`{"key":"value"}`),
			Labels:     []byte(`{}`),
			Generation: 3,
		}

		sameSpec := map[string]interface{}{"key": "value"}
		result, svcErr := service.Patch(ctx, "np1", &api.NodePoolPatch{Spec: sameSpec})

		g.Expect(svcErr).To(BeNil())
		g.Expect(result.Generation).To(Equal(int32(3)))
	})

	t.Run("labels changed increments generation", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		nodePoolDao := newMockNodePoolDao()
		adapterStatusDao := newMockAdapterStatusDao()
		adapterConfig := testNodePoolAdapterConfig()
		adapterConfig.Required.Nodepool = []string{}
		service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, adapterConfig, nil)
		ctx := context.Background()

		nodePoolDao.nodePools["np1"] = &api.NodePool{
			Meta:       api.Meta{ID: "np1"},
			Spec:       []byte(`{}`),
			Labels:     []byte(`{"env":"dev"}`),
			Generation: 1,
		}

		newLabels := map[string]string{"env": "prod"}
		result, svcErr := service.Patch(ctx, "np1", &api.NodePoolPatch{Labels: newLabels})

		g.Expect(svcErr).To(BeNil())
		g.Expect(result.Generation).To(Equal(int32(2)))
	})

	t.Run("spec unchanged with different key order keeps generation", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		nodePoolDao := newMockNodePoolDao()
		adapterStatusDao := newMockAdapterStatusDao()
		adapterConfig := testNodePoolAdapterConfig()
		adapterConfig.Required.Nodepool = []string{}
		service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, adapterConfig, nil)
		ctx := context.Background()

		nodePoolDao.nodePools["np1"] = &api.NodePool{
			Meta:       api.Meta{ID: "np1"},
			Spec:       []byte(`{"z":"last","a":"first","m":"middle"}`),
			Labels:     []byte(`{}`),
			Generation: 5,
		}

		sameSpec := map[string]interface{}{"z": "last", "a": "first", "m": "middle"}
		result, svcErr := service.Patch(ctx, "np1", &api.NodePoolPatch{Spec: sameSpec})

		g.Expect(svcErr).To(BeNil())
		g.Expect(result.Generation).To(Equal(int32(5)))
	})

	t.Run("labels unchanged with different key order keeps generation", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		nodePoolDao := newMockNodePoolDao()
		adapterStatusDao := newMockAdapterStatusDao()
		adapterConfig := testNodePoolAdapterConfig()
		adapterConfig.Required.Nodepool = []string{}
		service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, adapterConfig, nil)
		ctx := context.Background()

		nodePoolDao.nodePools["np1"] = &api.NodePool{
			Meta:       api.Meta{ID: "np1"},
			Spec:       []byte(`{}`),
			Labels:     []byte(`{"z":"zulu","a":"alpha"}`),
			Generation: 4,
		}

		sameLabels := map[string]string{"z": "zulu", "a": "alpha"}
		result, svcErr := service.Patch(ctx, "np1", &api.NodePoolPatch{Labels: sameLabels})

		g.Expect(svcErr).To(BeNil())
		g.Expect(result.Generation).To(Equal(int32(4)))
	})

	t.Run("not found returns 404", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		nodePoolDao := newMockNodePoolDao()
		adapterStatusDao := newMockAdapterStatusDao()
		service := NewNodePoolService(nodePoolDao, nil, adapterStatusDao, testNodePoolAdapterConfig(), nil)
		ctx := context.Background()

		newSpec := map[string]interface{}{"a": "b"}
		_, svcErr := service.Patch(ctx, "nonexistent", &api.NodePoolPatch{Spec: newSpec})

		g.Expect(svcErr).NotTo(BeNil())
		g.Expect(svcErr.HTTPCode).To(Equal(404))
	})
}

func TestGetByIDAndOwner(t *testing.T) {
	t.Parallel()
	t.Run("happy path returns nodepool", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		nodePoolDao := newMockNodePoolDao()
		service := NewNodePoolService(nodePoolDao, nil, newMockAdapterStatusDao(), testNodePoolAdapterConfig(), nil)
		ctx := context.Background()

		nodePoolDao.nodePools["np1"] = &api.NodePool{
			Meta:    api.Meta{ID: "np1"},
			OwnerID: "cluster1",
		}

		result, svcErr := service.GetByIDAndOwner(ctx, "np1", "cluster1")
		g.Expect(svcErr).To(BeNil())
		g.Expect(result.ID).To(Equal("np1"))
	})

	t.Run("owner mismatch returns 404", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		nodePoolDao := newMockNodePoolDao()
		service := NewNodePoolService(nodePoolDao, nil, newMockAdapterStatusDao(), testNodePoolAdapterConfig(), nil)
		ctx := context.Background()

		nodePoolDao.nodePools["np1"] = &api.NodePool{
			Meta:    api.Meta{ID: "np1"},
			OwnerID: "cluster1",
		}

		_, svcErr := service.GetByIDAndOwner(ctx, "np1", "wrong-cluster")
		g.Expect(svcErr).NotTo(BeNil())
		g.Expect(svcErr.HTTPCode).To(Equal(404))
	})

	t.Run("nodepool not found returns 404", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		nodePoolDao := newMockNodePoolDao()
		service := NewNodePoolService(nodePoolDao, nil, newMockAdapterStatusDao(), testNodePoolAdapterConfig(), nil)
		ctx := context.Background()

		_, svcErr := service.GetByIDAndOwner(ctx, "nonexistent", "cluster1")
		g.Expect(svcErr).NotTo(BeNil())
		g.Expect(svcErr.HTTPCode).To(Equal(404))
	})
}

func TestListByCluster(t *testing.T) {
	t.Parallel()
	testClusterUUID := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd380a11"

	t.Run("happy path returns nodepools for cluster", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		nodePoolDao := newMockNodePoolDao()
		clusterDao := newMockClusterDao()
		genericSvc := &mockGenericService{
			nodePools: []api.NodePool{
				{Meta: api.Meta{ID: "np1"}, OwnerID: testClusterUUID},
				{Meta: api.Meta{ID: "np2"}, OwnerID: testClusterUUID},
			},
		}
		service := NewNodePoolService(nodePoolDao,
			clusterDao,
			newMockAdapterStatusDao(),
			testNodePoolAdapterConfig(),
			genericSvc,
		)
		ctx := context.Background()

		clusterDao.clusters[testClusterUUID] = &api.Cluster{Meta: api.Meta{ID: testClusterUUID}}

		args := &ListArguments{Page: 1, Size: 100}
		result, paging, svcErr := service.ListByCluster(ctx, testClusterUUID, args)

		g.Expect(svcErr).To(BeNil())
		g.Expect(result).To(HaveLen(2))
		g.Expect(result[0].ID).To(Equal("np1"))
		g.Expect(result[1].ID).To(Equal("np2"))
		g.Expect(paging.Total).To(Equal(int64(2)))
	})

	t.Run("cluster not found returns 404", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		nodePoolDao := newMockNodePoolDao()
		clusterDao := newMockClusterDao()
		genericSvc := &mockGenericService{}
		service := NewNodePoolService(nodePoolDao,
			clusterDao,
			newMockAdapterStatusDao(),
			testNodePoolAdapterConfig(),
			genericSvc,
		)
		ctx := context.Background()

		nonexistentUUID := "b1ffbc99-9c0b-4ef8-bb6d-6bb9bd380a22"
		args := &ListArguments{Page: 1, Size: 100}
		_, _, svcErr := service.ListByCluster(ctx, nonexistentUUID, args)

		g.Expect(svcErr).NotTo(BeNil())
		g.Expect(svcErr.HTTPCode).To(Equal(404))
	})

	t.Run("existing search is preserved and ANDed with owner_id", func(t *testing.T) {
		t.Parallel()
		g := NewWithT(t)
		nodePoolDao := newMockNodePoolDao()
		clusterDao := newMockClusterDao()
		genericSvc := &mockGenericService{}
		service := NewNodePoolService(nodePoolDao,
			clusterDao,
			newMockAdapterStatusDao(),
			testNodePoolAdapterConfig(),
			genericSvc,
		)
		ctx := context.Background()

		clusterDao.clusters[testClusterUUID] = &api.Cluster{Meta: api.Meta{ID: testClusterUUID}}

		args := &ListArguments{Page: 1, Size: 100, Search: "name = 'test'"}
		_, _, svcErr := service.ListByCluster(ctx, testClusterUUID, args)

		g.Expect(svcErr).To(BeNil())
		g.Expect(args.Search).To(ContainSubstring("name = 'test'"))
		g.Expect(args.Search).To(ContainSubstring("AND owner_id = '" + testClusterUUID + "'"))
	})
}
