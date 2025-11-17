package mocks

import (
	"context"

	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

var _ dao.NodePoolDao = &nodePoolDaoMock{}

type nodePoolDaoMock struct {
	nodePools api.NodePoolList
}

func NewNodePoolDao() *nodePoolDaoMock {
	return &nodePoolDaoMock{}
}

func (d *nodePoolDaoMock) Get(ctx context.Context, id string) (*api.NodePool, error) {
	for _, nodePool := range d.nodePools {
		if nodePool.ID == id {
			return nodePool, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *nodePoolDaoMock) Create(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, error) {
	d.nodePools = append(d.nodePools, nodePool)
	return nodePool, nil
}

func (d *nodePoolDaoMock) Replace(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, error) {
	return nil, errors.NotImplemented("NodePool").AsError()
}

func (d *nodePoolDaoMock) Delete(ctx context.Context, id string) error {
	return errors.NotImplemented("NodePool").AsError()
}

func (d *nodePoolDaoMock) FindByIDs(ctx context.Context, ids []string) (api.NodePoolList, error) {
	return nil, errors.NotImplemented("NodePool").AsError()
}

func (d *nodePoolDaoMock) All(ctx context.Context) (api.NodePoolList, error) {
	return d.nodePools, nil
}
