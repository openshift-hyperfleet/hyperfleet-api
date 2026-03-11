package services

import (
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
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (api.ClusterList, *errors.ServiceError)

	FindByIDs(ctx context.Context, ids []string) (api.ClusterList, *errors.ServiceError)

	// Status aggregation
	UpdateClusterStatusFromAdapters(ctx context.Context, clusterID string) (*api.Cluster, *errors.ServiceError)

	// ProcessAdapterStatus handles the business logic for adapter status:
	// - First report: accepts Unknown Available condition, skips aggregation
	// - Subsequent reports: rejects Unknown Available condition
	// - Otherwise: upserts the status and triggers aggregation
	ProcessAdapterStatus(
		ctx context.Context, clusterID string, adapterStatus *api.AdapterStatus,
	) (*api.AdapterStatus, *errors.ServiceError)

	// idempotent functions for the control plane, but can also be called synchronously by any actor
	OnUpsert(ctx context.Context, id string) error
	OnDelete(ctx context.Context, id string) error
}

func NewClusterService(
	clusterDao dao.ClusterDao,
	adapterStatusDao dao.AdapterStatusDao,
	adapterConfig *config.AdapterRequirementsConfig,
) ClusterService {
	return &sqlClusterService{
		clusterDao:       clusterDao,
		adapterStatusDao: adapterStatusDao,
		adapterConfig:    adapterConfig,
	}
}

var _ ClusterService = &sqlClusterService{}

type sqlClusterService struct {
	clusterDao       dao.ClusterDao
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

	// REMOVED: Event creation - no event-driven components
	return updatedCluster, nil
}

func (s *sqlClusterService) Replace(ctx context.Context, cluster *api.Cluster) (*api.Cluster, *errors.ServiceError) {
	cluster, err := s.clusterDao.Replace(ctx, cluster)
	if err != nil {
		return nil, handleUpdateError("Cluster", err)
	}

	updatedCluster, svcErr := s.UpdateClusterStatusFromAdapters(ctx, cluster.ID)
	if svcErr != nil {
		return nil, svcErr
	}

	return updatedCluster, nil
}

func (s *sqlClusterService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.clusterDao.Delete(ctx, id); err != nil {
		return handleDeleteError("Cluster", errors.GeneralError("Unable to delete cluster: %s", err))
	}

	// REMOVED: Event creation - no event-driven components
	return nil
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

	logger.With(ctx, "cluster_id", cluster.ID).
		Info("Perform idempotent operations on cluster")

	return nil
}

func (s *sqlClusterService) OnDelete(ctx context.Context, id string) error {
	logger.With(ctx, "cluster_id", id).Info("Cluster has been deleted")
	return nil
}

// UpdateClusterStatusFromAdapters aggregates adapter statuses into cluster status.
// Uses time.Now() as the observed time (for generation-change recomputations).
// Called from Create/Replace, so isLifecycleChange=true (Available frozen, Ready resets).
func (s *sqlClusterService) UpdateClusterStatusFromAdapters(
	ctx context.Context, clusterID string,
) (*api.Cluster, *errors.ServiceError) {
	return s.updateClusterStatusFromAdapters(ctx, clusterID, time.Now(), true)
}

// updateClusterStatusFromAdapters is the internal implementation.
// observedTime is the triggering adapter's observed_time (its LastReportTime) and is used
// for transition timestamps in the synthetic conditions.
// isLifecycleChange=true freezes Available and resets Ready.lut=now (Create/Replace path).
// isLifecycleChange=false uses the normal adapter-report aggregation path.
func (s *sqlClusterService) updateClusterStatusFromAdapters(
	ctx context.Context, clusterID string, observedTime time.Time, isLifecycleChange bool,
) (*api.Cluster, *errors.ServiceError) {
	// Get the cluster
	cluster, err := s.clusterDao.Get(ctx, clusterID)
	if err != nil {
		return nil, handleGetError("Cluster", "id", clusterID, err)
	}

	// Get all adapter statuses for this cluster
	adapterStatuses, err := s.adapterStatusDao.FindByResource(ctx, "Cluster", clusterID)
	if err != nil {
		return nil, errors.GeneralError("Failed to get adapter statuses: %s", err)
	}

	now := time.Now()

	// Build the list of adapter ResourceConditions
	adapterConditions := buildAdapterResourceConditions(adapterStatuses)

	// Compute synthetic Available and Ready conditions
	availableCondition, readyCondition := BuildSyntheticConditions(
		ctx,
		cluster.StatusConditions,
		adapterStatuses,
		s.adapterConfig.RequiredClusterAdapters(),
		cluster.Generation,
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
	cluster.StatusConditions = conditionsJSON

	// Save the updated cluster
	cluster, err = s.clusterDao.Replace(ctx, cluster)
	if err != nil {
		return nil, handleUpdateError("Cluster", err)
	}

	return cluster, nil
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
func (s *sqlClusterService) ProcessAdapterStatus(
	ctx context.Context, clusterID string, adapterStatus *api.AdapterStatus,
) (*api.AdapterStatus, *errors.ServiceError) {
	return processAdapterStatus(ctx, "Cluster", clusterID, adapterStatus, s.adapterStatusDao,
		func(ctx context.Context) (int32, *errors.ServiceError) {
			cluster, err := s.clusterDao.Get(ctx, clusterID)
			if err != nil {
				return 0, handleGetError("Cluster", "id", clusterID, err)
			}
			return cluster.Generation, nil
		},
		func(ctx context.Context, observedTime time.Time) {
			if _, err := s.updateClusterStatusFromAdapters(ctx, clusterID, observedTime, false); err != nil {
				logger.With(ctx, "resource_type", "Cluster", "resource_id", clusterID).
					WithError(err).Warn("Failed to aggregate cluster status")
			}
		},
	)
}
