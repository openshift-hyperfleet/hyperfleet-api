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

func NewClusterService(lockFactory db.LockFactory, clusterDao dao.ClusterDao, adapterStatusDao dao.AdapterStatusDao, events EventService) ClusterService {
	return &sqlClusterService{
		lockFactory:      lockFactory,
		clusterDao:       clusterDao,
		adapterStatusDao: adapterStatusDao,
		events:           events,
	}
}

var _ ClusterService = &sqlClusterService{}

type sqlClusterService struct {
	lockFactory      db.LockFactory
	clusterDao       dao.ClusterDao
	adapterStatusDao dao.AdapterStatusDao
	events           EventService
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

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "Clusters",
		SourceID:  cluster.ID,
		EventType: api.CreateEventType,
	})
	if evErr != nil {
		return nil, handleCreateError("Cluster", evErr)
	}

	return cluster, nil
}

func (s *sqlClusterService) Replace(ctx context.Context, cluster *api.Cluster) (*api.Cluster, *errors.ServiceError) {
	cluster, err := s.clusterDao.Replace(ctx, cluster)
	if err != nil {
		return nil, handleUpdateError("Cluster", err)
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "Clusters",
		SourceID:  cluster.ID,
		EventType: api.UpdateEventType,
	})
	if evErr != nil {
		return nil, handleUpdateError("Cluster", evErr)
	}

	return cluster, nil
}

func (s *sqlClusterService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.clusterDao.Delete(ctx, id); err != nil {
		return handleDeleteError("Cluster", errors.GeneralError("Unable to delete cluster: %s", err))
	}

	_, evErr := s.events.Create(ctx, &api.Event{
		Source:    "Clusters",
		SourceID:  id,
		EventType: api.DeleteEventType,
	})
	if evErr != nil {
		return handleDeleteError("Cluster", evErr)
	}

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

	// Update cluster status fields
	now := time.Now()
	cluster.StatusPhase = phase
	cluster.StatusObservedGeneration = maxObservedGeneration

	// Marshal adapters to JSON
	adaptersJSON, err := json.Marshal(adapters)
	if err != nil {
		return nil, errors.GeneralError("Failed to marshal adapters: %s", err)
	}
	cluster.StatusAdapters = adaptersJSON
	cluster.StatusUpdatedAt = &now

	// Update last transition time if phase changed
	if cluster.StatusLastTransitionTime == nil || cluster.StatusPhase != phase {
		cluster.StatusLastTransitionTime = &now
	}

	// Save the updated cluster
	cluster, err = s.clusterDao.Replace(ctx, cluster)
	if err != nil {
		return nil, handleUpdateError("Cluster", err)
	}

	return cluster, nil
}
