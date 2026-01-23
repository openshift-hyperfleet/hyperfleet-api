package services

import (
	"context"
	"encoding/json"
	"math"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
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

	// idempotent functions for the control plane, but can also be called synchronously by any actor
	OnUpsert(ctx context.Context, id string) error
	OnDelete(ctx context.Context, id string) error
}

func NewNodePoolService(nodePoolDao dao.NodePoolDao, adapterStatusDao dao.AdapterStatusDao, adapterConfig *config.AdapterRequirementsConfig) NodePoolService {
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
	nodePool, err := s.nodePoolDao.Create(ctx, nodePool)
	if err != nil {
		return nil, handleCreateError("NodePool", err)
	}

	// REMOVED: Event creation - no event-driven components
	return nodePool, nil
}

func (s *sqlNodePoolService) Replace(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, *errors.ServiceError) {
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

	logger.With(ctx, logger.FieldNodePoolID, nodePool.ID).Info("Perform idempotent operations on node pool")

	return nil
}

func (s *sqlNodePoolService) OnDelete(ctx context.Context, id string) error {
	logger.With(ctx, logger.FieldNodePoolID, id).Info("Node pool has been deleted")
	return nil
}

// UpdateNodePoolStatusFromAdapters aggregates adapter statuses into nodepool status
func (s *sqlNodePoolService) UpdateNodePoolStatusFromAdapters(ctx context.Context, nodePoolID string) (*api.NodePool, *errors.ServiceError) {
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

	// Build the list of ResourceCondition
	adapters := []api.ResourceCondition{}
	minObservedGeneration := int32(math.MaxInt32)

	for _, adapterStatus := range adapterStatuses {
		// Unmarshal Conditions from JSONB
		var conditions []api.AdapterCondition
		if err := json.Unmarshal(adapterStatus.Conditions, &conditions); err != nil {
			continue // Skip if can't unmarshal
		}

		// Find the "Available" condition
		var availableCondition *api.AdapterCondition
		for i := range conditions {
			if conditions[i].Type == "Available" {
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
			Status:             availableCondition.Status,
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

		adapters = append(adapters, condResource)

		// Track min observed generation
		// Use the LOWEST generation to ensure nodepool status only advances when ALL adapters catch up
		if adapterStatus.ObservedGeneration < minObservedGeneration {
			minObservedGeneration = adapterStatus.ObservedGeneration
		}
	}

	// Compute overall phase using required adapters from config
	newPhase := ComputePhase(ctx, adapterStatuses, s.adapterConfig.RequiredNodePoolAdapters, nodePool.Generation)

	// Calculate min(adapters[].last_report_time) for nodepool.status.last_updated_time
	// This uses the OLDEST adapter timestamp to ensure Sentinel can detect stale adapters
	var minLastUpdatedTime *time.Time
	for _, adapterStatus := range adapterStatuses {
		if adapterStatus.LastReportTime != nil {
			if minLastUpdatedTime == nil || adapterStatus.LastReportTime.Before(*minLastUpdatedTime) {
				minLastUpdatedTime = adapterStatus.LastReportTime
			}
		}
	}

	// Save old phase to detect transitions
	oldPhase := nodePool.StatusPhase

	// Update nodepool status fields
	now := time.Now()
	nodePool.StatusPhase = newPhase
	// Set observed_generation to min across all adapters (0 if no adapters, matching DB default)
	if len(adapterStatuses) == 0 {
		nodePool.StatusObservedGeneration = 0
	} else {
		nodePool.StatusObservedGeneration = minObservedGeneration
	}

	// Marshal conditions to JSON
	conditionsJSON, err := json.Marshal(adapters)
	if err != nil {
		return nil, errors.GeneralError("Failed to marshal conditions: %s", err)
	}
	nodePool.StatusConditions = conditionsJSON

	// Use min(adapters[].last_report_time) instead of now()
	// This ensures Sentinel triggers reconciliation when ANY adapter is stale
	if minLastUpdatedTime != nil {
		nodePool.StatusLastUpdatedTime = minLastUpdatedTime
	} else {
		nodePool.StatusLastUpdatedTime = &now
	}

	// Update last transition time only if phase changed
	if nodePool.StatusLastTransitionTime == nil || oldPhase != newPhase {
		nodePool.StatusLastTransitionTime = &now
	}

	// Save the updated nodepool
	nodePool, err = s.nodePoolDao.Replace(ctx, nodePool)
	if err != nil {
		return nil, handleUpdateError("NodePool", err)
	}

	return nodePool, nil
}
