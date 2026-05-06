package dao

import (
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
)

type NodePoolDao interface {
	Get(ctx context.Context, id string) (*api.NodePool, error)
	GetByIDAndOwner(ctx context.Context, id string, ownerID string) (*api.NodePool, error)
	GetForUpdate(ctx context.Context, id string) (*api.NodePool, error)
	Create(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, error)
	Save(ctx context.Context, nodePool *api.NodePool) error
	SaveStatusConditions(ctx context.Context, id string, statusConditions []byte) error
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (api.NodePoolList, error)
	FindByOwner(ctx context.Context, ownerID string) (api.NodePoolList, error)
	SaveAll(ctx context.Context, nodePools api.NodePoolList) error
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

func (d *sqlNodePoolDao) GetByIDAndOwner(ctx context.Context, id string, ownerID string) (*api.NodePool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var nodePool api.NodePool
	if err := g2.Take(&nodePool, "id = ? AND owner_id = ?", id, ownerID).Error; err != nil {
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

func (d *sqlNodePoolDao) SaveAll(ctx context.Context, nodePools api.NodePoolList) error {
	if len(nodePools) == 0 {
		return nil
	}
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(nodePools).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlNodePoolDao) ExistsByOwner(ctx context.Context, ownerID string) (bool, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var count int64
	if err := g2.Model(&api.NodePool{}).Where("owner_id = ?", ownerID).Limit(1).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (d *sqlNodePoolDao) All(ctx context.Context) (api.NodePoolList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	nodePools := api.NodePoolList{}
	if err := g2.Find(&nodePools).Error; err != nil {
		return nil, err
	}
	return nodePools, nil
}
