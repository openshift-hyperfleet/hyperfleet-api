package services

import (
	"bytes"
	"context"
	"encoding/json"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

//go:generate mockgen-v0.6.0 -source=cluster.go -package=services -destination=cluster_mock.go

type ClusterService interface {
	Get(ctx context.Context, id string) (*api.Cluster, *errors.ServiceError)
	Create(ctx context.Context, cluster *api.Cluster) (*api.Cluster, *errors.ServiceError)
	Replace(ctx context.Context, cluster *api.Cluster) (*api.Cluster, *errors.ServiceError)
	SoftDelete(ctx context.Context, id string) (*api.Cluster, *errors.ServiceError)
	All(ctx context.Context) (api.ClusterList, *errors.ServiceError)
	FindByIDs(ctx context.Context, ids []string) (api.ClusterList, *errors.ServiceError)
	UpdateClusterStatusFromAdapters(ctx context.Context, clusterID string) (*api.Cluster, *errors.ServiceError)
	ProcessAdapterStatus(ctx context.Context, clusterID string, adapterStatus *api.AdapterStatus) (*api.AdapterStatus, *errors.ServiceError) // nolint:lll

	// idempotent functions for the control plane, but can also be called synchronously by any actor
	OnUpsert(ctx context.Context, id string) error
	OnDelete(ctx context.Context, id string) error
}

func NewClusterService(
	clusterDao dao.ClusterDao,
	nodePoolDao dao.NodePoolDao,
	adapterStatusDao dao.AdapterStatusDao,
	adapterConfig *config.AdapterRequirementsConfig,
) ClusterService {
	return &sqlClusterService{
		clusterDao:       clusterDao,
		nodePoolDao:      nodePoolDao,
		adapterStatusDao: adapterStatusDao,
		adapterConfig:    adapterConfig,
	}
}

var _ ClusterService = &sqlClusterService{}

type sqlClusterService struct {
	clusterDao       dao.ClusterDao
	nodePoolDao      dao.NodePoolDao
	adapterStatusDao dao.AdapterStatusDao
	adapterConfig    *config.AdapterRequirementsConfig
}

func (s *sqlClusterService) Get(ctx context.Context, id string) (*api.Cluster, *errors.ServiceError) {
	cluster, err := s.clusterDao.Get(ctx, id)
	if err != nil {
		return nil, handleGetError("Cluster", "id", id, err)
	}
	return cluster, nil
}

func (s *sqlClusterService) Create(ctx context.Context, cluster *api.Cluster) (*api.Cluster, *errors.ServiceError) {
	if cluster.Generation == 0 {
		cluster.Generation = 1
	}

	cluster, err := s.clusterDao.Create(ctx, cluster)
	if err != nil {
		return nil, handleCreateError("Cluster", err)
	}

	updatedCluster, svcErr := s.UpdateClusterStatusFromAdapters(ctx, cluster.ID)
	if svcErr != nil {
		return nil, svcErr
	}

	return updatedCluster, nil
}

func (s *sqlClusterService) Replace(ctx context.Context, cluster *api.Cluster) (*api.Cluster, *errors.ServiceError) {
	cluster, err := s.clusterDao.Replace(ctx, cluster)
	if err != nil {
		return nil, handleUpdateError("Cluster", err)
	}

	updated, svcErr := s.UpdateClusterStatusFromAdapters(ctx, cluster.ID)
	if svcErr != nil {
		return nil, svcErr
	}
	return updated, nil
}

// SoftDelete marks a cluster for deletion by setting DeletedTime and
// DeletedBy to the current time and system@hyperfleet.local.
// If already marked, it returns the cluster unchanged. Cascades the deletion timestamp to all child nodepools.
// Actual removal is handled by adapters detecting the new generation and triggering hard deletion asynchronously.
func (s *sqlClusterService) SoftDelete(ctx context.Context, id string) (*api.Cluster, *errors.ServiceError) {
	cluster, err := s.clusterDao.GetForUpdate(ctx, id)
	if err != nil {
		return nil, handleSoftDeleteError("Cluster", err)
	}

	// Already marked for deletion — skip cascade to avoid unnecessary DB roundtrips.
	if cluster.DeletedTime != nil {
		return cluster, nil
	}

	t := time.Now().UTC().Truncate(time.Microsecond)
	deletedBy := "system@hyperfleet.local"
	cluster.DeletedTime = &t
	cluster.DeletedBy = &deletedBy
	cluster.Generation++

	if saveErr := s.clusterDao.Save(ctx, cluster); saveErr != nil {
		return nil, handleSoftDeleteError("Cluster", saveErr)
	}

	if cascadeErr := s.nodePoolDao.SoftDeleteByOwner(ctx, id, t, deletedBy); cascadeErr != nil {
		return nil, handleSoftDeleteError("NodePool", cascadeErr)
	}

	cluster, svcErr := s.UpdateClusterStatusFromAdapters(ctx, cluster.ID)
	if svcErr != nil {
		return nil, svcErr
	}

	// Update status for all cascade-deleted nodepools so their Ready condition reflects the generation bump.
	nodePools, err := s.nodePoolDao.FindSoftDeletedByOwner(ctx, id)
	if err != nil {
		return nil, errors.GeneralError("Failed to fetch cascade-deleted nodepools: %s", err)
	}
	if svcErr := updateNodePoolStatusesForCascadeDelete(
		ctx,
		nodePools,
		s.nodePoolDao,
		s.adapterStatusDao,
		s.adapterConfig,
	); svcErr != nil {
		return nil, svcErr
	}

	return cluster, nil
}

func (s *sqlClusterService) FindByIDs(ctx context.Context, ids []string) (api.ClusterList, *errors.ServiceError) {
	clusters, err := s.clusterDao.FindByIDs(ctx, ids)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all clusters: %s", err)
	}
	return clusters, nil
}

