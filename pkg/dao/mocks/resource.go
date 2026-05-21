package mocks

import (
	"context"

	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
)

var _ dao.ResourceDao = &resourceDaoMock{}

type resourceDaoMock struct {
	resources api.ResourceList
}

func (d *resourceDaoMock) Get(_ context.Context, kind, id string) (*api.Resource, error) {
	for _, r := range d.resources {
		if r.ID == id && r.Kind == kind {
			return r, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *resourceDaoMock) GetForUpdate(ctx context.Context, kind, id string) (*api.Resource, error) {
	return d.Get(ctx, kind, id)
}

func (d *resourceDaoMock) GetByOwner(_ context.Context, kind, id, ownerID string) (*api.Resource, error) {
	for _, r := range d.resources {
		if r.ID == id && r.Kind == kind && r.OwnerID != nil && *r.OwnerID == ownerID {
			return r, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *resourceDaoMock) Create(_ context.Context, resource *api.Resource) (*api.Resource, error) {
	d.resources = append(d.resources, resource)
	return resource, nil
}

func (d *resourceDaoMock) Save(_ context.Context, resource *api.Resource) error {
	for i, r := range d.resources {
		if r.ID == resource.ID {
			d.resources[i] = resource
			return nil
		}
	}
	d.resources = append(d.resources, resource)
	return nil
}

func (d *resourceDaoMock) Delete(_ context.Context, kind, id string) error {
	for i, r := range d.resources {
		if r.ID == id && r.Kind == kind {
			d.resources = append(d.resources[:i], d.resources[i+1:]...)
			return nil
		}
	}
	return nil
}

func (d *resourceDaoMock) CountByOwner(_ context.Context, kind, ownerID string) (int64, error) {
	var count int64
	for _, r := range d.resources {
		if r.Kind == kind && r.OwnerID != nil && *r.OwnerID == ownerID {
			count++
		}
	}
	return count, nil
}

func (d *resourceDaoMock) FindByType(_ context.Context, kind string) (api.ResourceList, error) {
	var result api.ResourceList
	for _, r := range d.resources {
		if r.Kind == kind {
			result = append(result, r)
		}
	}
	return result, nil
}

func (d *resourceDaoMock) FindByTypeAndOwner(_ context.Context, kind, ownerID string) (api.ResourceList, error) {
	var result api.ResourceList
	for _, r := range d.resources {
		if r.Kind == kind && r.OwnerID != nil && *r.OwnerID == ownerID {
			result = append(result, r)
		}
	}
	return result, nil
}

func (d *resourceDaoMock) FindByIDs(_ context.Context, kind string, ids []string) (api.ResourceList, error) {
	idSet := make(map[string]bool, len(ids))
	for _, id := range ids {
		idSet[id] = true
	}
	var result api.ResourceList
	for _, r := range d.resources {
		if r.Kind == kind && idSet[r.ID] {
			result = append(result, r)
		}
	}
	return result, nil
}
