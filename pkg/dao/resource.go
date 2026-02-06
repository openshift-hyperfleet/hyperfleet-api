package dao

import (
	"bytes"
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
)

// ResourceDao defines the data access interface for generic resources.
type ResourceDao interface {
	// Get retrieves a resource by ID.
	Get(ctx context.Context, id string) (*api.Resource, error)

	// GetByKindAndID retrieves a resource by kind and ID.
	GetByKindAndID(ctx context.Context, kind, id string) (*api.Resource, error)

	// GetByOwner retrieves a resource by kind, owner ID, and resource ID.
	GetByOwner(ctx context.Context, kind, ownerID, id string) (*api.Resource, error)

	// GetByOwnerAndName retrieves a resource by kind, owner ID, and name.
	GetByOwnerAndName(ctx context.Context, kind, ownerID, name string) (*api.Resource, error)

	// GetByKindAndName retrieves a root resource by kind and name.
	GetByKindAndName(ctx context.Context, kind, name string) (*api.Resource, error)

	// Create inserts a new resource.
	Create(ctx context.Context, resource *api.Resource) (*api.Resource, error)

	// Replace updates an existing resource with generation tracking.
	Replace(ctx context.Context, resource *api.Resource) (*api.Resource, error)

	// Delete soft-deletes a resource by ID.
	Delete(ctx context.Context, id string) error

	// DeleteByKindAndID soft-deletes a resource by kind and ID.
	DeleteByKindAndID(ctx context.Context, kind, id string) error

	// ListByKind returns all resources of a given kind.
	ListByKind(ctx context.Context, kind string, offset, limit int) (api.ResourceList, int64, error)

	// ListByOwner returns all resources of a given kind under an owner.
	ListByOwner(ctx context.Context, kind, ownerID string, offset, limit int) (api.ResourceList, int64, error)

	// FindByIDs returns resources matching the given IDs.
	FindByIDs(ctx context.Context, ids []string) (api.ResourceList, error)
}

var _ ResourceDao = &sqlResourceDao{}

type sqlResourceDao struct {
	sessionFactory *db.SessionFactory
}

// NewResourceDao creates a new ResourceDao instance.
func NewResourceDao(sessionFactory *db.SessionFactory) ResourceDao {
	return &sqlResourceDao{sessionFactory: sessionFactory}
}

func (d *sqlResourceDao) Get(ctx context.Context, id string) (*api.Resource, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var resource api.Resource
	if err := g2.Take(&resource, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

func (d *sqlResourceDao) GetByKindAndID(ctx context.Context, kind, id string) (*api.Resource, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var resource api.Resource
	if err := g2.Take(&resource, "kind = ? AND id = ?", kind, id).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

func (d *sqlResourceDao) GetByOwner(ctx context.Context, kind, ownerID, id string) (*api.Resource, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var resource api.Resource
	if err := g2.Take(&resource, "kind = ? AND owner_id = ? AND id = ?", kind, ownerID, id).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

func (d *sqlResourceDao) GetByOwnerAndName(ctx context.Context, kind, ownerID, name string) (*api.Resource, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var resource api.Resource
	if err := g2.Take(&resource, "kind = ? AND owner_id = ? AND name = ?", kind, ownerID, name).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

func (d *sqlResourceDao) GetByKindAndName(ctx context.Context, kind, name string) (*api.Resource, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var resource api.Resource
	if err := g2.Take(&resource, "kind = ? AND name = ? AND owner_id IS NULL", kind, name).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

func (d *sqlResourceDao) Create(ctx context.Context, resource *api.Resource) (*api.Resource, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(resource).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return resource, nil
}

func (d *sqlResourceDao) Replace(ctx context.Context, resource *api.Resource) (*api.Resource, error) {
	g2 := (*d.sessionFactory).New(ctx)

	// Get the existing resource to compare spec
	existing, err := d.Get(ctx, resource.ID)
	if err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}

	// Compare spec: if changed, increment generation
	if !bytes.Equal(existing.Spec, resource.Spec) {
		resource.Generation = existing.Generation + 1
	} else {
		// Spec unchanged, preserve generation
		resource.Generation = existing.Generation
	}

	// Save the resource
	if err := g2.Omit(clause.Associations).Save(resource).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return resource, nil
}

func (d *sqlResourceDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&api.Resource{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlResourceDao) DeleteByKindAndID(ctx context.Context, kind, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Where("kind = ? AND id = ?", kind, id).Delete(&api.Resource{}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlResourceDao) ListByKind(ctx context.Context, kind string, offset, limit int) (api.ResourceList, int64, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var resources api.ResourceList
	var total int64

	// Count total
	if err := g2.Model(&api.Resource{}).Where("kind = ? AND owner_id IS NULL", kind).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Fetch with pagination
	query := g2.Where("kind = ? AND owner_id IS NULL", kind).Order("created_time DESC")
	if limit > 0 {
		query = query.Offset(offset).Limit(limit)
	}
	if err := query.Find(&resources).Error; err != nil {
		return nil, 0, err
	}

	return resources, total, nil
}

func (d *sqlResourceDao) ListByOwner(ctx context.Context, kind, ownerID string, offset, limit int) (api.ResourceList, int64, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var resources api.ResourceList
	var total int64

	// Count total
	if err := g2.Model(&api.Resource{}).Where("kind = ? AND owner_id = ?", kind, ownerID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Fetch with pagination
	query := g2.Where("kind = ? AND owner_id = ?", kind, ownerID).Order("created_time DESC")
	if limit > 0 {
		query = query.Offset(offset).Limit(limit)
	}
	if err := query.Find(&resources).Error; err != nil {
		return nil, 0, err
	}

	return resources, total, nil
}

func (d *sqlResourceDao) FindByIDs(ctx context.Context, ids []string) (api.ResourceList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var resources api.ResourceList
	if len(ids) == 0 {
		return resources, nil
	}
	if err := g2.Where("id IN (?)", ids).Find(&resources).Error; err != nil {
		return nil, err
	}
	return resources, nil
}
