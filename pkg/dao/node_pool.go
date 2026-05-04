package dao

import (
	"bytes"
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
)

type NodePoolDao interface {
	Get(ctx context.Context, id string) (*api.NodePool, error)
	GetForUpdate(ctx context.Context, id string) (*api.NodePool, error)
	Create(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, error)
	Replace(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, error)
	Save(ctx context.Context, nodePool *api.NodePool) error
	SaveStatusConditions(ctx context.Context, id string, statusConditions []byte) error
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (api.NodePoolList, error)
	FindByOwner(ctx context.Context, ownerID string) (api.NodePoolList, error)
	FindSoftDeletedByOwner(ctx context.Context, ownerID string) (api.NodePoolList, error)
	SoftDeleteByOwner(ctx context.Context, ownerID string, t time.Time, deletedBy string) error
	UpdateStatusConditionsByIDs(ctx context.Context, updates map[string][]byte) error
	ExistsByOwner(ctx context.Context, ownerID string) (bool, error)
	All(ctx context.Context) (api.NodePoolList, error)
}

var _ NodePoolDao = &sqlNodePoolDao{}

type sqlNodePoolDao struct {
	sessionFactory *db.SessionFactory
}

func NewNodePoolDao(sessionFactory *db.SessionFactory) NodePoolDao {
	return &sqlNodePoolDao{sessionFactory: sessionFactory}
}

func (d *sqlNodePoolDao) Get(ctx context.Context, id string) (*api.NodePool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var nodePool api.NodePool
	if err := g2.Take(&nodePool, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &nodePool, nil
}

func (d *sqlNodePoolDao) GetForUpdate(ctx context.Context, id string) (*api.NodePool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var nodePool api.NodePool
	if err := g2.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&nodePool, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &nodePool, nil
}

func (d *sqlNodePoolDao) SaveStatusConditions(ctx context.Context, id string, statusConditions []byte) error {
	g2 := (*d.sessionFactory).New(ctx)
	result := g2.Model(&api.NodePool{}).Where("id = ?", id).Update("status_conditions", statusConditions)
	if result.Error != nil {
		db.MarkForRollback(ctx, result.Error)
		return result.Error
	}
	return nil
}

func (d *sqlNodePoolDao) Create(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(nodePool).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return nodePool, nil
}

func (d *sqlNodePoolDao) Replace(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, error) {
	g2 := (*d.sessionFactory).New(ctx)

	// Get the existing nodePool to compare spec
	existing, err := d.Get(ctx, nodePool.ID)
	if err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}

	// Compare spec and labels: if either changed, increment generation.
	// Aggregated conditions are recomputed in the service layer.
	if !bytes.Equal(existing.Spec, nodePool.Spec) || !bytes.Equal(existing.Labels, nodePool.Labels) {
		nodePool.Generation = existing.Generation + 1
	} else {
		nodePool.Generation = existing.Generation
	}

	// Save the nodePool
	if err := g2.Omit(clause.Associations).Save(nodePool).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return nodePool, nil
}

func (d *sqlNodePoolDao) Save(ctx context.Context, nodePool *api.NodePool) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(nodePool).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlNodePoolDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&api.NodePool{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlNodePoolDao) SoftDeleteByOwner(ctx context.Context, ownerID string, t time.Time, deletedBy string) error {
	g2 := (*d.sessionFactory).New(ctx)
	result := g2.Model(&api.NodePool{}).
		Where("owner_id = ? AND deleted_time IS NULL", ownerID).
		Updates(map[string]interface{}{
			"deleted_time": t,
			"deleted_by":   deletedBy,
			"generation":   gorm.Expr("generation + 1"),
		})
	if result.Error != nil {
		db.MarkForRollback(ctx, result.Error)
		return result.Error
	}
	return nil
}

func (d *sqlNodePoolDao) FindSoftDeletedByOwner(ctx context.Context, ownerID string) (api.NodePoolList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var nodePools api.NodePoolList
	if err := g2.Where("owner_id = ? AND deleted_time IS NOT NULL", ownerID).Find(&nodePools).Error; err != nil {
		return nil, err
	}
	return nodePools, nil
}

func (d *sqlNodePoolDao) FindByIDs(ctx context.Context, ids []string) (api.NodePoolList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	nodePools := api.NodePoolList{}
	if err := g2.Where("id in (?)", ids).Find(&nodePools).Error; err != nil {
		return nil, err
	}
	return nodePools, nil
}

func (d *sqlNodePoolDao) FindByOwner(ctx context.Context, ownerID string) (api.NodePoolList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var nodePools api.NodePoolList
	if err := g2.Where("owner_id = ?", ownerID).Find(&nodePools).Error; err != nil {
		return nil, err
	}
	return nodePools, nil
}

func (d *sqlNodePoolDao) UpdateStatusConditionsByIDs(ctx context.Context, updates map[string][]byte) error {
	g2 := (*d.sessionFactory).New(ctx)
	if len(updates) == 0 {
		return nil
	}

	for id, statusConditions := range updates {
		result := g2.Model(&api.NodePool{}).
			Where("id = ?", id).
			Update("status_conditions", statusConditions)
		if result.Error != nil {
			db.MarkForRollback(ctx, result.Error)
			return result.Error
		}
	}
	return nil
}

func (d *sqlNodePoolDao) ExistsByOwner(ctx context.Context, ownerID string) (bool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var exists bool

	query := "SELECT EXISTS(SELECT 1 FROM node_pools WHERE owner_id = ?)"
	if err := g2.Raw(query, ownerID).Scan(&exists).Error; err != nil {
		return false, err
	}
	return exists, nil
}

func (d *sqlNodePoolDao) All(ctx context.Context) (api.NodePoolList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	nodePools := api.NodePoolList{}
	if err := g2.Find(&nodePools).Error; err != nil {
		return nil, err
	}
	return nodePools, nil
}
