package dao

import (
	"context"
	"fmt"

	"gorm.io/gorm/clause"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
)

type ResourceDao interface {
	Get(ctx context.Context, kind, id string) (*api.Resource, error)
	GetForUpdate(ctx context.Context, kind, id string) (*api.Resource, error)
	GetByOwner(ctx context.Context, kind, id, ownerID string) (*api.Resource, error)
	Create(ctx context.Context, resource *api.Resource) (*api.Resource, error)
	Save(ctx context.Context, resource *api.Resource) error
	Delete(ctx context.Context, kind, id string) error
	CountByOwner(ctx context.Context, kind, ownerID string) (int64, error)
	FindByType(ctx context.Context, kind string) (api.ResourceList, error)
	FindByTypeAndOwner(ctx context.Context, kind, ownerID string) (api.ResourceList, error)
	FindByIDs(ctx context.Context, kind string, ids []string) (api.ResourceList, error)
}

var _ ResourceDao = &sqlResourceDao{}

type sqlResourceDao struct {
	sessionFactory db.SessionFactory
}

func NewResourceDao(sessionFactory db.SessionFactory) ResourceDao {
	return &sqlResourceDao{sessionFactory: sessionFactory}
}

func (d *sqlResourceDao) Get(ctx context.Context, kind, id string) (*api.Resource, error) {
	g2 := d.sessionFactory.New(ctx)
	var resource api.Resource
	if err := g2.Take(&resource, "kind = ? AND id = ?", kind, id).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

func (d *sqlResourceDao) GetForUpdate(ctx context.Context, kind, id string) (*api.Resource, error) {
	g2 := d.sessionFactory.New(ctx)
	var resource api.Resource
	if err := g2.Clauses(clause.Locking{Strength: "UPDATE"}).Take(
		&resource, "kind = ? AND id = ?", kind, id).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

func (d *sqlResourceDao) GetByOwner(ctx context.Context, kind, id, ownerID string) (*api.Resource, error) {
	g2 := d.sessionFactory.New(ctx)
	var resource api.Resource
	if err := g2.Take(&resource, "kind = ? AND id = ? AND owner_id = ?", kind, id, ownerID).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

func (d *sqlResourceDao) Create(ctx context.Context, resource *api.Resource) (*api.Resource, error) {
	if resource.OwnerID != nil {
		// If OwnerID is empty, convert to nil
		if *resource.OwnerID == "" {
			resource.OwnerID = nil
			resource.OwnerKind = nil
			resource.OwnerHref = nil
		} else if resource.OwnerKind == nil || *resource.OwnerKind == "" {
			return nil, fmt.Errorf("owner_kind is required when owner_id is set")
		}
	}
	g2 := d.sessionFactory.New(ctx)
	if err := g2.Omit(clause.Associations).Create(resource).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return resource, nil
}

func (d *sqlResourceDao) Save(ctx context.Context, resource *api.Resource) error {
	g2 := d.sessionFactory.New(ctx)
	if err := g2.Omit(clause.Associations).Save(resource).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlResourceDao) Delete(ctx context.Context, kind, id string) error {
	g2 := d.sessionFactory.New(ctx)
	if err := g2.Omit(clause.Associations).Where("kind = ?", kind).Delete(
		&api.Resource{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlResourceDao) CountByOwner(ctx context.Context, kind, ownerID string) (int64, error) {
	g2 := d.sessionFactory.New(ctx)
	var count int64
	if err := g2.Model(&api.Resource{}).Where(
		"kind = ? AND owner_id = ?", kind, ownerID).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (d *sqlResourceDao) FindByType(ctx context.Context, kind string) (api.ResourceList, error) {
	g2 := d.sessionFactory.New(ctx)
	var resources api.ResourceList
	if err := g2.Where("kind = ?", kind).Find(&resources).Error; err != nil {
		return nil, err
	}
	return resources, nil
}

func (d *sqlResourceDao) FindByTypeAndOwner(ctx context.Context, kind, ownerID string) (api.ResourceList, error) {
	g2 := d.sessionFactory.New(ctx)
	var resources api.ResourceList
	if err := g2.Where("kind = ? AND owner_id = ?", kind, ownerID).Find(&resources).Error; err != nil {
		return nil, err
	}
	return resources, nil
}

func (d *sqlResourceDao) FindByIDs(ctx context.Context, kind string, ids []string) (api.ResourceList, error) {
	g2 := d.sessionFactory.New(ctx)
	var resources api.ResourceList
	if err := g2.Where("kind = ? AND id in (?)", kind, ids).Find(&resources).Error; err != nil {
		return nil, err
	}
	return resources, nil
}
