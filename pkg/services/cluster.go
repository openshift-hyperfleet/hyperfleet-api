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

//go:generate mockgen-v0.6.0 -source=cluster.go -package=services -destination=cluster_mock.go

type ClusterService interface {
	Get(ctx context.Context, id string) (*api.Cluster, *errors.ServiceError)
	Create(ctx context.Context, cluster *api.Cluster) (*api.Cluster, *errors.ServiceError)
	Replace(ctx context.Context, cluster *api.Cluster) (*api.Cluster, *errors.ServiceError)
	SoftDelete(ctx context.Context, id string) (*api.Cluster, *errors.ServiceError)
	All(ctx context.Context) (api.ClusterList, *errors.ServiceError)

	FindByIDs(ctx context.Context, ids []string) (api.ClusterList, *errors.ServiceError)

	// UpdateClusterStatusFromAdapters recomputes aggregated Ready and Available from stored adapter rows.
	UpdateClusterStatusFromAdapters(ctx context.Context, clusterID string) (*api.Cluster, *errors.ServiceError)

	// ProcessAdapterStatus validates mandatory conditions, applies discard rules, upserts adapter status,
	// and triggers aggregation when appropriate.
	ProcessAdapterStatus(
		ctx context.Context, clusterID string, adapterStatus *api.AdapterStatus,
	) (*api.AdapterStatus, *errors.ServiceError)

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
	cluster, err := s.clusterDao.Get(ctx, id)
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
	if svcErr := batchUpdateNodePoolStatusesFromAdapters(
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

// UpdateClusterStatusFromAdapters recomputes aggregated Ready, Available, and per-adapter conditions
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

	refTime := clusterRefTime(cluster)
	ready, available, adapterConditions := AggregateResourceStatus(ctx, AggregateResourceStatusInput{
		ResourceGeneration: cluster.Generation,
		RefTime:            refTime,
		PrevConditionsJSON: cluster.StatusConditions,
		RequiredAdapters:   s.adapterConfig.RequiredClusterAdapters(),
		AdapterStatuses:    adapterStatuses,
	})

	allConditions := make([]api.ResourceCondition, 0, 2+len(adapterConditions))
	allConditions = append(allConditions, ready, available)
	allConditions = append(allConditions, adapterConditions...)

	conditionsJSON, err := json.Marshal(allConditions)
	if err != nil {
		return nil, errors.GeneralError("Failed to marshal conditions: %s", err)
	}

	if bytes.Equal(cluster.StatusConditions, conditionsJSON) {
		return cluster, nil
	}

	cluster.StatusConditions = conditionsJSON

	cluster, err = s.clusterDao.Replace(ctx, cluster)
	if err != nil {
		return nil, handleUpdateError("Cluster", err)
	}

	return cluster, nil
}

// ProcessAdapterStatus applies discard rules, then mandatory validation and upsert.
func (s *sqlClusterService) ProcessAdapterStatus(
	ctx context.Context, clusterID string, adapterStatus *api.AdapterStatus,
) (*api.AdapterStatus, *errors.ServiceError) {
	cluster, err := s.clusterDao.Get(ctx, clusterID)
	if err != nil {
		return nil, handleGetError("Cluster", "id", clusterID, err)
	}

	existingStatus, findErr := s.adapterStatusDao.FindByResourceAndAdapter(
		ctx, "Cluster", clusterID, adapterStatus.Adapter,
	)
	if findErr != nil && !stderrors.Is(findErr, gorm.ErrRecordNotFound) {
		if !strings.Contains(findErr.Error(), errors.CodeNotFoundGeneric) {
			return nil, errors.GeneralError("Failed to get adapter status: %s", findErr)
		}
	}

	if adapterStatus.ObservedGeneration > cluster.Generation {
		logger.With(logger.WithClusterID(ctx, clusterID), logger.FieldAdapter, adapterStatus.Adapter).
			Debug("Discarding adapter status update: future generation")
		return nil, nil
	}

	if existingStatus != nil && adapterStatus.ObservedGeneration < existingStatus.ObservedGeneration {
		logger.With(logger.WithClusterID(ctx, clusterID), logger.FieldAdapter, adapterStatus.Adapter).
			Debug("Discarding adapter status update: stale generation")
		return nil, nil
	}

	incomingObs := AdapterObservedTime(adapterStatus)
	if incomingObs.IsZero() {
		logger.With(logger.WithClusterID(ctx, clusterID), logger.FieldAdapter, adapterStatus.Adapter).
			Debug("Discarding adapter status update: zero observed time")
		return nil, nil
	}

	if existingStatus != nil && adapterStatus.ObservedGeneration == existingStatus.ObservedGeneration {
		prevObs := AdapterObservedTime(existingStatus)
		if !prevObs.IsZero() && incomingObs.Before(prevObs) {
			logger.With(logger.WithClusterID(ctx, clusterID), logger.FieldAdapter, adapterStatus.Adapter).
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
				logger.With(logger.WithClusterID(ctx, clusterID), logger.FieldAdapter, adapterStatus.Adapter).
					Debug("Discarding adapter status update: subsequent Unknown Available")
				return nil, nil
			}
			// First report may carry Unknown; store it but do not aggregate from it.
			triggerAggregation = false
			break
		}

		triggerAggregation = true
		break
	}

	adapterStatus.ResourceType = "Cluster"
	adapterStatus.ResourceID = clusterID

	upsertedStatus, err := s.adapterStatusDao.Upsert(ctx, adapterStatus)
	if err != nil {
		return nil, handleCreateError("AdapterStatus", err)
	}

	if triggerAggregation {
		if _, aggregateErr := s.UpdateClusterStatusFromAdapters(ctx, clusterID); aggregateErr != nil {
			return nil, aggregateErr
		}
	}

	return upsertedStatus, nil
}
