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

func (d *nodePoolDaoMock) Get(ctx context.Context, id string) (*api.NodePool, error) {
	for _, nodePool := range d.nodePools {
		if nodePool.ID == id {
			return nodePool, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *nodePoolDaoMock) GetByIDAndOwner(ctx context.Context, id string, ownerID string) (*api.NodePool, error) {
	for _, nodePool := range d.nodePools {
		if nodePool.ID == id && nodePool.OwnerID == ownerID {
			return nodePool, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (d *nodePoolDaoMock) GetForUpdate(ctx context.Context, id string) (*api.NodePool, error) {
	return d.Get(ctx, id)
}

func (d *nodePoolDaoMock) SaveStatusConditions(ctx context.Context, id string, statusConditions []byte) error {
	for _, np := range d.nodePools {
		if np.ID == id {
			np.StatusConditions = statusConditions
			return nil
		}
	}
	return gorm.ErrRecordNotFound
}

func (d *nodePoolDaoMock) Create(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, error) {
	d.nodePools = append(d.nodePools, nodePool)
	return nodePool, nil
}

func (d *nodePoolDaoMock) Save(ctx context.Context, nodePool *api.NodePool) error {
	d.nodePools = append(d.nodePools, nodePool)
	return nil
}

func (d *nodePoolDaoMock) Delete(ctx context.Context, id string) error {
	return errors.NotImplemented("NodePool").AsError()
}

func (d *nodePoolDaoMock) FindByIDs(ctx context.Context, ids []string) (api.NodePoolList, error) {
	return nil, errors.NotImplemented("NodePool").AsError()
}

func (d *nodePoolDaoMock) FindByOwner(ctx context.Context, ownerID string) (api.NodePoolList, error) {
	return nil, errors.NotImplemented("NodePool").AsError()
}

func (d *nodePoolDaoMock) SaveAll(ctx context.Context, nodePools api.NodePoolList) error {
	return errors.NotImplemented("NodePool").AsError()
}

func (d *nodePoolDaoMock) ExistsByOwner(ctx context.Context, ownerID string) (bool, error) {
	for _, np := range d.nodePools {
		if np.OwnerID == ownerID {
			return true, nil
		}
	}
	return false, nil
}

func (d *nodePoolDaoMock) All(ctx context.Context) (api.NodePoolList, error) {
	return d.nodePools, nil
}
