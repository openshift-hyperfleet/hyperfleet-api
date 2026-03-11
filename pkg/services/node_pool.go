package services

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"gorm.io/gorm"
)

//go:generate mockgen-v0.6.0 -source=node_pool.go -package=services -destination=node_pool_mock.go

type NodePoolService interface {
	Get(ctx context.Context, id string) (*api.NodePool, *errors.ServiceError)
	Create(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, *errors.ServiceError)
	Replace(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (api.NodePoolList, *errors.ServiceError)

	FindByIDs(ctx context.Context, ids []string) (api.NodePoolList, *errors.ServiceError)

	// Status aggregation
	UpdateNodePoolStatusFromAdapters(ctx context.Context, nodePoolID string) (*api.NodePool, *errors.ServiceError)

	// ProcessAdapterStatus handles the business logic for adapter status:
	// - First report: accepts Unknown Available condition, skips aggregation
	// - Subsequent reports: rejects Unknown Available condition
	// - Otherwise: upserts the status and triggers aggregation
	ProcessAdapterStatus(
		ctx context.Context, nodePoolID string, adapterStatus *api.AdapterStatus,
	) (*api.AdapterStatus, *errors.ServiceError)

	// idempotent functions for the control plane, but can also be called synchronously by any actor
	OnUpsert(ctx context.Context, id string) error
	OnDelete(ctx context.Context, id string) error
}

func NewNodePoolService(
	nodePoolDao dao.NodePoolDao,
	adapterStatusDao dao.AdapterStatusDao,
	adapterConfig *config.AdapterRequirementsConfig,
) NodePoolService {
	return &sqlNodePoolService{
		nodePoolDao:      nodePoolDao,
		adapterStatusDao: adapterStatusDao,
		adapterConfig:    adapterConfig,
	}
}

var _ NodePoolService = &sqlNodePoolService{}

type sqlNodePoolService struct {
	nodePoolDao      dao.NodePoolDao
	adapterStatusDao dao.AdapterStatusDao
	adapterConfig    *config.AdapterRequirementsConfig
}

func (s *sqlNodePoolService) Get(ctx context.Context, id string) (*api.NodePool, *errors.ServiceError) {
	nodePool, err := s.nodePoolDao.Get(ctx, id)
	if err != nil {
		return nil, handleGetError("NodePool", "id", id, err)
	}
	return nodePool, nil
}

func (s *sqlNodePoolService) Create(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, *errors.ServiceError) {
	if nodePool.Generation == 0 {
		nodePool.Generation = 1
	}

	nodePool, err := s.nodePoolDao.Create(ctx, nodePool)
	if err != nil {
		return nil, handleCreateError("NodePool", err)
	}

	updatedNodePool, svcErr := s.UpdateNodePoolStatusFromAdapters(ctx, nodePool.ID)
	if svcErr != nil {
		return nil, svcErr
	}

	// REMOVED: Event creation - no event-driven components
	return updatedNodePool, nil
}

func (s *sqlNodePoolService) Replace(
	ctx context.Context, nodePool *api.NodePool,
) (*api.NodePool, *errors.ServiceError) {
	nodePool, err := s.nodePoolDao.Replace(ctx, nodePool)
	if err != nil {
		return nil, handleUpdateError("NodePool", err)
	}

	updatedNodePool, svcErr := s.UpdateNodePoolStatusFromAdapters(ctx, nodePool.ID)
	if svcErr != nil {
		return nil, svcErr
	}

	return updatedNodePool, nil
}

func (s *sqlNodePoolService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.nodePoolDao.Delete(ctx, id); err != nil {
		return handleDeleteError("NodePool", errors.GeneralError("Unable to delete nodePool: %s", err))
	}

	// REMOVED: Event creation - no event-driven components
	return nil
}

func (s *sqlNodePoolService) FindByIDs(ctx context.Context, ids []string) (api.NodePoolList, *errors.ServiceError) {
	nodePools, err := s.nodePoolDao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all nodePools: %s", err)
	}
	return nodePools, nil
}

func (s *sqlNodePoolService) All(ctx context.Context) (api.NodePoolList, *errors.ServiceError) {
	nodePools, err := s.nodePoolDao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all nodePools: %s", err)
	}
	return nodePools, nil
}

