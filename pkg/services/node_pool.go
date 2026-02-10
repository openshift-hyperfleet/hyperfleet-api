package services

import (
	"context"
	"encoding/json"
	stderrors "errors"
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
	// - If Available condition is "Unknown": returns (nil, nil) indicating no-op
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

	// REMOVED: Event creation - no event-driven components
	return nodePool, nil
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

// UpdateNodePoolStatusFromAdapters aggregates adapter statuses into nodepool status
func (s *sqlNodePoolService) UpdateNodePoolStatusFromAdapters(
	ctx context.Context, nodePoolID string,
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
			if conditions[i].Type == conditionTypeAvailable {
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
		nodePool.StatusConditions,
		adapterStatuses,
		s.adapterConfig.RequiredNodePoolAdapters,
		nodePool.Generation,
		now,
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

// ProcessAdapterStatus handles the business logic for adapter status:
// - If Available is "Unknown" and a status already exists: returns (nil, nil) as no-op
// - If Available is "Unknown" and no status exists (first report): upserts the status
// - Otherwise: upserts the status and triggers aggregation
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
	if existingStatus != nil && adapterStatus.ObservedGeneration < existingStatus.ObservedGeneration {
		// Discard stale status updates (older observed_generation).
		return nil, nil
	}

	// Parse conditions from the adapter status
	var conditions []api.AdapterCondition
	if len(adapterStatus.Conditions) > 0 {
		if err := json.Unmarshal(adapterStatus.Conditions, &conditions); err != nil {
			return nil, errors.GeneralError("Failed to unmarshal adapter status conditions: %s", err)
		}
	}

	// Find the "Available" condition
	hasAvailableCondition := false
	for _, cond := range conditions {
		if cond.Type != conditionTypeAvailable {
			continue
		}

		hasAvailableCondition = true
		if cond.Status == api.AdapterConditionUnknown {
			if existingStatus != nil {
				// Available condition is "Unknown" and a status already exists, return nil to indicate no-op
				return nil, nil
			}
			// First report from this adapter: allow storing even with Available=Unknown
			// but skip aggregation since Unknown should not affect nodepool-level conditions
			hasAvailableCondition = false
		}
	}

	// Upsert the adapter status
	upsertedStatus, err := s.adapterStatusDao.Upsert(ctx, adapterStatus)
	if err != nil {
		return nil, handleCreateError("AdapterStatus", err)
	}

	// Only trigger aggregation when the adapter reported an Available condition.
	// If the adapter status doesn't include Available, saving it should not overwrite
	// the nodepool's synthetic Available/Ready conditions.
	if hasAvailableCondition {
		if _, aggregateErr := s.UpdateNodePoolStatusFromAdapters(
			ctx, nodePoolID,
		); aggregateErr != nil {
			// Log error but don't fail the request - the status will be computed on next update
			logger.With(ctx, logger.FieldNodePoolID, nodePoolID).
				WithError(aggregateErr).Warn("Failed to aggregate nodepool status")
		}
	}

	return upsertedStatus, nil
}
