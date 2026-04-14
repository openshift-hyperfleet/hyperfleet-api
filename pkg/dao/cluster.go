package dao

import (
	"bytes"
	"context"
	"time"

	"gorm.io/gorm/clause"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
)

type ClusterDao interface {
	Get(ctx context.Context, id string) (*api.Cluster, error)
	Create(ctx context.Context, cluster *api.Cluster) (*api.Cluster, error)
	Replace(ctx context.Context, cluster *api.Cluster) (*api.Cluster, error)
	RequestDeletion(ctx context.Context, id string) (*api.Cluster, bool, error)
	Delete(ctx context.Context, id string) error
	FindByIDs(ctx context.Context, ids []string) (api.ClusterList, error)
	All(ctx context.Context) (api.ClusterList, error)
}

var _ ClusterDao = &sqlClusterDao{}

type sqlClusterDao struct {
	sessionFactory *db.SessionFactory
}

func NewClusterDao(sessionFactory *db.SessionFactory) ClusterDao {
	return &sqlClusterDao{sessionFactory: sessionFactory}
}

func (d *sqlClusterDao) Get(ctx context.Context, id string) (*api.Cluster, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var cluster api.Cluster
	if err := g2.Take(&cluster, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &cluster, nil
}

func (d *sqlClusterDao) Create(ctx context.Context, cluster *api.Cluster) (*api.Cluster, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(cluster).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return cluster, nil
}

func (d *sqlClusterDao) Replace(ctx context.Context, cluster *api.Cluster) (*api.Cluster, error) {
	g2 := (*d.sessionFactory).New(ctx)

	// Get the existing cluster to compare spec
	existing, err := d.Get(ctx, cluster.ID)
	if err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}

	// Compare spec: if changed, increment generation. Aggregated conditions are recomputed in the service layer.
	if !bytes.Equal(existing.Spec, cluster.Spec) {
		cluster.Generation = existing.Generation + 1
	} else {
		// Spec unchanged, preserve generation
		cluster.Generation = existing.Generation
	}

	// Save the cluster
	if err := g2.Omit(clause.Associations).Save(cluster).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return cluster, nil
}

func (d *sqlClusterDao) RequestDeletion(ctx context.Context, id string) (*api.Cluster, bool, error) {
	g2 := (*d.sessionFactory).New(ctx)

	cluster, err := d.Get(ctx, id)
	if err != nil {
		db.MarkForRollback(ctx, err)
		return nil, false, err
	}

	// Already marked for deletion — return as-is (idempotent, no DB write).
	if cluster.DeletedTime != nil {
		return cluster, false, nil
	}

	// Set deleted_time, deleted_by, and increment generation to trigger Sentinel reconciliation.
	t := time.Now()
	deletedBy := "system@hyperfleet.local"
	cluster.DeletedTime = &t
	cluster.DeletedBy = &deletedBy
	cluster.Generation++
	if err := g2.Omit(clause.Associations).Save(cluster).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, false, err
	}
	return cluster, true, nil
}

func (d *sqlClusterDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Delete(&api.Cluster{Meta: api.Meta{ID: id}}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlClusterDao) FindByIDs(ctx context.Context, ids []string) (api.ClusterList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	clusters := api.ClusterList{}
	if err := g2.Where("id in (?)", ids).Find(&clusters).Error; err != nil {
		return nil, err
	}
	return clusters, nil
}

func (d *sqlClusterDao) All(ctx context.Context) (api.ClusterList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	clusters := api.ClusterList{}
	if err := g2.Find(&clusters).Error; err != nil {
		return nil, err
	}
	return clusters, nil
}
