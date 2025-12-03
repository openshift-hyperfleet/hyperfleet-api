package services

import (
	"context"
	"encoding/json"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

//go:generate mockgen -source=cluster.go -package=services -destination=cluster_mock.go

type ClusterService interface {
	Get(ctx context.Context, id string) (*api.Cluster, *errors.ServiceError)
	Create(ctx context.Context, cluster *api.Cluster) (*api.Cluster, *errors.ServiceError)
	Replace(ctx context.Context, cluster *api.Cluster) (*api.Cluster, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	All(ctx context.Context) (api.ClusterList, *errors.ServiceError)

	FindByIDs(ctx context.Context, ids []string) (api.ClusterList, *errors.ServiceError)

	// Status aggregation
	UpdateClusterStatusFromAdapters(ctx context.Context, clusterID string) (*api.Cluster, *errors.ServiceError)

	// idempotent functions for the control plane, but can also be called synchronously by any actor
	OnUpsert(ctx context.Context, id string) error
	OnDelete(ctx context.Context, id string) error
}

func NewClusterService(clusterDao dao.ClusterDao, adapterStatusDao dao.AdapterStatusDao) ClusterService {
	return &sqlClusterService{
		clusterDao:       clusterDao,
		adapterStatusDao: adapterStatusDao,
	}
}

var _ ClusterService = &sqlClusterService{}

type sqlClusterService struct {
	clusterDao       dao.ClusterDao
	adapterStatusDao dao.AdapterStatusDao
}

func (s *sqlClusterService) Get(ctx context.Context, id string) (*api.Cluster, *errors.ServiceError) {
	cluster, err := s.clusterDao.Get(ctx, id)
	if err != nil {
		return nil, handleGetError("Cluster", "id", id, err)
	}
	return cluster, nil
}

func (s *sqlClusterService) Create(ctx context.Context, cluster *api.Cluster) (*api.Cluster, *errors.ServiceError) {
	cluster, err := s.clusterDao.Create(ctx, cluster)
	if err != nil {
		return nil, handleCreateError("Cluster", err)
	}

	// REMOVED: Event creation - no event-driven components
	return cluster, nil
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
	logger := logger.NewOCMLogger(ctx)

	cluster, err := s.clusterDao.Get(ctx, id)
	if err != nil {
		return err
	}

	logger.Infof("Do idempotent somethings with this cluster: %s", cluster.ID)

	return nil
}

func (s *sqlClusterService) OnDelete(ctx context.Context, id string) error {
	logger := logger.NewOCMLogger(ctx)
	logger.Infof("This cluster has been deleted: %s", id)
	return nil
}

// UpdateClusterStatusFromAdapters aggregates adapter statuses into cluster status
func (s *sqlClusterService) UpdateClusterStatusFromAdapters(ctx context.Context, clusterID string) (*api.Cluster, *errors.ServiceError) {
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

	// Build the list of ResourceCondition
	adapters := []openapi.ResourceCondition{}
	maxObservedGeneration := int32(0)

	for _, adapterStatus := range adapterStatuses {
		// Unmarshal Conditions from JSONB
		var conditions []openapi.AdapterCondition
		if err := json.Unmarshal(adapterStatus.Conditions, &conditions); err != nil {
			continue // Skip if can't unmarshal
		}

		// Find the "Available" condition
		var availableCondition *openapi.AdapterCondition
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
		condResource := openapi.ResourceCondition{
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

		// Track max observed generation
		if adapterStatus.ObservedGeneration > maxObservedGeneration {
			maxObservedGeneration = adapterStatus.ObservedGeneration
		}
	}

	// Compute overall phase using required adapters
	newPhase := ComputePhase(ctx, adapterStatuses, requiredClusterAdapters, cluster.Generation)

	// Calculate min(adapters[].last_report_time) for cluster.status.last_updated_time
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
	oldPhase := cluster.StatusPhase

	// Update cluster status fields
	now := time.Now()
	cluster.StatusPhase = newPhase
	cluster.StatusObservedGeneration = maxObservedGeneration

	// Marshal conditions to JSON
	conditionsJSON, err := json.Marshal(adapters)
	if err != nil {
		return nil, errors.GeneralError("Failed to marshal conditions: %s", err)
	}
	cluster.StatusConditions = conditionsJSON

	// Use min(adapters[].last_report_time) instead of now()
	// This ensures Sentinel triggers reconciliation when ANY adapter is stale
	if minLastUpdatedTime != nil {
		cluster.StatusLastUpdatedTime = minLastUpdatedTime
	} else {
		cluster.StatusLastUpdatedTime = &now
	}

	// Update last transition time only if phase changed
	if cluster.StatusLastTransitionTime == nil || oldPhase != newPhase {
		cluster.StatusLastTransitionTime = &now
	}

	// Save the updated cluster
	cluster, err = s.clusterDao.Replace(ctx, cluster)
	if err != nil {
		return nil, handleUpdateError("Cluster", err)
	}

	return cluster, nil
}
