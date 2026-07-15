package services

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

type mockAdapterStatusDao struct {
	statuses            map[string]*api.AdapterStatus
	findByResourceErr   error
	deleteByResourceErr error
}

func newMockAdapterStatusDao() *mockAdapterStatusDao {
	return &mockAdapterStatusDao{
		statuses: make(map[string]*api.AdapterStatus),
	}
}

func (d *mockAdapterStatusDao) Get(ctx context.Context, id string) (*api.AdapterStatus, error) {
	if s, ok := d.statuses[id]; ok {
		return s, nil
	}
	return nil, errors.NotFound("AdapterStatus").AsError()
}

func (d *mockAdapterStatusDao) Create(ctx context.Context, status *api.AdapterStatus) (*api.AdapterStatus, error) {
	d.statuses[status.ID] = status
	return status, nil
}

func (d *mockAdapterStatusDao) Upsert(
	ctx context.Context, status *api.AdapterStatus, existing *api.AdapterStatus,
) (*api.AdapterStatus, error) {
	key := status.ResourceType + ":" + status.ResourceID + ":" + status.Adapter
	if existing != nil {
		isStoredFresherOrEqual := existing.ObservedGeneration > status.ObservedGeneration ||
			(existing.ObservedGeneration == status.ObservedGeneration &&
				!existing.LastReportTime.Before(status.LastReportTime))
		if isStoredFresherOrEqual {
			return existing, nil
		}

		status.ID = existing.ID
		if !existing.CreatedTime.IsZero() {
			status.CreatedTime = existing.CreatedTime
		}
	} else {
		status.ID = key
	}

	d.statuses[key] = status
	return status, nil
}

func (d *mockAdapterStatusDao) Delete(ctx context.Context, id string) error {
	delete(d.statuses, id)
	return nil
}

func (d *mockAdapterStatusDao) DeleteByResource(ctx context.Context, resourceType, resourceID string) error {
	if d.deleteByResourceErr != nil {
		return d.deleteByResourceErr
	}
	for key, s := range d.statuses {
		if s.ResourceType == resourceType && s.ResourceID == resourceID {
			delete(d.statuses, key)
		}
	}
	return nil
}

func (d *mockAdapterStatusDao) FindByResource(
	ctx context.Context,
	resourceType, resourceID string,
) (api.AdapterStatusList, error) {
	if d.findByResourceErr != nil {
		return nil, d.findByResourceErr
	}
	var result api.AdapterStatusList
	for _, s := range d.statuses {
		if s.ResourceType == resourceType && s.ResourceID == resourceID {
			result = append(result, s)
		}
	}
	return result, nil
}

func (d *mockAdapterStatusDao) FindByResourceIDs(
	ctx context.Context,
	resourceType string,
	resourceIDs []string,
) (api.AdapterStatusList, error) {
	var result api.AdapterStatusList
	resourceIDSet := make(map[string]bool, len(resourceIDs))
	for _, id := range resourceIDs {
		resourceIDSet[id] = true
	}
	for _, s := range d.statuses {
		if s.ResourceType == resourceType && resourceIDSet[s.ResourceID] {
			result = append(result, s)
		}
	}
	return result, nil
}

func (d *mockAdapterStatusDao) FindByResourcePaginated(
	ctx context.Context,
	resourceType, resourceID string,
	offset, limit int,
) (api.AdapterStatusList, int64, error) {
	statuses, err := d.FindByResource(ctx, resourceType, resourceID)
	if err != nil {
		return nil, 0, err
	}
	total := int64(len(statuses))
	if offset >= len(statuses) {
		return nil, total, nil
	}
	end := min(offset+limit, len(statuses))
	return statuses[offset:end], total, nil
}

func (d *mockAdapterStatusDao) FindByResourceAndAdapter(
	ctx context.Context,
	resourceType, resourceID, adapter string,
) (*api.AdapterStatus, error) {
	for _, s := range d.statuses {
		if s.ResourceType == resourceType && s.ResourceID == resourceID && s.Adapter == adapter {
			return s, nil
		}
	}
	return nil, errors.NotFound("AdapterStatus").AsError()
}

func (d *mockAdapterStatusDao) All(ctx context.Context) (api.AdapterStatusList, error) {
	var result api.AdapterStatusList
	for _, s := range d.statuses {
		result = append(result, s)
	}
	return result, nil
}
