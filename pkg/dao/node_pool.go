package dao

import (
	"bytes"
	"context"

	"gorm.io/gorm/clause"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
)

type NodePoolDao interface {
	Get(ctx context.Context, id string) (*api.NodePool, error)
	Create(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, error)
	Replace(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (api.NodePoolList, error)
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

	// Compare spec: if changed, increment generation
	if !bytes.Equal(existing.Spec, nodePool.Spec) {
		nodePool.Generation = existing.Generation + 1
	} else {
		// Spec unchanged, preserve generation
		nodePool.Generation = existing.Generation
	}

	// Save the nodePool
	if err := g2.Omit(clause.Associations).Save(nodePool).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return nodePool, nil
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

func (d *sqlNodePoolDao) All(ctx context.Context) (api.NodePoolList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	nodePools := api.NodePoolList{}
	if err := g2.Find(&nodePools).Error; err != nil {
		return nil, err
	}
	return nodePools, nil
}
