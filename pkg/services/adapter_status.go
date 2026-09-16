package services

import (
	"context"
	"encoding/json"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

//go:generate go tool -modfile=../../tools/go.mod mockgen -source=adapter_status.go -package=services -destination=adapter_status_mock.go

type AdapterStatusService interface {
	FindByResourcePaginated(
		ctx context.Context, resourceType, resourceID string, listArgs *ListArguments,
	) (api.AdapterStatusList, int64, *errors.ServiceError)
}

func NewAdapterStatusService(adapterStatusDao dao.AdapterStatusDao) AdapterStatusService {
	return &sqlAdapterStatusService{adapterStatusDao: adapterStatusDao}
}

var _ AdapterStatusService = &sqlAdapterStatusService{}

type sqlAdapterStatusService struct {
	adapterStatusDao dao.AdapterStatusDao
}

func (s *sqlAdapterStatusService) FindByResourcePaginated(
	ctx context.Context, resourceType, resourceID string, listArgs *ListArguments,
) (api.AdapterStatusList, int64, *errors.ServiceError) {
	offset := int((listArgs.Page - 1) * listArgs.Size)
	limit := int(listArgs.Size)

	statuses, total, err := s.adapterStatusDao.FindByResourcePaginated(ctx, resourceType, resourceID, offset, limit)
	if err != nil {
		return nil, 0, errors.GeneralError("Unable to get adapter statuses: %s", err)
	}

	return statuses, total, nil
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
