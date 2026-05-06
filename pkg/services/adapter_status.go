package services

import (
	"context"
	"encoding/json"
	e "errors"
	"time"

	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

//go:generate mockgen-v0.6.0 -source=adapter_status.go -package=services -destination=adapter_status_mock.go

type AdapterStatusService interface {
	Get(ctx context.Context, id string) (*api.AdapterStatus, *errors.ServiceError)
	Create(ctx context.Context, adapterStatus *api.AdapterStatus) (*api.AdapterStatus, *errors.ServiceError)
	Upsert(ctx context.Context, adapterStatus *api.AdapterStatus) (*api.AdapterStatus, *errors.ServiceError)
	Delete(ctx context.Context, id string) *errors.ServiceError
	FindByResource(
		ctx context.Context, resourceType, resourceID string,
	) (api.AdapterStatusList, *errors.ServiceError)
	FindByResourcePaginated(
		ctx context.Context, resourceType, resourceID string, listArgs *ListArguments,
	) (api.AdapterStatusList, int64, *errors.ServiceError)
	FindByResourceAndAdapter(
		ctx context.Context, resourceType, resourceID, adapter string,
	) (*api.AdapterStatus, *errors.ServiceError)
	All(ctx context.Context) (api.AdapterStatusList, *errors.ServiceError)
}

func NewAdapterStatusService(adapterStatusDao dao.AdapterStatusDao) AdapterStatusService {
	return &sqlAdapterStatusService{
		adapterStatusDao: adapterStatusDao,
	}
}

var _ AdapterStatusService = &sqlAdapterStatusService{}

type sqlAdapterStatusService struct {
	adapterStatusDao dao.AdapterStatusDao
}

func (s *sqlAdapterStatusService) Get(ctx context.Context, id string) (*api.AdapterStatus, *errors.ServiceError) {
	adapterStatus, err := s.adapterStatusDao.Get(ctx, id)
	if err != nil {
		return nil, handleGetError("AdapterStatus", "id", id, err)
	}
	return adapterStatus, nil
}

func (s *sqlAdapterStatusService) Create(
	ctx context.Context, adapterStatus *api.AdapterStatus,
) (*api.AdapterStatus, *errors.ServiceError) {
	adapterStatus, err := s.adapterStatusDao.Create(ctx, adapterStatus)
	if err != nil {
		return nil, handleCreateError("AdapterStatus", err)
	}
	return adapterStatus, nil
}

func (s *sqlAdapterStatusService) Delete(ctx context.Context, id string) *errors.ServiceError {
	if err := s.adapterStatusDao.Delete(ctx, id); err != nil {
		return handleDeleteError("AdapterStatus", errors.GeneralError("Unable to delete adapter status: %s", err))
	}
	return nil
}

func (s *sqlAdapterStatusService) FindByResource(
	ctx context.Context, resourceType, resourceID string,
) (api.AdapterStatusList, *errors.ServiceError) {
	statuses, err := s.adapterStatusDao.FindByResource(ctx, resourceType, resourceID)
	if err != nil {
		return nil, errors.GeneralError("Unable to get adapter statuses: %s", err)
	}
	return statuses, nil
}

func (s *sqlAdapterStatusService) FindByResourcePaginated(
	ctx context.Context, resourceType, resourceID string, listArgs *ListArguments,
) (api.AdapterStatusList, int64, *errors.ServiceError) {
	offset := (listArgs.Page - 1) * int(listArgs.Size)
	limit := int(listArgs.Size)

	statuses, total, err := s.adapterStatusDao.FindByResourcePaginated(ctx, resourceType, resourceID, offset, limit)
	if err != nil {
		return nil, 0, errors.GeneralError("Unable to get adapter statuses: %s", err)
	}

	return statuses, total, nil
}

func (s *sqlAdapterStatusService) FindByResourceAndAdapter(
	ctx context.Context, resourceType, resourceID, adapter string,
) (*api.AdapterStatus, *errors.ServiceError) {
	status, err := s.adapterStatusDao.FindByResourceAndAdapter(ctx, resourceType, resourceID, adapter)
	if err != nil {
		return nil, handleGetError("AdapterStatus", "adapter", adapter, err)
	}
	return status, nil
}

func (s *sqlAdapterStatusService) All(ctx context.Context) (api.AdapterStatusList, *errors.ServiceError) {
	statuses, err := s.adapterStatusDao.All(ctx)
	if err != nil {
		return nil, errors.GeneralError("Unable to get all adapter statuses: %s", err)
	}
	return statuses, nil
}

func (s *sqlAdapterStatusService) Upsert(
	ctx context.Context, adapterStatus *api.AdapterStatus,
) (*api.AdapterStatus, *errors.ServiceError) {
	existing, findErr := s.adapterStatusDao.FindByResourceAndAdapter(
		ctx, adapterStatus.ResourceType, adapterStatus.ResourceID, adapterStatus.Adapter,
	)
	if findErr != nil {
		if !e.Is(findErr, gorm.ErrRecordNotFound) {
			return nil, handleGetError("AdapterStatus", "adapter", adapterStatus.Adapter, findErr)
		}
		existing = nil
	}
	setConditionTransitionTimes(adapterStatus, existing)
	result, err := s.adapterStatusDao.Upsert(ctx, adapterStatus, existing)
	if err != nil {
		return nil, handleCreateError("AdapterStatus", err)
	}
	return result, nil
}

// setConditionTransitionTimes sets LastReportTime if unset and preserves condition
// LastTransitionTime for any condition whose status hasn't changed since the last report
// (Kubernetes condition semantic: LastTransitionTime only updates on status change).
func setConditionTransitionTimes(incoming *api.AdapterStatus, existing *api.AdapterStatus) {
	if incoming.LastReportTime.IsZero() {
		incoming.LastReportTime = time.Now()
	}
	if existing == nil || len(existing.Conditions) == 0 {
		return
	}

	var oldConds []api.AdapterCondition
	if err := json.Unmarshal(existing.Conditions, &oldConds); err != nil {
		return
	}
	var newConds []api.AdapterCondition
	if len(incoming.Conditions) == 0 || json.Unmarshal(incoming.Conditions, &newConds) != nil {
		return
	}

	oldByType := make(map[string]api.AdapterCondition, len(oldConds))
	for _, c := range oldConds {
		oldByType[c.Type] = c
	}
	for i := range newConds {
		if old, ok := oldByType[newConds[i].Type]; ok && old.Status == newConds[i].Status {
			newConds[i].LastTransitionTime = old.LastTransitionTime
		}
	}
	if b, err := json.Marshal(newConds); err == nil {
		incoming.Conditions = b
	}
}
