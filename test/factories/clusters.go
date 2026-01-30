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
		CreatedBy:  "test@example.com",
		UpdatedBy:  "test@example.com",
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

// NewClusterWithStatus creates a cluster with specific status conditions
// dbFactory parameter is needed to update database fields
// The isAvailable and isReady parameters control which synthetic conditions are set
func NewClusterWithStatus(
	f *Factories, dbFactory db.SessionFactory, id string, isAvailable, isReady bool,
) (*api.Cluster, error) {
	cluster, err := f.NewCluster(id)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	availableStatus := api.ConditionFalse
	if isAvailable {
		availableStatus = api.ConditionTrue
	}
	readyStatus := api.ConditionFalse
	if isReady {
		readyStatus = api.ConditionTrue
	}

	conditions := []api.ResourceCondition{
		{
			Type:               "Available",
			Status:             availableStatus,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			CreatedTime:        now,
			LastUpdatedTime:    now,
		},
		{
			Type:               "Ready",
			Status:             readyStatus,
			ObservedGeneration: cluster.Generation,
			LastTransitionTime: now,
			CreatedTime:        now,
			LastUpdatedTime:    now,
		},
	}

	conditionsJSON, err := json.Marshal(conditions)
	if err != nil {
		return nil, err
	}

	// Update database record with status conditions
	dbSession := dbFactory.New(context.Background())
	err = dbSession.Model(cluster).Update("status_conditions", conditionsJSON).Error
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
func NewClusterWithLabels(
	f *Factories, dbFactory db.SessionFactory, id string, labels map[string]string,
) (*api.Cluster, error) {
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

// NewClusterWithStatusAndLabels creates a cluster with both status conditions and labels
func NewClusterWithStatusAndLabels(
	f *Factories, dbFactory db.SessionFactory, id string, isAvailable, isReady bool, labels map[string]string,
) (*api.Cluster, error) {
	cluster, err := NewClusterWithStatus(f, dbFactory, id, isAvailable, isReady)
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