func (s *sqlClusterService) All(ctx context.Context) (api.ClusterList, *errors.ServiceError) {
	clusters, err := s.clusterDao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all clusters: %s", err)
	}
	return clusters, nil
}

func (s *sqlClusterService) OnUpsert(ctx context.Context, id string) error {
	cluster, err := s.clusterDao.Get(ctx, id)
	if err != nil {
		return err
	}

	ctx = logger.WithClusterID(ctx, cluster.ID)
	logger.Info(ctx, "Perform idempotent operations on cluster")

	return nil
}

func (s *sqlClusterService) OnDelete(ctx context.Context, id string) error {
	ctx = logger.WithClusterID(ctx, id)
	logger.Info(ctx, "Cluster has been deleted")
	return nil
}

func clusterRefTime(c *api.Cluster) time.Time {
	if c == nil {
		return time.Time{}
	}
	if !c.UpdatedTime.IsZero() {
		return c.UpdatedTime
	}
	return c.CreatedTime
}

// UpdateClusterStatusFromAdapters recomputes aggregated Reconciled, Available, and per-adapter conditions
// from stored adapter rows and persists them to the cluster's status.
func (s *sqlClusterService) UpdateClusterStatusFromAdapters(
	ctx context.Context, clusterID string,
) (*api.Cluster, *errors.ServiceError) {
	cluster, err := s.clusterDao.Get(ctx, clusterID)
	if err != nil {
		return nil, handleGetError("Cluster", "id", clusterID, err)
	}

	adapterStatuses, err := s.adapterStatusDao.FindByResource(ctx, "Cluster", clusterID)
	if err != nil {
		return nil, errors.GeneralError("Failed to get adapter statuses: %s", err)
	}

	return s.recomputeAndSaveClusterStatus(ctx, cluster, adapterStatuses)
}

