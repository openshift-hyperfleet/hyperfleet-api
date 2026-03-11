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

	ctx = logger.WithClusterID(ctx, cluster.ID)
	logger.Info(ctx, "Perform idempotent operations on cluster")

	return nil
}

func (s *sqlClusterService) OnDelete(ctx context.Context, id string) error {
	ctx = logger.WithClusterID(ctx, id)
	logger.Info(ctx, "Cluster has been deleted")
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
	existingStatus, findErr := s.adapterStatusDao.FindByResourceAndAdapter(
		ctx, "Cluster", clusterID, adapterStatus.Adapter,
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
		ctx = logger.WithClusterID(ctx, clusterID)
		logger.Info(ctx, fmt.Sprintf("Discarding adapter status update from %s: %s condition %s",
			adapterStatus.Adapter, errorType, conditionName))
		return nil, nil
	}

	// P3: discard if Available == Unknown (spec §2, all reports).
	for _, cond := range conditions {
		if cond.Type == api.ConditionTypeAvailable && cond.Status == api.AdapterConditionUnknown {
			ctx = logger.WithClusterID(ctx, clusterID)
			logger.Info(ctx, fmt.Sprintf("Discarding adapter status update from %s: Available=Unknown reports are not processed",
				adapterStatus.Adapter))
			return nil, nil
		}
	}

	// P1: discard if observed_generation is ahead of the current resource generation.
	// Checked after P2/P3 to avoid unnecessary resource fetches for invalid/Unknown reports.
	cluster, err := s.clusterDao.Get(ctx, clusterID)
	if err != nil {
		return nil, handleGetError("Cluster", "id", clusterID, err)
	}
	if adapterStatus.ObservedGeneration > cluster.Generation {
		ctx = logger.WithClusterID(ctx, clusterID)
		logger.Info(ctx, fmt.Sprintf(
			"Discarding adapter status update from %s: observed_generation %d > resource generation %d",
			adapterStatus.Adapter, adapterStatus.ObservedGeneration, cluster.Generation))
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
	if _, aggregateErr := s.updateClusterStatusFromAdapters(ctx, clusterID, observedTime, false); aggregateErr != nil {
		ctx = logger.WithClusterID(ctx, clusterID)
		logger.WithError(ctx, aggregateErr).Warn("Failed to aggregate cluster status")
	}

	return upsertedStatus, nil
}
