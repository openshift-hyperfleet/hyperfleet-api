package factories

import (
	"context"
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet/cmd/hyperfleet/environments"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet/plugins/nodePools"
)

func (f *Factories) NewNodePool(id string) (*api.NodePool, error) {
	nodePoolService := nodePools.Service(&environments.Environment().Services)

	if nodePoolService == nil {
		return nil, fmt.Errorf("nodePoolService is nil - service not initialized")
	}

	// Create a parent cluster first to get a valid OwnerID
	cluster, err := f.NewCluster(f.NewID())
	if err != nil {
		return nil, fmt.Errorf("failed to create parent cluster: %w", err)
	}

	if cluster == nil {
		return nil, fmt.Errorf("cluster is nil after NewCluster call")
	}

	nodePool := &api.NodePool{
		Meta:    api.Meta{ID: id},
		Name:    "test-nodepool-" + id, // Use unique name based on ID
		Spec:    []byte(`{"test": "spec"}`),
		OwnerID: cluster.ID, // Use real cluster ID
	}

	sub, serviceErr := nodePoolService.Create(context.Background(), nodePool)
	// Check for real errors (not typed nil)
	if serviceErr != nil && serviceErr.Code != 0 {
		return nil, fmt.Errorf("failed to create nodepool: %s (code: %d)", serviceErr.Reason, serviceErr.Code)
	}

	if sub == nil {
		return nil, fmt.Errorf("nodePoolService.Create returned nil without error")
	}

	return sub, nil
}

func (f *Factories) NewNodePoolList(name string, count int) ([]*api.NodePool, error) {
	var NodePools []*api.NodePool
	for i := 1; i <= count; i++ {
		c, err := f.NewNodePool(f.NewID())
		if err != nil {
			return nil, err
		}
		NodePools = append(NodePools, c)
	}
	return NodePools, nil
}

// Aliases for test compatibility
func (f *Factories) NewNodePools(id string) (*api.NodePool, error) {
	return f.NewNodePool(id)
}

func (f *Factories) NewNodePoolsList(name string, count int) ([]*api.NodePool, error) {
	return f.NewNodePoolList(name, count)
}