// recomputeAndSaveClusterStatus aggregates adapter reports into cluster conditions and
// persists only when the result differs from the current stored value.
func (s *sqlClusterService) recomputeAndSaveClusterStatus(
	ctx context.Context, cluster *api.Cluster, adapterStatuses api.AdapterStatusList,
) (*api.Cluster, *errors.ServiceError) {
	refTime := clusterRefTime(cluster)

	hasChildResources := false
	if cluster.DeletedTime != nil {
		exists, err := s.nodePoolDao.ExistsByOwner(ctx, cluster.ID)
		if err != nil {
			return nil, errors.GeneralError("Failed to check child node pools for status aggregation: %s", err)
		}
		hasChildResources = exists
	}

	reconciled, lastKnownReconciled, adapterConditions := AggregateResourceStatus(ctx, AggregateResourceStatusInput{
		ResourceGeneration: cluster.Generation,
		RefTime:            refTime,
		DeletedTime:        cluster.DeletedTime,
		PrevConditionsJSON: cluster.StatusConditions,
		RequiredAdapters:   s.adapterConfig.RequiredClusterAdapters(),
		AdapterStatuses:    adapterStatuses,
		HasChildResources:  hasChildResources,
	})

	// Ready mirrors Reconciled for backward compatibility (deprecated)
	ready := reconciled
	ready.Type = api.ConditionTypeReady

	allConditions := make([]api.ResourceCondition, 0, fixedConditionCount+len(adapterConditions))
	allConditions = append(allConditions, ready, reconciled, lastKnownReconciled)
	allConditions = append(allConditions, adapterConditions...)

	conditionsJSON, err := json.Marshal(allConditions)
	if err != nil {
		return nil, errors.GeneralError("Failed to marshal conditions: %s", err)
	}

	if bytes.Equal(cluster.StatusConditions, conditionsJSON) {
		return cluster, nil
	}

	if err := s.clusterDao.SaveStatusConditions(ctx, cluster.ID, conditionsJSON); err != nil {
		return nil, handleUpdateError("Cluster", err)
	}

	cluster.StatusConditions = conditionsJSON
	return cluster, nil
}

// ProcessAdapterStatus validates mandatory conditions, applies discard rules, upserts adapter
// status, and triggers aggregation when appropriate.
//
// DB call budget (non-deleted happy path):
//  1. GetForUpdate        — lock + fetch cluster
//  2. FindByResource      — all adapter statuses (existing status found in-memory)
//  3. UpsertWithExisting  — write adapter status
//  4. SaveStatusConditions — write updated cluster conditions (skipped when unchanged)
func (s *sqlClusterService) ProcessAdapterStatus(
	ctx context.Context, clusterID string, adapterStatus *api.AdapterStatus,
) (*api.AdapterStatus, *errors.ServiceError) {
	// 1. Acquire a row-level exclusive lock on the cluster for the duration of this
	// transaction. Concurrent adapter status updates for the same cluster are
	// serialized here: the second caller blocks until the first commits, ensuring
	// the aggregation step always reads a fully up-to-date set of adapter statuses.
	cluster, err := s.clusterDao.GetForUpdate(ctx, clusterID)
	if err != nil {
		return nil, handleGetError("Cluster", "id", clusterID, err)
	}

	allStatuses, err := s.adapterStatusDao.FindByResource(ctx, "Cluster", clusterID)
	if err != nil {
		return nil, errors.GeneralError("Failed to get adapter statuses: %s", err)
	}

	existingStatus := findAdapterStatusInList(allStatuses, adapterStatus.Adapter)

	conditions, triggerAggregation, svcErr := s.validateAndClassify(
		ctx, clusterID, adapterStatus, cluster, existingStatus,
	)
	if svcErr != nil {
		return nil, svcErr
	}
	if conditions == nil && !triggerAggregation {
		return nil, nil
	}

	adapterStatus.ResourceType = "Cluster"
	adapterStatus.ResourceID = clusterID
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

	if cluster.DeletedTime != nil {
		hardDeleted, hdErr := s.tryHardDeleteCluster(ctx, cluster, conditions, updatedStatuses)
		if hdErr != nil {
			return nil, hdErr
		}
		if hardDeleted {
			return upsertedStatus, nil
		}
	}

	// 4. Re-aggregate using data already in memory.
	if triggerAggregation {
		if _, aggregateErr := s.recomputeAndSaveClusterStatus(ctx, cluster, updatedStatuses); aggregateErr != nil {
			return nil, aggregateErr
		}
	}

	return upsertedStatus, nil
}

