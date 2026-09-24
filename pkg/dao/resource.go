package dao

import (
	"context"
	"fmt"

	"gorm.io/gorm/clause"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/tenant"
)

type ResourceDao interface {
	Get(ctx context.Context, kind, id string) (*api.Resource, error)
	GetForUpdate(ctx context.Context, kind, id string) (*api.Resource, error)
	GetRowForUpdate(ctx context.Context, kind, id string) (*api.Resource, error)
	GetByOwner(ctx context.Context, kind, id, ownerID string) (*api.Resource, error)
	Create(ctx context.Context, resource *api.Resource) (*api.Resource, error)
	Save(ctx context.Context, resource *api.Resource) error
	Delete(ctx context.Context, kind, id string) error
	DeleteIDs(ctx context.Context, ids []string) error
	ExistsByOwner(ctx context.Context, kind, ownerID string) (bool, error)
	ExistsSoftDeletedByOwner(ctx context.Context, kinds []string, ownerID string) (bool, error)
	FindByKind(ctx context.Context, kind string) (api.ResourceList, error)
	FindByKindAndOwner(ctx context.Context, kind, ownerID string) (api.ResourceList, error)
	FindByKindAndOwnerForUpdate(ctx context.Context, kind, ownerID string) (api.ResourceList, error)
	FindChildrenForUpdate(ctx context.Context, ownerID string) (api.ResourceList, error)
	GetByID(ctx context.Context, id string) (*api.Resource, error)
	ReplaceReferences(ctx context.Context, sourceID string, refs []api.ResourceReference) error
	FindExternalReferenceCounts(ctx context.Context, targetIDs, sourceIDs []string) ([]ExternalReferenceCount, error)
	DeleteReferencesByTargets(ctx context.Context, targetIDs []string) error
	FindReferencers(ctx context.Context, targetID string) ([]api.ResourceSummary, error)
	ClearTargetReferences(ctx context.Context, targetID string) error
	FindSourceIDsByRef(ctx context.Context, refType, targetID string) ([]string, error)
}

// ExternalReferenceCount describes a surviving source reference type affected by deletion.
type ExternalReferenceCount struct {
	SourceKind string
	RefType    string
	Remaining  int64
}

var _ ResourceDao = &sqlResourceDao{}

type sqlResourceDao struct {
	sessionFactory db.SessionFactory
}

func NewResourceDao(sessionFactory db.SessionFactory) ResourceDao {
	return &sqlResourceDao{sessionFactory: sessionFactory}
}

func (d *sqlResourceDao) Get(ctx context.Context, kind, id string) (*api.Resource, error) {
	g2 := tenant.ScopeDB(d.sessionFactory.New(ctx), ctx)
	var resource api.Resource
	if err := g2.Preload("Conditions").Preload("Labels").Preload("References").
		Take(&resource, "kind = ? AND id = ?", kind, id).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

func (d *sqlResourceDao) GetForUpdate(ctx context.Context, kind, id string) (*api.Resource, error) {
	g2 := tenant.ScopeDB(d.sessionFactory.New(ctx), ctx)
	var resource api.Resource
	if err := g2.Clauses(clause.Locking{Strength: "UPDATE"}).
		Preload("Conditions").Preload("Labels").Preload("References").
		Take(&resource, "kind = ? AND id = ?", kind, id).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

// GetRowForUpdate locks a resource without loading its associations.
func (d *sqlResourceDao) GetRowForUpdate(ctx context.Context, kind, id string) (*api.Resource, error) {
	g2 := tenant.ScopeDB(d.sessionFactory.New(ctx), ctx)
	var resource api.Resource
	if err := g2.Clauses(clause.Locking{Strength: "UPDATE"}).
		Take(&resource, "kind = ? AND id = ?", kind, id).Error; err != nil {
		return nil, err
	}
	return &resource, nil
}

func (d *sqlResourceDao) GetByOwner(ctx context.Context, kind, id, ownerID string) (*api.Resource, error) {
	g2 := tenant.ScopeDB(d.sessionFactory.New(ctx), ctx)
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
		return nil, err
	}
	return resource, nil
}

func (d *sqlResourceDao) Save(ctx context.Context, resource *api.Resource) error {
	g2 := d.sessionFactory.New(ctx)
	if err := g2.Omit(clause.Associations).Save(resource).Error; err != nil {
		return err
	}
	return nil
}

func (d *sqlResourceDao) Delete(ctx context.Context, kind, id string) error {
	g2 := tenant.ScopeDB(d.sessionFactory.New(ctx), ctx)
	if err := g2.Omit(clause.Associations).Where("kind = ?", kind).Delete(
		&api.Resource{Meta: api.Meta{ID: id}}).Error; err != nil {
		return err
	}
	return nil
}

// DeleteIDs removes the given resource IDs within the caller's tenant scope.
func (d *sqlResourceDao) DeleteIDs(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return tenant.ScopeDB(d.sessionFactory.New(ctx), ctx).
		Omit(clause.Associations).Where("id IN ?", ids).Delete(&api.Resource{}).Error
}

func (d *sqlResourceDao) ExistsByOwner(ctx context.Context, kind, ownerID string) (bool, error) {
	g2 := d.sessionFactory.New(ctx)
	query := "SELECT EXISTS(SELECT 1 FROM resources WHERE kind = ? AND owner_id = ? AND deleted_time IS NULL"
	args := []any{kind, ownerID}
	if scopeClause, scopeArgs := tenant.ScopeClause(ctx); scopeClause != "" {
		query += " AND " + scopeClause
		args = append(args, scopeArgs...)
	}
	query += ")"
	var exists bool
	if err := g2.Raw(query, args...).Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("failed to check owner existence: %w", err)
	}
	return exists, nil
}

