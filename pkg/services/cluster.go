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
	// - If Available condition is "Unknown": returns (nil, nil) indicating no-op
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

	// REMOVED: Event creation - no event-driven components
	return cluster, nil
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

// UpdateClusterStatusFromAdapters aggregates adapter statuses into cluster status
func (s *sqlClusterService) UpdateClusterStatusFromAdapters(
	ctx context.Context, clusterID string,
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
		cluster.StatusConditions,
		adapterStatuses,
		s.adapterConfig.RequiredClusterAdapters,
		cluster.Generation,
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
	cluster.StatusConditions = conditionsJSON

	// Save the updated cluster
	cluster, err = s.clusterDao.Replace(ctx, cluster)
	if err != nil {
		return nil, handleUpdateError("Cluster", err)
	}

	return cluster, nil
}

// ProcessAdapterStatus handles the business logic for adapter status:
// - If Available is "Unknown" and a status already exists: returns (nil, nil) as no-op
// - If Available is "Unknown" and no status exists (first report): upserts the status
// - Otherwise: upserts the status and triggers aggregation
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
		if cond.Type != "Available" {
			continue
		}

		hasAvailableCondition = true
		if cond.Status == api.AdapterConditionUnknown {
			if existingStatus != nil {
				// Available condition is "Unknown" and a status already exists, return nil to indicate no-op
				return nil, nil
			}
			// First report from this adapter: allow storing even with Available=Unknown
			// but skip aggregation since Unknown should not affect cluster-level conditions
			hasAvailableCondition = false
		}
	}

	// Upsert the adapter status
	upsertedStatus, err := s.adapterStatusDao.Upsert(ctx, adapterStatus)
	if err != nil {
		return nil, handleCreateError("AdapterStatus", err)
	}

	// Only trigger aggregation when the adapter reported an Available condition.
	// If the adapter status doesn't include Available (e.g. it only reports Ready/Progressing),
	// saving it should not overwrite the cluster's synthetic Available/Ready conditions.
	if hasAvailableCondition {
		if _, aggregateErr := s.UpdateClusterStatusFromAdapters(
			ctx, clusterID,
		); aggregateErr != nil {
			// Log error but don't fail the request - the status will be computed on next update
			ctx = logger.WithClusterID(ctx, clusterID)
			logger.WithError(ctx, aggregateErr).Warn("Failed to aggregate cluster status")
		}
	}

	return upsertedStatus, nil
}
