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
	ExistsByOwner(ctx context.Context, kind, ownerID string) (bool, error)
	ExistsSoftDeletedByOwner(ctx context.Context, kinds []string, ownerID string) (bool, error)
	FindByKind(ctx context.Context, kind string) (api.ResourceList, error)
	FindByKindAndOwner(ctx context.Context, kind, ownerID string) (api.ResourceList, error)
	FindByKindAndOwnerForUpdate(ctx context.Context, kind, ownerID string) (api.ResourceList, error)
	GetByID(ctx context.Context, id string) (*api.Resource, error)
	ReplaceReferences(ctx context.Context, sourceID string, refs []api.ResourceReference) error
	FindReferencer(ctx context.Context, targetID string) (*api.ResourceSummary, error)
	ClearTargetReferences(ctx context.Context, targetID string) error
	FindSourceIDsByRef(ctx context.Context, refType, targetID string) ([]string, error)
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
	if err := g2.Preload("Conditions").Preload("Labels").Preload("References").
		Take(&resource, "kind = ? AND id = ?", kind, id).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

func (d *sqlResourceDao) GetForUpdate(ctx context.Context, kind, id string) (*api.Resource, error) {
	g2 := d.sessionFactory.New(ctx)
	var resource api.Resource
	if err := g2.Clauses(clause.Locking{Strength: "UPDATE"}).
		Preload("Conditions").Preload("Labels").Preload("References").
		Take(&resource, "kind = ? AND id = ?", kind, id).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

func (d *sqlResourceDao) GetByOwner(ctx context.Context, kind, id, ownerID string) (*api.Resource, error) {
	g2 := d.sessionFactory.New(ctx)
	var resource api.Resource
	if err := g2.Preload("Conditions").Preload("Labels").Preload("References").
		Take(&resource, "kind = ? AND id = ? AND owner_id = ?", kind, id, ownerID).Error; err != nil {
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

func (d *sqlResourceDao) ExistsByOwner(ctx context.Context, kind, ownerID string) (bool, error) {
	g2 := d.sessionFactory.New(ctx)
	var exists bool
	if err := g2.Raw(
		"SELECT EXISTS(SELECT 1 FROM resources WHERE kind = ? AND owner_id = ? AND deleted_time IS NULL)",
		kind, ownerID).Scan(&exists).Error; err != nil {
		return false, err
	}
	return exists, nil
}

func (d *sqlResourceDao) ExistsSoftDeletedByOwner(ctx context.Context, kinds []string, ownerID string) (bool, error) {
	if len(kinds) == 0 {
		return false, nil
	}
	g2 := d.sessionFactory.New(ctx)
	var exists bool
	if err := g2.Raw(
		"SELECT EXISTS(SELECT 1 FROM resources WHERE kind IN (?) AND owner_id = ? AND deleted_time IS NOT NULL)",
		kinds, ownerID).Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("failed to check soft-deleted children: %w", err)
	}
	return exists, nil
}

func (d *sqlResourceDao) FindByKind(ctx context.Context, kind string) (api.ResourceList, error) {
	g2 := d.sessionFactory.New(ctx)
	var resources api.ResourceList
	if err := g2.Preload("Labels").Preload("Conditions").Preload("References").
		Where("kind = ?", kind).Find(&resources).Error; err != nil {
		return nil, err
	}
	return resources, nil
}

func (d *sqlResourceDao) FindByKindAndOwner(ctx context.Context, kind, ownerID string) (api.ResourceList, error) {
	g2 := d.sessionFactory.New(ctx)
	var resources api.ResourceList
	if err := g2.Preload("Labels").Preload("Conditions").Preload("References").
		Where("kind = ? AND owner_id = ?", kind, ownerID).Find(&resources).Error; err != nil {
		return nil, err
	}
	return resources, nil
}

func (d *sqlResourceDao) GetByID(ctx context.Context, id string) (*api.Resource, error) {
	g2 := d.sessionFactory.New(ctx)
	var resource api.Resource
	if err := g2.Preload("Conditions").Preload("Labels").Preload("References").
		Take(&resource, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

func (d *sqlResourceDao) FindByKindAndOwnerForUpdate(
	ctx context.Context, kind, ownerID string,
) (api.ResourceList, error) {
	g2 := d.sessionFactory.New(ctx)
	var resources api.ResourceList
	if err := g2.Preload("Labels").Preload("Conditions").Preload("References").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("kind = ? AND owner_id = ?", kind, ownerID).Find(&resources).Error; err != nil {
		return nil, err
	}
	return resources, nil
}

func (d *sqlResourceDao) ReplaceReferences(
	ctx context.Context, sourceID string, refs []api.ResourceReference,
) error {
	g2 := d.sessionFactory.New(ctx)
	if err := g2.Where("source_id = ?", sourceID).Delete(&api.ResourceReference{}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	for i := range refs {
		refs[i].SourceID = sourceID
	}
	if len(refs) > 0 {
		if err := g2.Create(&refs).Error; err != nil {
			db.MarkForRollback(ctx, err)
			return err
		}
	}
	return nil
}

// FindReferencer returns the first non-deleted resource that references targetID,
// or nil if none exists. Used as an existence check for 409 conflict responses.
func (d *sqlResourceDao) FindReferencer(
	ctx context.Context, targetID string,
) (*api.ResourceSummary, error) {
	g2 := d.sessionFactory.New(ctx)
	var summary api.ResourceSummary
	err := g2.Model(&api.ResourceReference{}).
		Select("resources.kind, resources.name").
		Joins("JOIN resources ON resource_references.source_id = resources.id").
		Where("resource_references.target_id = ? AND resources.deleted_time IS NULL", targetID).
		Limit(1).
		Scan(&summary).Error
	if err != nil {
		return nil, err
	}
	if summary.Kind == "" {
		return nil, nil
	}
	return &summary, nil
}

// ClearTargetReferences removes all inbound references pointing at targetID.
// Called by forceDeleteResourceTree before hard-deleting a referenced target,
// because the target_id FK uses ON DELETE RESTRICT.
func (d *sqlResourceDao) ClearTargetReferences(ctx context.Context, targetID string) error {
	g2 := d.sessionFactory.New(ctx)
	if err := g2.Where("target_id = ?", targetID).Delete(&api.ResourceReference{}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlResourceDao) FindSourceIDsByRef(
	ctx context.Context, refType, targetID string,
) ([]string, error) {
	g2 := d.sessionFactory.New(ctx)
	var ids []string
	if err := g2.Model(&api.ResourceReference{}).
		Where("ref_type = ? AND target_id = ?", refType, targetID).
		Pluck("source_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}
