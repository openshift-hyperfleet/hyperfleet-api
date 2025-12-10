package factories

import (
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
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

// reloadCluster reloads a cluster from the database to ensure all fields are current
func reloadCluster(dbSession *gorm.DB, cluster *api.Cluster) error {
	return dbSession.First(cluster, "id = ?", cluster.ID).Error
}

// NewClusterWithStatus creates a cluster with specific status phase and last_updated_time
// dbFactory parameter is needed to update database fields
func NewClusterWithStatus(f *Factories, dbFactory db.SessionFactory, id string, phase string, lastUpdatedTime *time.Time) (*api.Cluster, error) {
	cluster, err := f.NewCluster(id)
	if err != nil {
		return nil, err
	}

	// Update database record with status fields
	dbSession := dbFactory.New(context.Background())
	updates := map[string]interface{}{
		"status_phase": phase,
	}
	if lastUpdatedTime != nil {
		updates["status_last_updated_time"] = lastUpdatedTime
	}
	err = dbSession.Model(cluster).Updates(updates).Error
	if err != nil {
		return nil, err
	}

	// Reload to get updated values
	if err := reloadCluster(dbSession, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}

// NewClusterWithLabels creates a cluster with specific labels
func NewClusterWithLabels(f *Factories, dbFactory db.SessionFactory, id string, labels map[string]string) (*api.Cluster, error) {
	cluster, err := f.NewCluster(id)
	if err != nil {
		return nil, err
	}

	// Convert labels to JSON and update
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, err
	}

	dbSession := dbFactory.New(context.Background())
	err = dbSession.Model(cluster).Update("labels", labelsJSON).Error
	if err != nil {
		return nil, err
	}

	// Reload to get updated values
	if err := reloadCluster(dbSession, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}

// NewClusterWithStatusAndLabels creates a cluster with both status and labels
func NewClusterWithStatusAndLabels(f *Factories, dbFactory db.SessionFactory, id string, phase string, lastUpdatedTime *time.Time, labels map[string]string) (*api.Cluster, error) {
	cluster, err := NewClusterWithStatus(f, dbFactory, id, phase, lastUpdatedTime)
	if err != nil {
		return nil, err
	}

	if labels != nil {
		labelsJSON, err := json.Marshal(labels)
		if err != nil {
			return nil, err
		}

		dbSession := dbFactory.New(context.Background())
		err = dbSession.Model(cluster).Update("labels", labelsJSON).Error
		if err != nil {
			return nil, err
		}

		if err := reloadCluster(dbSession, cluster); err != nil {
			return nil, err
		}
	}

	return cluster, nil
}