func (d *sqlResourceDao) ExistsSoftDeletedByOwner(ctx context.Context, kinds []string, ownerID string) (bool, error) {
	if len(kinds) == 0 {
		return false, nil
	}
	g2 := d.sessionFactory.New(ctx)
	query := "SELECT EXISTS(SELECT 1 FROM resources WHERE kind IN (?) AND owner_id = ? AND deleted_time IS NOT NULL"
	args := []any{kinds, ownerID}
	if scopeClause, scopeArgs := tenant.ScopeClause(ctx); scopeClause != "" {
		query += " AND " + scopeClause
		args = append(args, scopeArgs...)
	}
	query += ")"
	var exists bool
	if err := g2.Raw(query, args...).Scan(&exists).Error; err != nil {
		return false, fmt.Errorf("failed to check soft-deleted children: %w", err)
	}
	return exists, nil
}

func (d *sqlResourceDao) FindByKind(ctx context.Context, kind string) (api.ResourceList, error) {
	g2 := tenant.ScopeDB(d.sessionFactory.New(ctx), ctx)
	var resources api.ResourceList
	if err := g2.Preload("Labels").Preload("Conditions").Preload("References").
		Where("kind = ?", kind).Find(&resources).Error; err != nil {
		return nil, err
	}
	return resources, nil
}

func (d *sqlResourceDao) FindByKindAndOwner(ctx context.Context, kind, ownerID string) (api.ResourceList, error) {
	g2 := tenant.ScopeDB(d.sessionFactory.New(ctx), ctx)
	var resources api.ResourceList
	if err := g2.Preload("Labels").Preload("Conditions").Preload("References").
		Where("kind = ? AND owner_id = ?", kind, ownerID).Find(&resources).Error; err != nil {
		return nil, err
	}
	return resources, nil
}

func (d *sqlResourceDao) GetByID(ctx context.Context, id string) (*api.Resource, error) {
	g2 := tenant.ScopeDB(d.sessionFactory.New(ctx), ctx)
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
	g2 := tenant.ScopeDB(d.sessionFactory.New(ctx), ctx)
	var resources api.ResourceList
	if err := g2.Preload("Labels").Preload("Conditions").Preload("References").
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("kind = ? AND owner_id = ?", kind, ownerID).Find(&resources).Error; err != nil {
		return nil, err
	}
	return resources, nil
}

