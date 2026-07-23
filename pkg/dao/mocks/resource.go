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

func (d *resourceDaoMock) ExistsByOwner(_ context.Context, kind, ownerID string) (bool, error) {
	for _, r := range d.resources {
		if r.Kind == kind && r.OwnerID != nil && *r.OwnerID == ownerID && r.DeletedTime == nil {
			return true, nil
		}
	}
	return false, nil
}

func (d *resourceDaoMock) ExistsSoftDeletedByOwner(_ context.Context, kinds []string, ownerID string) (bool, error) {
	if len(kinds) == 0 {
		return false, nil
	}
	kindSet := make(map[string]bool, len(kinds))
	for _, k := range kinds {
		kindSet[k] = true
	}
	for _, r := range d.resources {
		if kindSet[r.Kind] && r.OwnerID != nil && *r.OwnerID == ownerID && r.DeletedTime != nil {
			return true, nil
		}
	}
	return false, nil
}

func (d *resourceDaoMock) FindByKind(_ context.Context, kind string) (api.ResourceList, error) {
	var result api.ResourceList
	for _, r := range d.resources {
		if r.Kind == kind {
			result = append(result, r)
		}
	}
	return result, nil
}

func (d *resourceDaoMock) FindByKindAndOwner(_ context.Context, kind, ownerID string) (api.ResourceList, error) {
	var result api.ResourceList
	for _, r := range d.resources {
		if r.Kind == kind && r.OwnerID != nil && *r.OwnerID == ownerID {
			result = append(result, r)
		}
	}
	return result, nil
}

func (d *resourceDaoMock) FindByKindAndOwnerForUpdate(
	ctx context.Context, kind, ownerID string,
) (api.ResourceList, error) {
	return d.FindByKindAndOwner(ctx, kind, ownerID)
}

func (d *resourceDaoMock) GetByID(_ context.Context, id string) (*api.Resource, error) {
	for _, r := range d.resources {
		if r.ID == id {
			return r, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *resourceDaoMock) ReplaceReferences(_ context.Context, _ string, _ []api.ResourceReference) error {
	return nil
}

func (d *resourceDaoMock) FindReferencers(_ context.Context, _ string) ([]api.ResourceSummary, error) {
	return nil, nil
}

func (d *resourceDaoMock) ClearTargetReferences(_ context.Context, _ string) error {
	return nil
}

func (d *resourceDaoMock) FindSourceIDsByRef(_ context.Context, _, _ string) ([]string, error) {
	return nil, nil
}
