package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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
	Patch(ctx context.Context, id string, patch *api.NodePoolPatchRequest) (*api.NodePool, *errors.ServiceError)
	GetByIDAndOwner(ctx context.Context, id string, ownerID string) (*api.NodePool, *errors.ServiceError)
	ListByCluster(
		ctx context.Context, clusterID string, args *ListArguments,
	) (api.NodePoolList, *api.PagingMeta, *errors.ServiceError)
	SoftDelete(ctx context.Context, id string) (*api.NodePool, *errors.ServiceError)
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
	clusterDao dao.ClusterDao,
	adapterStatusDao dao.AdapterStatusDao,
	adapterConfig *config.AdapterRequirementsConfig,
	generic GenericService,
) NodePoolService {
	return &sqlNodePoolService{
		nodePoolDao:      nodePoolDao,
		clusterDao:       clusterDao,
		adapterStatusDao: adapterStatusDao,
		adapterConfig:    adapterConfig,
		generic:          generic,
	}
}

var _ NodePoolService = &sqlNodePoolService{}

type sqlNodePoolService struct {
	nodePoolDao      dao.NodePoolDao
	clusterDao       dao.ClusterDao
	adapterStatusDao dao.AdapterStatusDao
	adapterConfig    *config.AdapterRequirementsConfig
	generic          GenericService
}

func (s *sqlNodePoolService) Get(ctx context.Context, id string) (*api.NodePool, *errors.ServiceError) {
	nodePool, err := s.nodePoolDao.Get(ctx, id)
	if err != nil {
		return nil, handleGetError("NodePool", "id", id, err)
	}
	return nodePool, nil
}