func (s *sqlNodePoolService) OnUpsert(ctx context.Context, id string) error {
	nodePool, err := s.nodePoolDao.Get(ctx, id)
	if err != nil {
		return err
	}

	logger.With(ctx, logger.FieldNodePoolID, nodePool.ID).
		Info("Perform idempotent operations on node pool")

	return nil
}

func (s *sqlNodePoolService) OnDelete(ctx context.Context, id string) error {
	logger.With(ctx, logger.FieldNodePoolID, id).Info("Node pool has been deleted")
	return nil
}

// UpdateNodePoolStatusFromAdapters aggregates adapter statuses into nodepool status.
// Uses time.Now() as the observed time (for generation-change recomputations).
// Called from Create/Replace, so isLifecycleChange=true (Available frozen, Ready resets).
func (s *sqlNodePoolService) UpdateNodePoolStatusFromAdapters(
	ctx context.Context, nodePoolID string,
) (*api.NodePool, *errors.ServiceError) {
	return s.updateNodePoolStatusFromAdapters(ctx, nodePoolID, time.Now(), true)
}

// updateNodePoolStatusFromAdapters is the internal implementation.
// observedTime is the triggering adapter's observed_time (its LastReportTime) and is used
// for transition timestamps in the synthetic conditions.
// isLifecycleChange=true freezes Available and resets Ready.lut=now (Create/Replace path).
// isLifecycleChange=false uses the normal adapter-report aggregation path.
func (s *sqlNodePoolService) updateNodePoolStatusFromAdapters(
	ctx context.Context, nodePoolID string, observedTime time.Time, isLifecycleChange bool,
) (*api.NodePool, *errors.ServiceError) {
	// Get the nodepool
	nodePool, err := s.nodePoolDao.Get(ctx, nodePoolID)
	if err != nil {
		return nil, handleGetError("NodePool", "id", nodePoolID, err)
	}

	// Get all adapter statuses for this nodepool
	adapterStatuses, err := s.adapterStatusDao.FindByResource(ctx, "NodePool", nodePoolID)
	if err != nil {
		return nil, errors.GeneralError("Failed to get adapter statuses: %s", err)
	}

	now := time.Now()

	// Build the list of adapter ResourceConditions
	adapterConditions := []api.ResourceCondition{}

	for _, adapterStatus := range adapterStatuses {
		// Unmarshal Conditions from JSONB
		var conditions []api.AdapterCondition
		if err := json.Unmarshal(adapterStatus.Conditions, &conditions); err != nil {
			continue // Skip if can't unmarshal
		}

		// Find the "Available" condition
		var availableCondition *api.AdapterCondition
		for i := range conditions {
			if conditions[i].Type == api.ConditionTypeAvailable {
				availableCondition = &conditions[i]
				break
			}
		}

		if availableCondition == nil {
			// No Available condition, skip this adapter
			continue
		}

		// Convert to ResourceCondition
		condResource := api.ResourceCondition{
			Type:               MapAdapterToConditionType(adapterStatus.Adapter),
			Status:             api.ResourceConditionStatus(availableCondition.Status),
			Reason:             availableCondition.Reason,
			Message:            availableCondition.Message,
			ObservedGeneration: adapterStatus.ObservedGeneration,
			LastTransitionTime: availableCondition.LastTransitionTime,
		}

		// Set CreatedTime with nil check
		if adapterStatus.CreatedTime != nil {
			condResource.CreatedTime = *adapterStatus.CreatedTime
		}

		// Set LastUpdatedTime with nil check
		if adapterStatus.LastReportTime != nil {
			condResource.LastUpdatedTime = *adapterStatus.LastReportTime
		}

		adapterConditions = append(adapterConditions, condResource)
	}

	// Compute synthetic Available and Ready conditions
	availableCondition, readyCondition := BuildSyntheticConditions(
		ctx,
		nodePool.StatusConditions,
		adapterStatuses,
		s.adapterConfig.RequiredNodePoolAdapters(),
		nodePool.Generation,
		now,
		observedTime,
		isLifecycleChange,
	)

	// Combine synthetic conditions with adapter conditions
	// Put Available and Ready first
	allConditions := []api.ResourceCondition{availableCondition, readyCondition}
	allConditions = append(allConditions, adapterConditions...)

	// Marshal conditions to JSON
	conditionsJSON, err := json.Marshal(allConditions)
	if err != nil {
		return nil, errors.GeneralError("Failed to marshal conditions: %s", err)
	}
	nodePool.StatusConditions = conditionsJSON

	// Save the updated nodepool
	nodePool, err = s.nodePoolDao.Replace(ctx, nodePool)
	if err != nil {
		return nil, handleUpdateError("NodePool", err)
	}

	return nodePool, nil
}

