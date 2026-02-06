package services

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

//go:generate mockgen-v0.6.0 -source=resource.go -package=services -destination=resource_mock.go

// ResourceService defines the service interface for generic CRD-based resources.
type ResourceService interface {
	// Get retrieves a resource by ID.
	Get(ctx context.Context, kind, id string) (*api.Resource, *errors.ServiceError)

	// GetByOwner retrieves an owned resource by owner ID and resource ID.
	GetByOwner(ctx context.Context, kind, ownerID, id string) (*api.Resource, *errors.ServiceError)

	// Create creates a new resource.
	Create(ctx context.Context, resource *api.Resource, requiredAdapters []string) (*api.Resource, *errors.ServiceError)

	// Replace updates an existing resource.
	Replace(ctx context.Context, resource *api.Resource) (*api.Resource, *errors.ServiceError)

	// Delete soft-deletes a resource by ID.
	Delete(ctx context.Context, kind, id string) *errors.ServiceError

	// ListByKind returns all resources of a given kind.
	ListByKind(ctx context.Context, kind string, offset, limit int) (api.ResourceList, int64, *errors.ServiceError)

	// ListByOwner returns all resources of a given kind under an owner.
	ListByOwner(ctx context.Context, kind, ownerID string, offset, limit int) (api.ResourceList, int64, *errors.ServiceError)

	// Status aggregation
	UpdateResourceStatusFromAdapters(ctx context.Context, kind, resourceID string, requiredAdapters []string) (*api.Resource, *errors.ServiceError)

	// ProcessAdapterStatus handles the business logic for adapter status:
	// - If Available condition is "Unknown": returns (nil, nil) indicating no-op
	// - Otherwise: upserts the status and triggers aggregation
	ProcessAdapterStatus(
		ctx context.Context, kind, resourceID string, adapterStatus *api.AdapterStatus, requiredAdapters []string,
	) (*api.AdapterStatus, *errors.ServiceError)

	// Idempotent functions for control plane operations
	OnUpsert(ctx context.Context, kind, id string) error
	OnDelete(ctx context.Context, kind, id string) error
}

// NewResourceService creates a new ResourceService instance.
func NewResourceService(
	resourceDao dao.ResourceDao,
	adapterStatusDao dao.AdapterStatusDao,
) ResourceService {
	return &sqlResourceService{
		resourceDao:      resourceDao,
		adapterStatusDao: adapterStatusDao,
	}
}

var _ ResourceService = &sqlResourceService{}

type sqlResourceService struct {
	resourceDao      dao.ResourceDao
	adapterStatusDao dao.AdapterStatusDao
}

func (s *sqlResourceService) Get(ctx context.Context, kind, id string) (*api.Resource, *errors.ServiceError) {
	resource, err := s.resourceDao.GetByKindAndID(ctx, kind, id)
	if err != nil {
		return nil, handleGetError(kind, "id", id, err)
	}
	return resource, nil
}

func (s *sqlResourceService) GetByOwner(ctx context.Context, kind, ownerID, id string) (*api.Resource, *errors.ServiceError) {
	resource, err := s.resourceDao.GetByOwner(ctx, kind, ownerID, id)
	if err != nil {
		return nil, handleGetError(kind, "id", id, err)
	}
	return resource, nil
}

func (s *sqlResourceService) Create(ctx context.Context, resource *api.Resource, requiredAdapters []string) (*api.Resource, *errors.ServiceError) {
	if resource.Generation == 0 {
		resource.Generation = 1
	}

	resource, err := s.resourceDao.Create(ctx, resource)
	if err != nil {
		return nil, handleCreateError(resource.Kind, err)
	}

	// Trigger status aggregation after creation
	updatedResource, svcErr := s.UpdateResourceStatusFromAdapters(ctx, resource.Kind, resource.ID, requiredAdapters)
	if svcErr != nil {
		return nil, svcErr
	}

	return updatedResource, nil
}

func (s *sqlResourceService) Replace(ctx context.Context, resource *api.Resource) (*api.Resource, *errors.ServiceError) {
	resource, err := s.resourceDao.Replace(ctx, resource)
	if err != nil {
		return nil, handleUpdateError(resource.Kind, err)
	}
	return resource, nil
}

func (s *sqlResourceService) Delete(ctx context.Context, kind, id string) *errors.ServiceError {
	if err := s.resourceDao.DeleteByKindAndID(ctx, kind, id); err != nil {
		return handleDeleteError(kind, errors.GeneralError("Unable to delete resource: %s", err))
	}
	return nil
}

