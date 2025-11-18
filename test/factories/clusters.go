package factories

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/clusters"
)

func (f *Factories) NewCluster(id string) (*api.Cluster, error) {
	clusterService := clusters.Service(&environments.Environment().Services)

	cluster := &api.Cluster{
		Meta:       api.Meta{ID: id},
		Name:       "test-cluster-" + id, // Use unique name based on ID
		Spec:       []byte(`{"test": "spec"}`),
		Generation: 42,
	}

	sub, err := clusterService.Create(context.Background(), cluster)
	if err != nil {
		return nil, err
	}

	return sub, nil
}

func (f *Factories) NewClusterList(name string, count int) ([]*api.Cluster, error) {
	var Clusters []*api.Cluster
	for i := 1; i <= count; i++ {
		c, err := f.NewCluster(f.NewID())
		if err != nil {
			return nil, err
		}
		Clusters = append(Clusters, c)
	}
	return Clusters, nil
}

// Aliases for test compatibility
func (f *Factories) NewClusters(id string) (*api.Cluster, error) {
	return f.NewCluster(id)
}

func (f *Factories) NewClustersList(name string, count int) ([]*api.Cluster, error) {
	return f.NewClusterList(name, count)
}
