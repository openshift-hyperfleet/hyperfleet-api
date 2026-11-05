package orchestrator

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
)

// NewDAOClusterLister returns a ClusterLister backed by the generic DAO.
func NewDAOClusterLister(generic dao.GenericDao) ClusterLister {
	return &daoClusterLister{generic: generic}
}

type daoClusterLister struct {
	generic dao.GenericDao
}

func (l *daoClusterLister) ListClusters(ctx context.Context) (api.ResourceList, error) {
	d := l.generic.GetInstanceDao(ctx, &api.Resource{})
	d.Preload("Conditions")
	d.Where(dao.NewWhere("kind = ?", []any{api.ResourceTypeCluster}))
	d.OrderBy("id")

	var clusters api.ResourceList
	if err := d.Fetch(0, -1, &clusters); err != nil {
		return nil, err
	}
	return clusters, nil
}