// ProcessAdapterStatus handles the business logic for adapter status.
// Pre-processing rules applied in order (spec §2):
//   - Stale: discards if observed_generation < existing adapter generation
//   - P1: discards if observed_generation > resource generation (report ahead of resource)
//   - P2: rejects if mandatory conditions (Available, Applied, Health) are missing or have invalid status
//   - P3: discards if Available == Unknown (not processed per spec)
//
// Otherwise: upserts the status and triggers aggregation.
// Returns (nil, nil) for discarded/rejected updates.
func (s *sqlNodePoolService) ProcessAdapterStatus(
	ctx context.Context, nodePoolID string, adapterStatus *api.AdapterStatus,
) (*api.AdapterStatus, *errors.ServiceError) {
	existingStatus, findErr := s.adapterStatusDao.FindByResourceAndAdapter(
		ctx, "NodePool", nodePoolID, adapterStatus.Adapter,
	)
	if findErr != nil && !stderrors.Is(findErr, gorm.ErrRecordNotFound) {
		if !strings.Contains(findErr.Error(), errors.CodeNotFoundGeneric) {
			return nil, errors.GeneralError("Failed to get adapter status: %s", findErr)
		}
	}
	// Stale check: discard if older than the adapter's last recorded generation.
	if existingStatus != nil && adapterStatus.ObservedGeneration < existingStatus.ObservedGeneration {
		return nil, nil
	}

	// Parse conditions from the adapter status (needed for P2 and P3 before resource fetch).
	var conditions []api.AdapterCondition
	if len(adapterStatus.Conditions) > 0 {
		if err := json.Unmarshal(adapterStatus.Conditions, &conditions); err != nil {
			return nil, errors.GeneralError("Failed to unmarshal adapter status conditions: %s", err)
		}
	}

	// P2: validate mandatory conditions (presence and valid status values).
	if errorType, conditionName := ValidateMandatoryConditions(conditions); errorType != "" {
		logger.With(ctx, logger.FieldNodePoolID, nodePoolID).
			Info(fmt.Sprintf("Discarding adapter status update from %s: %s condition %s",
				adapterStatus.Adapter, errorType, conditionName))
		return nil, nil
	}

	// P3: discard if Available == Unknown (spec §2, all reports).
	for _, cond := range conditions {
		if cond.Type == api.ConditionTypeAvailable && cond.Status == api.AdapterConditionUnknown {
			logger.With(ctx, logger.FieldNodePoolID, nodePoolID).
				Info(fmt.Sprintf("Discarding adapter status update from %s: Available=Unknown reports are not processed",
					adapterStatus.Adapter))
			return nil, nil
		}
	}

	// P1: discard if observed_generation is ahead of the current resource generation.
	// Checked after P2/P3 to avoid unnecessary resource fetches for invalid/Unknown reports.
	nodePool, err := s.nodePoolDao.Get(ctx, nodePoolID)
	if err != nil {
		return nil, handleGetError("NodePool", "id", nodePoolID, err)
	}
	if adapterStatus.ObservedGeneration > nodePool.Generation {
		logger.With(ctx, logger.FieldNodePoolID, nodePoolID).
			Info(fmt.Sprintf("Discarding adapter status update from %s: observed_generation %d > resource generation %d",
				adapterStatus.Adapter, adapterStatus.ObservedGeneration, nodePool.Generation))
		return nil, nil
	}

	// Upsert the adapter status (complete replacement).
	upsertedStatus, upsertErr := s.adapterStatusDao.Upsert(ctx, adapterStatus)
	if upsertErr != nil {
		return nil, handleCreateError("AdapterStatus", upsertErr)
	}

	// Trigger aggregation using the adapter's observed_time for transition timestamps.
	observedTime := time.Now()
	if upsertedStatus.LastReportTime != nil {
		observedTime = *upsertedStatus.LastReportTime
	}
	if _, aggregateErr := s.updateNodePoolStatusFromAdapters(ctx, nodePoolID, observedTime, false); aggregateErr != nil {
		logger.With(ctx, logger.FieldNodePoolID, nodePoolID).
			WithError(aggregateErr).Warn("Failed to aggregate nodepool status")
	}

	return upsertedStatus, nil
}
