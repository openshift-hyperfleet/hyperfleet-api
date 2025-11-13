package services

import (
	"context"
	"encoding/json"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/logger"
)

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

func NewNodePoolService(lockFactory db.LockFactory, nodePoolDao dao.NodePoolDao, adapterStatusDao dao.AdapterStatusDao, events EventService) NodePoolService {
	return &sqlNodePoolService{
		lockFactory:      lockFactory,
		nodePoolDao:      nodePoolDao,
		adapterStatusDao: adapterStatusDao,
		events:           events,
	}
}

var _ NodePoolService = &sqlNodePoolService{}

type sqlNodePoolService struct {
	lockFactory      db.LockFactory
	nodePoolDao      dao.NodePoolDao
	adapterStatusDao dao.AdapterStatusDao
	events           EventService
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

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "NodePools",
		SourceID:  nodePool.ID,
		EventType: api.CreateEventType,
	})
	if evErr != nil {
		return nil, handleCreateError("NodePool", evErr)
	}

	return nodePool, nil
}

func (s *sqlNodePoolService) Replace(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, *errors.ServiceError) {
	nodePool, err := s.nodePoolDao.Replace(ctx, nodePool)
	if err != nil {
		return nil, handleUpdateError("NodePool", err)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "NodePools",
		SourceID:  nodePool.ID,
		EventType: api.UpdateEventType,
	})
	if evErr != nil {
		return nil, handleUpdateError("NodePool", evErr)
	}

	return nodePool, nil
}

func (s *sqlNodePoolService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.nodePoolDao.Delete(ctx, id); err != nil {
		return handleDeleteError("NodePool", errors.GeneralError("Unable to delete nodePool: %s", err))
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "NodePools",
		SourceID:  id,
		EventType: api.DeleteEventType,
	})
	if evErr != nil {
		return handleDeleteError("NodePool", evErr)
	}

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
	logger := logger.NewOCMLogger(ctx)

	nodePool, err := s.nodePoolDao.Get(ctx, id)
	if err != nil {
		return err
	}

	logger.Infof("Do idempotent somethings with this nodePool: %s", nodePool.ID)

	return nil
}

func (s *sqlNodePoolService) OnDelete(ctx context.Context, id string) error {
	logger := logger.NewOCMLogger(ctx)
	logger.Infof("This nodePool has been deleted: %s", id)
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

	// Build the list of ConditionAvailable
	adapters := []openapi.ConditionAvailable{}
	allReady := true
	anyFailed := false
	maxObservedGeneration := int32(0)

	for _, adapterStatus := range adapterStatuses {
		// Unmarshal Conditions from JSONB
		var conditions []openapi.Condition
		if err := json.Unmarshal(adapterStatus.Conditions, &conditions); err != nil {
			continue // Skip if can't unmarshal
		}

		// Find the "Available" condition
		var availableCondition *openapi.Condition
		for i := range conditions {
			if conditions[i].Type == "Available" {
				availableCondition = &conditions[i]
				break
			}
		}

		if availableCondition == nil {
			// No Available condition means adapter is not ready
			allReady = false
			continue
		}

		// Convert to ConditionAvailable
		condAvail := openapi.ConditionAvailable{
			Type:               availableCondition.Type,
			Adapter:            adapterStatus.Adapter,
			Status:             availableCondition.Status,
			Reason:             availableCondition.Reason,
			Message:            availableCondition.Message,
			ObservedGeneration: availableCondition.ObservedGeneration,
		}
		adapters = append(adapters, condAvail)

		// Check status
		if availableCondition.Status != "True" {
			allReady = false
			if availableCondition.Status == "False" {
				anyFailed = true
			}
		}

		// Track max observed generation
		if adapterStatus.ObservedGeneration > maxObservedGeneration {
			maxObservedGeneration = adapterStatus.ObservedGeneration
		}
	}

	// Compute overall phase
	phase := "NotReady"
	if len(adapterStatuses) > 0 {
		if allReady {
			phase = "Ready"
		} else if anyFailed {
			phase = "Failed"
		}
	}

	// Update nodepool status fields
	now := time.Now()
	nodePool.StatusPhase = phase
	nodePool.StatusObservedGeneration = maxObservedGeneration

	// Marshal adapters to JSON
	adaptersJSON, err := json.Marshal(adapters)
	if err != nil {
		return nil, errors.GeneralError("Failed to marshal adapters: %s", err)
	}
	nodePool.StatusAdapters = adaptersJSON
	nodePool.StatusUpdatedAt = &now

	// Update last transition time if phase changed
	if nodePool.StatusLastTransitionTime == nil || nodePool.StatusPhase != phase {
		nodePool.StatusLastTransitionTime = &now
	}

	// Save the updated nodepool
	nodePool, err = s.nodePoolDao.Replace(ctx, nodePool)
	if err != nil {
		return nil, handleUpdateError("NodePool", err)
	}

	return nodePool, nil
}