func (s *sqlNodePoolService) Create(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, *errors.ServiceError) {
	if nodePool.CreatedBy == "" {
		nodePool.CreatedBy = defaultSystemUser
	}
	if nodePool.UpdatedBy == "" {
		nodePool.UpdatedBy = defaultSystemUser
	}
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

func (s *sqlNodePoolService) Patch(
	ctx context.Context, nodePoolID string, patch *api.NodePoolPatchRequest,
) (*api.NodePool, *errors.ServiceError) {
	nodePool, err := s.nodePoolDao.GetForUpdate(ctx, nodePoolID)
	if err != nil {
		return nil, handleGetError("NodePool", "id", nodePoolID, err)
	}

	if nodePool.DeletedTime != nil {
		return nil, errors.ConflictState("NodePool '%s' is marked for deletion", nodePoolID)
	}

	oldSpec := nodePool.Spec
	oldLabels := nodePool.Labels

	if applyErr := applyNodePoolPatch(nodePool, patch); applyErr != nil {
		return nil, errors.Validation("Invalid patch data: %v", applyErr)
	}

	if bytes.Equal(oldSpec, nodePool.Spec) && bytes.Equal(oldLabels, nodePool.Labels) {
		return nodePool, nil
	}

	nodePool.IncrementGeneration()

	if saveErr := s.nodePoolDao.Save(ctx, nodePool); saveErr != nil {
		return nil, handleUpdateError("NodePool", saveErr)
	}

	updated, svcErr := s.UpdateNodePoolStatusFromAdapters(ctx, nodePool.ID)
	if svcErr != nil {
		return nil, svcErr
	}
	return updated, nil
}

func (s *sqlNodePoolService) GetByIDAndOwner(
	ctx context.Context, nodePoolID string, clusterID string,
) (*api.NodePool, *errors.ServiceError) {
	nodePool, err := s.nodePoolDao.GetByIDAndOwner(ctx, nodePoolID, clusterID)
	if err != nil {
		return nil, handleGetError("NodePool", "id", nodePoolID, err)
	}
	return nodePool, nil
}

func (s *sqlNodePoolService) ListByCluster(
	ctx context.Context, clusterID string, args *ListArguments,
) (api.NodePoolList, *api.PagingMeta, *errors.ServiceError) {
	if _, err := s.clusterDao.Get(ctx, clusterID); err != nil {
		return nil, nil, handleGetError("Cluster", "id", clusterID, err)
	}

	if args.Search == "" {
		args.Search = "owner_id = '" + clusterID + "'"
	} else {
		args.Search = "(" + args.Search + ") AND owner_id = '" + clusterID + "'"
	}

	var nodePools []api.NodePool
	paging, svcErr := s.generic.List(ctx, args, &nodePools)
	if svcErr != nil {
		return nil, nil, svcErr
	}

	result := make(api.NodePoolList, len(nodePools))
	for i := range nodePools {
		result[i] = &nodePools[i]
	}
	return result, paging, nil
}

func (s *sqlNodePoolService) SoftDelete(ctx context.Context, nodePoolID string) (*api.NodePool, *errors.ServiceError) {
	nodePool, err := s.nodePoolDao.GetForUpdate(ctx, nodePoolID)
	if err != nil {
		return nil, handleSoftDeleteError("NodePool", err)
	}

	if nodePool.DeletedTime != nil {
		return nodePool, nil
	}

	t := time.Now().UTC().Truncate(time.Microsecond)
	deletedBy := defaultSystemUser
	nodePool.MarkDeleted(deletedBy, t)
	nodePool.IncrementGeneration()

	if err := s.nodePoolDao.Save(ctx, nodePool); err != nil {
		return nil, handleSoftDeleteError("NodePool", err)
	}

	updated, svcErr := s.UpdateNodePoolStatusFromAdapters(ctx, nodePool.ID)
	if svcErr != nil {
		return nil, svcErr
	}

	return updated, nil
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

func applyNodePoolPatch(nodePool *api.NodePool, patch *api.NodePoolPatchRequest) error {
	if patch.Spec != nil {
		specJSON, err := json.Marshal(*patch.Spec)
		if err != nil {
			return fmt.Errorf("failed to marshal nodepool spec: %w", err)
		}
		nodePool.Spec = specJSON
	}
	if patch.Labels != nil {
		labelsJSON, err := json.Marshal(*patch.Labels)
		if err != nil {
			return fmt.Errorf("failed to marshal nodepool labels: %w", err)
		}
		nodePool.Labels = labelsJSON
	}
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

// UpdateNodePoolStatusFromAdapters is the public entry point for callers outside
// ProcessAdapterStatus (e.g. Create, Patch, SoftDelete) that don't already hold the node
// pool and adapter statuses.
func (s *sqlNodePoolService) UpdateNodePoolStatusFromAdapters(
	ctx context.Context, nodePoolID string,
) (*api.NodePool, *errors.ServiceError) {
	return updateNodePoolStatusFromAdapters(
		ctx,
		nodePoolID,
		s.nodePoolDao,
		s.adapterStatusDao,
		s.adapterConfig,
	)
}

// recomputeAndSaveNodePoolStatus aggregates adapter reports into node pool conditions and
// persists only when the result differs from the current stored value. Accepts pre-fetched
// data to avoid redundant DB reads.
func (s *sqlNodePoolService) recomputeAndSaveNodePoolStatus(
	ctx context.Context, nodePool *api.NodePool, adapterStatuses api.AdapterStatusList,
) (*api.NodePool, *errors.ServiceError) {
	conditionsJSON, svcErr := computeNodePoolConditionsJSON(
		ctx, nodePool, adapterStatuses, s.adapterConfig.RequiredNodePoolAdapters(),
	)
	if svcErr != nil {
		return nil, svcErr
	}
	if conditionsJSON == nil {
		return nodePool, nil
	}

	if err := s.nodePoolDao.SaveStatusConditions(ctx, nodePool.ID, conditionsJSON); err != nil {
		return nil, handleUpdateError("NodePool", err)
	}

	nodePool.StatusConditions = conditionsJSON
	return nodePool, nil
}

// ProcessAdapterStatus validates mandatory conditions, applies discard rules, upserts adapter
// status, and triggers aggregation when appropriate.
//
// DB call budget (non-deleted happy path):
//  1. GetForUpdate        — lock + fetch node pool
//  2. FindByResource      — all adapter statuses (existing status found in-memory)
//  3. Upsert             — write adapter status
//  4. SaveStatusConditions — write updated node pool conditions (skipped when unchanged)
func (s *sqlNodePoolService) ProcessAdapterStatus(
	ctx context.Context, nodePoolID string, adapterStatus *api.AdapterStatus,
) (*api.AdapterStatus, *errors.ServiceError) {
	// 1. Acquire a row-level exclusive lock on the node pool for the duration of this
	// transaction. Concurrent adapter status updates for the same node pool are
	// serialized here: the second caller blocks until the first commits, ensuring
	// the aggregation step always reads a fully up-to-date set of adapter statuses.
	nodePool, err := s.nodePoolDao.GetForUpdate(ctx, nodePoolID)
	if err != nil {
		return nil, handleGetError("NodePool", "id", nodePoolID, err)
	}

	// 2. Fetch all adapter statuses for this node pool in one query. This replaces the
	// individual FindByResourceAndAdapter and later FindByResource calls.
	allStatuses, err := s.adapterStatusDao.FindByResource(ctx, "NodePool", nodePoolID)
	if err != nil {
		return nil, errors.GeneralError("Failed to get adapter statuses: %s", err)
	}

	existingStatus := findAdapterStatusInList(allStatuses, adapterStatus.Adapter)

	conditions, triggerAggregation, svcErr := s.validateAndClassifyNodePool(
		ctx, nodePoolID, adapterStatus, nodePool, existingStatus,
	)
	if svcErr != nil {
		return nil, svcErr
	}
	if conditions == nil && !triggerAggregation {
		return nil, nil
	}

	// 3. Upsert using the already-fetched existing status to skip a redundant lookup.
	adapterStatus.ResourceType = "NodePool"
	adapterStatus.ResourceID = nodePoolID
	setConditionTransitionTimes(adapterStatus, existingStatus)

	upsertedStatus, err := s.adapterStatusDao.Upsert(ctx, adapterStatus, existingStatus)
	if err != nil {
		return nil, handleCreateError("AdapterStatus", err)
	}

	// Build the post-upsert snapshot once and reuse it for both the hard-delete check
	// and aggregation. Using the pre-upsert allStatuses for the hard-delete check would
	// cause allAdaptersFinalized to return false when the current adapter is the last one
	// needed to complete the Finalized=True quorum.
	updatedStatuses := replaceAdapterStatusInList(allStatuses, upsertedStatus)

	if nodePool.DeletedTime != nil {
		hardDeleted, hdErr := s.tryHardDeleteNodePool(ctx, nodePool, conditions, updatedStatuses)
		if hdErr != nil {
			return nil, hdErr
		}
		if hardDeleted {
			return upsertedStatus, nil
		}
	}

	// 4. Re-aggregate using data already in memory.
	if triggerAggregation {
		if _, aggregateErr := s.recomputeAndSaveNodePoolStatus(ctx, nodePool, updatedStatuses); aggregateErr != nil {
			return nil, aggregateErr
		}
	}

	return upsertedStatus, nil
}

// validateAndClassifyNodePool performs all stateless validation and discard-rule checks on an
// incoming adapter status for a node pool. Returns the parsed conditions and whether aggregation
// should be triggered. Returns (nil, false, nil) when the update should be silently discarded.
func (s *sqlNodePoolService) validateAndClassifyNodePool(
	ctx context.Context,
	nodePoolID string,
	adapterStatus *api.AdapterStatus,
	nodePool *api.NodePool,
	existingStatus *api.AdapterStatus,
) ([]api.AdapterCondition, bool, *errors.ServiceError) {
	l := logger.With(ctx, logger.FieldNodePoolID, nodePoolID, logger.FieldAdapter, adapterStatus.Adapter)

	if adapterStatus.ObservedGeneration > nodePool.Generation {
		l.Debug("Discarding adapter status update: future generation")
		return nil, false, nil
	}

	if existingStatus != nil && adapterStatus.ObservedGeneration < existingStatus.ObservedGeneration {
		l.Debug("Discarding adapter status update: stale generation")
		return nil, false, nil
	}

	incomingObs := AdapterObservedTime(adapterStatus)
	if incomingObs.IsZero() {
		l.Debug("Discarding adapter status update: zero observed time")
		return nil, false, nil
	}

	if existingStatus != nil && adapterStatus.ObservedGeneration == existingStatus.ObservedGeneration {
		prevObs := AdapterObservedTime(existingStatus)
		if !prevObs.IsZero() && incomingObs.Before(prevObs) {
			l.Debug("Discarding adapter status update: stale observed time")
			return nil, false, nil
		}
	}

	var conditions []api.AdapterCondition
	if len(adapterStatus.Conditions) > 0 {
		if errUnmarshal := json.Unmarshal(adapterStatus.Conditions, &conditions); errUnmarshal != nil {
			return nil, false, errors.GeneralError("Failed to unmarshal adapter status conditions: %s", errUnmarshal)
		}
	}

	if errorType, conditionName := ValidateMandatoryConditions(conditions); errorType != "" {
		return nil, false, errors.Validation(
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
			return nil, false, errors.Validation(
				"condition '%s' has invalid status '%s': must be True, False, or Unknown",
				cond.Type, cond.Status,
			)
		}

		if cond.Status != api.AdapterConditionTrue && cond.Status != api.AdapterConditionFalse {
			if existingStatus != nil {
				l.Debug("Discarding adapter status update: subsequent Unknown Available")
				return nil, false, nil
			}
			triggerAggregation = false
			break
		}

		triggerAggregation = true
		break
	}

	return conditions, triggerAggregation, nil
}

// tryHardDeleteNodePool checks whether all required adapters have reported Finalized=True at the current
// resource generation for a soft-deleted node pool and, when so, permanently removes the node pool and all
// its adapter statuses. Unlike clusters, node pools have no child resources to check. Returns true when the
// hard-delete was performed.
//
// Accepts the pre-fetched (post-upsert) adapter statuses list to avoid a redundant FindByResource
// call and to ensure the just-upserted status is visible to allAdaptersFinalized.
func (s *sqlNodePoolService) tryHardDeleteNodePool(
	ctx context.Context, nodePool *api.NodePool, conditions []api.AdapterCondition,
	allStatuses api.AdapterStatusList,
) (bool, *errors.ServiceError) {
	if !incomingReportedFinalized(conditions) {
		return false, nil
	}

	if !allAdaptersFinalized(s.adapterConfig.Required.Nodepool, allStatuses, nodePool.Generation) {
		return false, nil
	}

	if err := s.adapterStatusDao.DeleteByResource(ctx, "NodePool", nodePool.ID); err != nil {
		return false, errors.GeneralError("Failed to delete adapter statuses during hard-delete: %s", err)
	}
	if err := s.nodePoolDao.Delete(ctx, nodePool.ID); err != nil {
		return false, errors.GeneralError("Failed to hard-delete nodepool: %s", err)
	}

	logger.With(ctx, logger.FieldNodePoolID, nodePool.ID).
		Info("Hard-deleted nodepool after all required adapters reported Finalized=True")

	return true, nil
}
