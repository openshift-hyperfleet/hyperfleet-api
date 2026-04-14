package services

import (
	"bytes"
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
	RequestDeletion(ctx context.Context, id string) (*api.NodePool, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (api.NodePoolList, *errors.ServiceError)

	FindByIDs(ctx context.Context, ids []string) (api.NodePoolList, *errors.ServiceError)

	UpdateNodePoolStatusFromAdapters(ctx context.Context, nodePoolID string) (*api.NodePool, *errors.ServiceError)

	ProcessAdapterStatus(
		ctx context.Context, nodePoolID string, adapterStatus *api.AdapterStatus,
	) (*api.AdapterStatus, *errors.ServiceError)

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

	return updatedNodePool, nil
}

func (s *sqlNodePoolService) Replace(
	ctx context.Context, nodePool *api.NodePool,
) (*api.NodePool, *errors.ServiceError) {
	nodePool, err := s.nodePoolDao.Replace(ctx, nodePool)
	if err != nil {
		return nil, handleUpdateError("NodePool", err)
	}

	updated, svcErr := s.UpdateNodePoolStatusFromAdapters(ctx, nodePool.ID)
	if svcErr != nil {
		return nil, svcErr
	}
	return updated, nil
}

func (s *sqlNodePoolService) RequestDeletion(ctx context.Context, id string) (*api.NodePool, *errors.ServiceError) {
	nodePool, err := s.nodePoolDao.RequestDeletion(ctx, id)
	if err != nil {
		return nil, handleRequestDeletionError("NodePool", err)
	}

	return nodePool, nil
}

func (s *sqlNodePoolService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.nodePoolDao.Delete(ctx, id); err != nil {
		return handleDeleteError("NodePool", errors.GeneralError("Unable to delete nodePool: %s", err))
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

func nodePoolRefTime(np *api.NodePool) time.Time {
	if np == nil {
		return time.Time{}
	}
	if !np.UpdatedTime.IsZero() {
		return np.UpdatedTime
	}
	return np.CreatedTime
}

func (s *sqlNodePoolService) UpdateNodePoolStatusFromAdapters(
	ctx context.Context, nodePoolID string,
) (*api.NodePool, *errors.ServiceError) {
	nodePool, err := s.nodePoolDao.Get(ctx, nodePoolID)
	if err != nil {
		return nil, handleGetError("NodePool", "id", nodePoolID, err)
	}

	adapterStatuses, err := s.adapterStatusDao.FindByResource(ctx, "NodePool", nodePoolID)
	if err != nil {
		return nil, errors.GeneralError("Failed to get adapter statuses: %s", err)
	}

	refTime := nodePoolRefTime(nodePool)
	ready, available, adapterConditions := AggregateResourceStatus(ctx, AggregateResourceStatusInput{
		ResourceGeneration: nodePool.Generation,
		RefTime:            refTime,
		PrevConditionsJSON: nodePool.StatusConditions,
		RequiredAdapters:   s.adapterConfig.RequiredNodePoolAdapters(),
		AdapterStatuses:    adapterStatuses,
	})

	allConditions := make([]api.ResourceCondition, 0, 2+len(adapterConditions))
	allConditions = append(allConditions, ready, available)
	allConditions = append(allConditions, adapterConditions...)

	conditionsJSON, err := json.Marshal(allConditions)
	if err != nil {
		return nil, errors.GeneralError("Failed to marshal conditions: %s", err)
	}

	if bytes.Equal(nodePool.StatusConditions, conditionsJSON) {
		return nodePool, nil
	}

	nodePool.StatusConditions = conditionsJSON

	nodePool, err = s.nodePoolDao.Replace(ctx, nodePool)
	if err != nil {
		return nil, handleUpdateError("NodePool", err)
	}

	return nodePool, nil
}

func (s *sqlNodePoolService) ProcessAdapterStatus(
	ctx context.Context, nodePoolID string, adapterStatus *api.AdapterStatus,
) (*api.AdapterStatus, *errors.ServiceError) {
	nodePool, err := s.nodePoolDao.Get(ctx, nodePoolID)
	if err != nil {
		return nil, handleGetError("NodePool", "id", nodePoolID, err)
	}

	existingStatus, findErr := s.adapterStatusDao.FindByResourceAndAdapter(
		ctx, "NodePool", nodePoolID, adapterStatus.Adapter,
	)
	if findErr != nil && !stderrors.Is(findErr, gorm.ErrRecordNotFound) {
		if !strings.Contains(findErr.Error(), errors.CodeNotFoundGeneric) {
			return nil, errors.GeneralError("Failed to get adapter status: %s", findErr)
		}
	}

	if adapterStatus.ObservedGeneration > nodePool.Generation {
		logger.With(ctx, logger.FieldNodePoolID, nodePoolID, logger.FieldAdapter, adapterStatus.Adapter).
			Debug("Discarding adapter status update: future generation")
		return nil, nil
	}

	if existingStatus != nil && adapterStatus.ObservedGeneration < existingStatus.ObservedGeneration {
		logger.With(ctx, logger.FieldNodePoolID, nodePoolID, logger.FieldAdapter, adapterStatus.Adapter).
			Debug("Discarding adapter status update: stale generation")
		return nil, nil
	}

	incomingObs := AdapterObservedTime(adapterStatus)
	if incomingObs.IsZero() {
		logger.With(ctx, logger.FieldNodePoolID, nodePoolID, logger.FieldAdapter, adapterStatus.Adapter).
			Debug("Discarding adapter status update: zero observed time")
		return nil, nil
	}

	if existingStatus != nil && adapterStatus.ObservedGeneration == existingStatus.ObservedGeneration {
		prevObs := AdapterObservedTime(existingStatus)
		if !prevObs.IsZero() && incomingObs.Before(prevObs) {
			logger.With(ctx, logger.FieldNodePoolID, nodePoolID, logger.FieldAdapter, adapterStatus.Adapter).
				Debug("Discarding adapter status update: stale observed time")
			return nil, nil
		}
	}

	var conditions []api.AdapterCondition
	if len(adapterStatus.Conditions) > 0 {
		if errUnmarshal := json.Unmarshal(adapterStatus.Conditions, &conditions); errUnmarshal != nil {
			return nil, errors.GeneralError("Failed to unmarshal adapter status conditions: %s", errUnmarshal)
		}
	}

	if errorType, conditionName := ValidateMandatoryConditions(conditions); errorType != "" {
		return nil, errors.Validation(
			"missing mandatory condition '%s': all adapters must report Available, Applied, and Health",
			conditionName,
		)
	}

	triggerAggregation := false
	for _, cond := range conditions {
		if cond.Type != api.ConditionTypeAvailable {
			continue
		}

		isValidStatus := cond.Status == api.AdapterConditionTrue ||
			cond.Status == api.AdapterConditionFalse ||
			cond.Status == api.AdapterConditionUnknown
		if !isValidStatus {
			return nil, errors.Validation(
				"condition '%s' has invalid status '%s': must be True, False, or Unknown",
				cond.Type, cond.Status,
			)
		}

		if cond.Status != api.AdapterConditionTrue && cond.Status != api.AdapterConditionFalse {
			// Status is Unknown — only valid on first report; discard subsequent reports.
			if existingStatus != nil {
				logger.With(ctx, logger.FieldNodePoolID, nodePoolID, logger.FieldAdapter, adapterStatus.Adapter).
					Debug("Discarding adapter status update: subsequent Unknown Available")
				return nil, nil
			}
			triggerAggregation = false
			break
		}

		triggerAggregation = true
		break
	}

	adapterStatus.ResourceType = "NodePool"
	adapterStatus.ResourceID = nodePoolID

	upsertedStatus, err := s.adapterStatusDao.Upsert(ctx, adapterStatus)
	if err != nil {
		return nil, handleCreateError("AdapterStatus", err)
	}

	if triggerAggregation {
		if _, aggregateErr := s.UpdateNodePoolStatusFromAdapters(ctx, nodePoolID); aggregateErr != nil {
			return nil, aggregateErr
		}
	}

	return upsertedStatus, nil
}