// validateAndClassify performs all stateless validation and discard-rule checks on an incoming
// adapter status. Returns the parsed conditions and whether aggregation should be triggered.
// Returns (nil, false, nil) when the update should be silently discarded.
func (s *sqlClusterService) validateAndClassify(
	ctx context.Context,
	clusterID string,
	adapterStatus *api.AdapterStatus,
	cluster *api.Cluster,
	existingStatus *api.AdapterStatus,
) ([]api.AdapterCondition, bool, *errors.ServiceError) {
	log := logger.With(logger.WithClusterID(ctx, clusterID), logger.FieldAdapter, adapterStatus.Adapter)

	if adapterStatus.ObservedGeneration > cluster.Generation {
		log.Debug("Discarding adapter status update: future generation")
		return nil, false, nil
	}

	if existingStatus != nil && adapterStatus.ObservedGeneration < existingStatus.ObservedGeneration {
		log.Debug("Discarding adapter status update: stale generation")
		return nil, false, nil
	}

	incomingObs := AdapterObservedTime(adapterStatus)
	if incomingObs.IsZero() {
		log.Debug("Discarding adapter status update: zero observed time")
		return nil, false, nil
	}

	if existingStatus != nil && adapterStatus.ObservedGeneration == existingStatus.ObservedGeneration {
		prevObs := AdapterObservedTime(existingStatus)
		if !prevObs.IsZero() && incomingObs.Before(prevObs) {
			log.Debug("Discarding adapter status update: stale observed time")
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
				log.Debug("Discarding adapter status update: subsequent Unknown Available")
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

// tryHardDeleteCluster checks whether all required adapters have reported Finalized=True for
// a soft-deleted cluster and, when there are no remaining node pools, permanently removes the
// cluster and all its adapter statuses. Returns true when the hard-delete was performed.
func (s *sqlClusterService) tryHardDeleteCluster(
	ctx context.Context,
	cluster *api.Cluster,
	conditions []api.AdapterCondition,
	allStatuses api.AdapterStatusList,
) (bool, *errors.ServiceError) {
	if !incomingReportedFinalized(conditions) {
		return false, nil
	}

	if !allAdaptersFinalized(s.adapterConfig.Required.Cluster, allStatuses, cluster.Generation) {
		return false, nil
	}

	hasNodePools, err := s.nodePoolDao.ExistsByOwner(ctx, cluster.ID)
	if err != nil {
		return false, errors.GeneralError("Failed to check nodepools during hard-delete: %s", err)
	}
	if hasNodePools {
		return false, nil
	}

	if err := s.adapterStatusDao.DeleteByResource(ctx, "Cluster", cluster.ID); err != nil {
		return false, errors.GeneralError("Failed to delete adapter statuses during hard-delete: %s", err)
	}
	if err := s.clusterDao.Delete(ctx, cluster.ID); err != nil {
		return false, errors.GeneralError("Failed to hard-delete cluster: %s", err)
	}

	logger.With(logger.WithClusterID(ctx, cluster.ID)).
		Info("Hard-deleted cluster after all required adapters reported Finalized=True and no nodepools exist")

	return true, nil
}

func findAdapterStatusInList(statuses api.AdapterStatusList, adapter string) *api.AdapterStatus {
	for _, s := range statuses {
		if s.Adapter == adapter {
			return s
		}
	}
	return nil
}

// replaceAdapterStatusInList returns a copy of the list with the entry for the given adapter
// replaced (or appended if not present). Used to build an up-to-date snapshot for aggregation
// after an upsert without re-querying.
func replaceAdapterStatusInList(statuses api.AdapterStatusList, updated *api.AdapterStatus) api.AdapterStatusList {
	result := make(api.AdapterStatusList, 0, len(statuses)+1)
	found := false
	for _, s := range statuses {
		if s.Adapter == updated.Adapter && s.ResourceType == updated.ResourceType && s.ResourceID == updated.ResourceID {
			result = append(result, updated)
			found = true
		} else {
			result = append(result, s)
		}
	}
	if !found {
		result = append(result, updated)
	}
	return result
}

// incomingReportedFinalized returns true when the adapter conditions contain Finalized=True.
func incomingReportedFinalized(conditions []api.AdapterCondition) bool {
	for _, cond := range conditions {
		if cond.Type == api.ConditionTypeFinalized && cond.Status == api.AdapterConditionTrue {
			return true
		}
	}
	return false
}
