package mocks

import (
	"context"

	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

var _ dao.ClusterDao = &clusterDaoMock{}

type clusterDaoMock struct {
	clusters api.ClusterList
}

func NewClusterDao() *clusterDaoMock {
	return &clusterDaoMock{}
}

func (d *clusterDaoMock) Get(ctx context.Context, id string) (*api.Cluster, error) {
	for _, cluster := range d.clusters {
		if cluster.ID == id {
			return cluster, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *clusterDaoMock) Create(ctx context.Context, cluster *api.Cluster) (*api.Cluster, error) {
	d.clusters = append(d.clusters, cluster)
	return cluster, nil
}

func (d *clusterDaoMock) Replace(ctx context.Context, cluster *api.Cluster) (*api.Cluster, error) {
	return nil, errors.NotImplemented("Cluster").AsError()
}

func (d *clusterDaoMock) Delete(ctx context.Context, id string) error {
	return errors.NotImplemented("Cluster").AsError()
}

func (d *clusterDaoMock) FindByIDs(ctx context.Context, ids []string) (api.ClusterList, error) {
	return nil, errors.NotImplemented("Cluster").AsError()
}

func (d *clusterDaoMock) All(ctx context.Context) (api.ClusterList, error) {
	return d.clusters, nil
}