// FindChildrenForUpdate locks all owned rows in deterministic ID order.
func (d *sqlResourceDao) FindChildrenForUpdate(ctx context.Context, ownerID string) (api.ResourceList, error) {
	var resources api.ResourceList
	err := tenant.ScopeDB(d.sessionFactory.New(ctx), ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("owner_id = ?", ownerID).Order("id").Find(&resources).Error
	return resources, err
}

func (d *sqlResourceDao) ReplaceReferences(
	ctx context.Context, sourceID string, refs []api.ResourceReference,
) error {
	g2 := d.sessionFactory.New(ctx)
	if err := g2.Where("source_id = ?", sourceID).Delete(&api.ResourceReference{}).Error; err != nil {
		return err
	}
	for i := range refs {
		refs[i].SourceID = sourceID
	}
	if len(refs) > 0 {
		if err := g2.Create(&refs).Error; err != nil {
			return err
		}
	}
	return nil
}

// FindReferencers returns the list of resources that references targetID,
// or nil if none exists. Used as an existence check for 409 conflict responses.
func (d *sqlResourceDao) FindReferencers(
	ctx context.Context, targetID string,
) ([]api.ResourceSummary, error) {
	g2 := d.sessionFactory.New(ctx).Model(&api.ResourceReference{}).
		Select("resources.kind, resources.name").
		Joins("JOIN resources ON resource_references.source_id = resources.id").
		Where("resource_references.target_id = ? AND resources.deleted_time IS NULL", targetID)
	if scopeClause, scopeArgs := tenant.ScopeClause(ctx); scopeClause != "" {
		if len(scopeArgs) > 0 {
			// Qualify column predicates with "resources.": the base model is
			// ResourceReference, only the joined resources table has tenancy.
			scopeClause = "resources." + scopeClause
		}
		g2 = g2.Where(scopeClause, scopeArgs...)
	}
	var summaries []api.ResourceSummary
	if err := g2.Scan(&summaries).Error; err != nil {
		return nil, err
	}
	return summaries, nil
}

// ClearTargetReferences removes all inbound references pointing at targetID.
// Called by forceDeleteResourceTree before hard-deleting a referenced target,
// because the target_id FK uses ON DELETE RESTRICT.
func (d *sqlResourceDao) ClearTargetReferences(ctx context.Context, targetID string) error {
	g2 := d.sessionFactory.New(ctx)
	if err := g2.Where("target_id = ?", targetID).Delete(&api.ResourceReference{}).Error; err != nil {
		return err
	}
	return nil
}

// FindExternalReferenceCounts locks surviving sources, then counts their
// remaining references per source and type. The count is unscoped: inbound
// target references can originate in any tenant.
func (d *sqlResourceDao) FindExternalReferenceCounts(
	ctx context.Context, targetIDs, sourceIDs []string,
) ([]ExternalReferenceCount, error) {
	if len(targetIDs) == 0 {
		return nil, nil
	}
	lockQuery := d.sessionFactory.New(ctx).Model(&api.Resource{}).
		Select("id").
		Where("id IN (SELECT source_id FROM resource_references WHERE target_id IN ?)", targetIDs)
	if len(sourceIDs) > 0 {
		lockQuery = lockQuery.Where("id NOT IN ?", sourceIDs)
	}
	var lockedSources []api.Resource
	if err := lockQuery.Order("id").Clauses(clause.Locking{Strength: "UPDATE"}).
		Find(&lockedSources).Error; err != nil {
		return nil, err
	}
	if len(lockedSources) == 0 {
		return nil, nil
	}

	g2 := d.sessionFactory.New(ctx).Table("resource_references AS refs").
		Select(`resources.kind AS source_kind, refs.ref_type,
			COUNT(*) FILTER (WHERE refs.target_id NOT IN ?) AS remaining`, targetIDs).
		Joins("JOIN resources ON refs.source_id = resources.id").
		Where(`EXISTS (SELECT 1 FROM resource_references AS affected
			WHERE affected.source_id = refs.source_id AND affected.ref_type = refs.ref_type
			AND affected.target_id IN ?)`, targetIDs)
	if len(sourceIDs) > 0 {
		g2 = g2.Where("refs.source_id NOT IN ?", sourceIDs)
	}
	var counts []ExternalReferenceCount
	if err := g2.Group("refs.source_id, resources.kind, refs.ref_type").
		Scan(&counts).Error; err != nil {
		return nil, err
	}
	return counts, nil
}

func (d *sqlResourceDao) DeleteReferencesByTargets(ctx context.Context, targetIDs []string) error {
	if len(targetIDs) == 0 {
		return nil
	}
	return d.sessionFactory.New(ctx).Where("target_id IN ?", targetIDs).
		Delete(&api.ResourceReference{}).Error
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
