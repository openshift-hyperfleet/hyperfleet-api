package dao

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-hyperfleet/hyperfleet/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/db"
)

type AdapterStatusDao interface {
	Get(ctx context.Context, id string) (*api.AdapterStatus, error)
	Create(ctx context.Context, adapterStatus *api.AdapterStatus) (*api.AdapterStatus, error)
	Replace(ctx context.Context, adapterStatus *api.AdapterStatus) (*api.AdapterStatus, error)
	Delete(ctx context.Context, id string) error
	FindByResource(ctx context.Context, resourceType, resourceID string) (api.AdapterStatusList, error)
	FindByResourcePaginated(ctx context.Context, resourceType, resourceID string, offset, limit int) (api.AdapterStatusList, int64, error)
	FindByResourceAndAdapter(ctx context.Context, resourceType, resourceID, adapter string) (*api.AdapterStatus, error)
	All(ctx context.Context) (api.AdapterStatusList, error)
}

var _ AdapterStatusDao = &sqlAdapterStatusDao{}

type sqlAdapterStatusDao struct {
	sessionFactory *db.SessionFactory
}

func NewAdapterStatusDao(sessionFactory *db.SessionFactory) AdapterStatusDao {
	return &sqlAdapterStatusDao{sessionFactory: sessionFactory}
}

func (d *sqlAdapterStatusDao) Get(ctx context.Context, id string) (*api.AdapterStatus, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var adapterStatus api.AdapterStatus
	if err := g2.Take(&adapterStatus, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &adapterStatus, nil
}

func (d *sqlAdapterStatusDao) Create(ctx context.Context, adapterStatus *api.AdapterStatus) (*api.AdapterStatus, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(adapterStatus).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return adapterStatus, nil
}

func (d *sqlAdapterStatusDao) Replace(ctx context.Context, adapterStatus *api.AdapterStatus) (*api.AdapterStatus, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(adapterStatus).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return adapterStatus, nil
}

func (d *sqlAdapterStatusDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&api.AdapterStatus{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlAdapterStatusDao) FindByResource(ctx context.Context, resourceType, resourceID string) (api.AdapterStatusList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	statuses := api.AdapterStatusList{}
	if err := g2.Where("resource_type = ? AND resource_id = ?", resourceType, resourceID).Find(&statuses).Error; err != nil {
		return nil, err
	}
	return statuses, nil
}

func (d *sqlAdapterStatusDao) FindByResourcePaginated(ctx context.Context, resourceType, resourceID string, offset, limit int) (api.AdapterStatusList, int64, error) {
	g2 := (*d.sessionFactory).New(ctx)
	statuses := api.AdapterStatusList{}
	var total int64

	// Base query
	query := g2.Where("resource_type = ? AND resource_id = ?", resourceType, resourceID)

	// Get total count for pagination metadata
	if err := query.Model(&api.AdapterStatus{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination using OFFSET and LIMIT
	if err := query.Offset(offset).Limit(limit).Find(&statuses).Error; err != nil {
		return nil, 0, err
	}

	return statuses, total, nil
}

func (d *sqlAdapterStatusDao) FindByResourceAndAdapter(ctx context.Context, resourceType, resourceID, adapter string) (*api.AdapterStatus, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var adapterStatus api.AdapterStatus
	if err := g2.Where("resource_type = ? AND resource_id = ? AND adapter = ?", resourceType, resourceID, adapter).Take(&adapterStatus).Error; err != nil {
		return nil, err
	}
	return &adapterStatus, nil
}

func (d *sqlAdapterStatusDao) All(ctx context.Context) (api.AdapterStatusList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	statuses := api.AdapterStatusList{}
	if err := g2.Find(&statuses).Error; err != nil {
		return nil, err
	}
	return statuses, nil
}
