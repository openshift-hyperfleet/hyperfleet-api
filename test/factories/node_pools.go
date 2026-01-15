package factories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/nodePools"
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
		Meta:      api.Meta{ID: id},
		Name:      "test-nodepool-" + id, // Use unique name based on ID
		Spec:      []byte(`{"test": "spec"}`),
		OwnerID:   cluster.ID, // Use real cluster ID
		CreatedBy: "test@example.com",
		UpdatedBy: "test@example.com",
	}

	sub, serviceErr := nodePoolService.Create(context.Background(), nodePool)
	// Check for real errors (not typed nil)
	if serviceErr != nil && serviceErr.RFC9457Code != "" {
		return nil, fmt.Errorf("failed to create nodepool: %s (code: %s)", serviceErr.Reason, serviceErr.RFC9457Code)
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

// reloadNodePool reloads a node pool from the database to ensure all fields are current
func reloadNodePool(dbSession *gorm.DB, nodePool *api.NodePool) error {
	return dbSession.First(nodePool, "id = ?", nodePool.ID).Error
}

// NewNodePoolWithStatus creates a node pool with specific status phase and last_updated_time
// dbFactory parameter is needed to update database fields
func NewNodePoolWithStatus(f *Factories, dbFactory db.SessionFactory, id string, phase string, lastUpdatedTime *time.Time) (*api.NodePool, error) {
	nodePool, err := f.NewNodePool(id)
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
	err = dbSession.Model(nodePool).Updates(updates).Error
	if err != nil {
		return nil, err
	}

	// Reload to get updated values
	if err := reloadNodePool(dbSession, nodePool); err != nil {
		return nil, err
	}
	return nodePool, nil
}

// NewNodePoolWithLabels creates a node pool with specific labels
func NewNodePoolWithLabels(f *Factories, dbFactory db.SessionFactory, id string, labels map[string]string) (*api.NodePool, error) {
	nodePool, err := f.NewNodePool(id)
	if err != nil {
		return nil, err
	}

	// Convert labels to JSON and update
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, err
	}

	dbSession := dbFactory.New(context.Background())
	err = dbSession.Model(nodePool).Update("labels", labelsJSON).Error
	if err != nil {
		return nil, err
	}

	// Reload to get updated values
	if err := reloadNodePool(dbSession, nodePool); err != nil {
		return nil, err
	}
	return nodePool, nil
}

// NewNodePoolWithStatusAndLabels creates a node pool with both status and labels
func NewNodePoolWithStatusAndLabels(f *Factories, dbFactory db.SessionFactory, id string, phase string, lastUpdatedTime *time.Time, labels map[string]string) (*api.NodePool, error) {
	nodePool, err := NewNodePoolWithStatus(f, dbFactory, id, phase, lastUpdatedTime)
	if err != nil {
		return nil, err
	}

	if labels != nil {
		labelsJSON, err := json.Marshal(labels)
		if err != nil {
			return nil, err
		}

		dbSession := dbFactory.New(context.Background())
		err = dbSession.Model(nodePool).Update("labels", labelsJSON).Error
		if err != nil {
			return nil, err
		}

		if err := reloadNodePool(dbSession, nodePool); err != nil {
			return nil, err
		}
	}

	return nodePool, nil
}