func (s *sqlResourceService) ListByKind(ctx context.Context, kind string, offset, limit int) (api.ResourceList, int64, *errors.ServiceError) {
	resources, total, err := s.resourceDao.ListByKind(ctx, kind, offset, limit)
	if err != nil {
		return nil, 0, errors.GeneralError("Unable to list %s resources: %s", kind, err)
	}
	return resources, total, nil
}

func (s *sqlResourceService) ListByOwner(ctx context.Context, kind, ownerID string, offset, limit int) (api.ResourceList, int64, *errors.ServiceError) {
	resources, total, err := s.resourceDao.ListByOwner(ctx, kind, ownerID, offset, limit)
	if err != nil {
		return nil, 0, errors.GeneralError("Unable to list %s resources for owner %s: %s", kind, ownerID, err)
	}
	return resources, total, nil
}

func (s *sqlResourceService) OnUpsert(ctx context.Context, kind, id string) error {
	resource, err := s.resourceDao.GetByKindAndID(ctx, kind, id)
	if err != nil {
		return err
	}

	ctx = logger.WithResourceID(ctx, resource.ID)
	ctx = logger.WithResourceType(ctx, resource.Kind)
	logger.Info(ctx, "Perform idempotent operations on resource")

	return nil
}

func (s *sqlResourceService) OnDelete(ctx context.Context, kind, id string) error {
	ctx = logger.WithResourceID(ctx, id)
	ctx = logger.WithResourceType(ctx, kind)
	logger.Info(ctx, "Resource has been deleted")
	return nil
}

// UpdateResourceStatusFromAdapters aggregates adapter statuses into resource status.
// It reuses the existing BuildSyntheticConditions logic.
func (s *sqlResourceService) UpdateResourceStatusFromAdapters(
	ctx context.Context, kind, resourceID string, requiredAdapters []string,
) (*api.Resource, *errors.ServiceError) {
	// Get the resource
	resource, err := s.resourceDao.GetByKindAndID(ctx, kind, resourceID)
	if err != nil {
		return nil, handleGetError(kind, "id", resourceID, err)
	}

	// Get all adapter statuses for this resource
	adapterStatuses, err := s.adapterStatusDao.FindByResource(ctx, kind, resourceID)
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
		resource.StatusConditions,
		adapterStatuses,
		requiredAdapters,
		resource.Generation,
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
	resource.StatusConditions = conditionsJSON

	// Save the updated resource
	resource, err = s.resourceDao.Replace(ctx, resource)
	if err != nil {
		return nil, handleUpdateError(kind, err)
	}

	return resource, nil
}

// ProcessAdapterStatus handles the business logic for adapter status.
// If Available condition is "Unknown", returns (nil, nil) indicating no-op.
// Otherwise, upserts the status and triggers aggregation.
func (s *sqlResourceService) ProcessAdapterStatus(
	ctx context.Context, kind, resourceID string, adapterStatus *api.AdapterStatus, requiredAdapters []string,
) (*api.AdapterStatus, *errors.ServiceError) {
	existingStatus, findErr := s.adapterStatusDao.FindByResourceAndAdapter(
		ctx, kind, resourceID, adapterStatus.Adapter,
	)
	if findErr != nil && !stderrors.Is(findErr, gorm.ErrRecordNotFound) {
		if !strings.Contains(findErr.Error(), errors.CodeNotFoundGeneric) {
			return nil, errors.GeneralError("Failed to get adapter status: %s", findErr)
		}
	}
	if existingStatus != nil && adapterStatus.ObservedGeneration < existingStatus.ObservedGeneration {
		// Discard stale status updates (older observed_generation)
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
			// Available condition is "Unknown", return nil to indicate no-op
			return nil, nil
		}
	}

	// Upsert the adapter status
	upsertedStatus, err := s.adapterStatusDao.Upsert(ctx, adapterStatus)
	if err != nil {
		return nil, handleCreateError("AdapterStatus", err)
	}

	// Only trigger aggregation when the adapter reported an Available condition
	if hasAvailableCondition {
		if _, aggregateErr := s.UpdateResourceStatusFromAdapters(
			ctx, kind, resourceID, requiredAdapters,
		); aggregateErr != nil {
			// Log error but don't fail the request - the status will be computed on next update
			ctx = logger.WithResourceID(ctx, resourceID)
			ctx = logger.WithResourceType(ctx, kind)
			logger.WithError(ctx, aggregateErr).Warn("Failed to aggregate resource status")
		}
	}

	return upsertedStatus, nil
}
