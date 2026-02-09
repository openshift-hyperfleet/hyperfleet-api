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
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/resources"
)

// NewResource creates a generic resource of the specified kind.
func (f *Factories) NewResource(id, kind string) (*api.Resource, error) {
	resourceService := resources.Service(&environments.Environment().Services)

	if resourceService == nil {
		return nil, fmt.Errorf("resourceService is nil - service not initialized")
	}

	resource := &api.Resource{
		Meta:      api.Meta{ID: id},
		Kind:      kind,
		Name:      fmt.Sprintf("test-%s-%s", kind, id),
		Spec:      []byte(`{"test": "spec"}`),
		Labels:    []byte(`{}`),
		CreatedBy: "test@example.com",
		UpdatedBy: "test@example.com",
	}

	// Use empty required adapters for test resources
	created, err := resourceService.Create(context.Background(), resource, []string{})
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %s", err.Reason)
	}

	return created, nil
}

// NewClusterResource creates a Cluster resource using the generic resources table.
func (f *Factories) NewClusterResource(id string) (*api.Resource, error) {
	return f.NewResource(id, "Cluster")
}

// NewNodePoolResource creates a NodePool resource owned by a Cluster.
func (f *Factories) NewNodePoolResource(id string, clusterID string) (*api.Resource, error) {
	resourceService := resources.Service(&environments.Environment().Services)

	if resourceService == nil {
		return nil, fmt.Errorf("resourceService is nil - service not initialized")
	}

	ownerKind := "Cluster"
	ownerHref := fmt.Sprintf("/api/hyperfleet/v1/clusters/%s", clusterID)

	resource := &api.Resource{
		Meta:      api.Meta{ID: id},
		Kind:      "NodePool",
		Name:      fmt.Sprintf("test-nodepool-%s", id),
		Spec:      []byte(`{"test": "spec"}`),
		Labels:    []byte(`{}`),
		OwnerID:   &clusterID,
		OwnerKind: &ownerKind,
		OwnerHref: &ownerHref,
		CreatedBy: "test@example.com",
		UpdatedBy: "test@example.com",
	}

	// Use empty required adapters for test resources
	created, err := resourceService.Create(context.Background(), resource, []string{})
	if err != nil {
		return nil, fmt.Errorf("failed to create nodepool resource: %s", err.Reason)
	}

	return created, nil
}

// NewClusters creates a Cluster resource (alias for backwards compatibility with adapter_status_test).
func (f *Factories) NewClusters(id string) (*api.Resource, error) {
	return f.NewClusterResource(id)
}

// NewNodePools creates a NodePool resource with its parent Cluster.
func (f *Factories) NewNodePools(id string) (*api.Resource, error) {
	// Create a parent cluster first
	cluster, err := f.NewClusterResource(f.NewID())
	if err != nil {
		return nil, fmt.Errorf("failed to create parent cluster: %w", err)
	}

	// Create the nodepool owned by the cluster
	nodePool, err := f.NewNodePoolResource(id, cluster.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to create nodepool: %w", err)
	}

	// Set OwnerID for test compatibility (some tests check this)
	nodePool.OwnerID = &cluster.ID

	return nodePool, nil
}

// reloadResource reloads a resource from the database to ensure all fields are current.
func reloadResource(dbSession *gorm.DB, resource *api.Resource) error {
	return dbSession.First(resource, "id = ?", resource.ID).Error
}

// NewResourceWithStatus creates a resource with specific status conditions.
func NewResourceWithStatus(
	f *Factories, dbFactory db.SessionFactory, id, kind string, isAvailable, isReady bool,
) (*api.Resource, error) {
	resource, err := f.NewResource(id, kind)
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
			ObservedGeneration: resource.Generation,
			LastTransitionTime: now,
			CreatedTime:        now,
			LastUpdatedTime:    now,
		},
		{
			Type:               "Ready",
			Status:             readyStatus,
			ObservedGeneration: resource.Generation,
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
	err = dbSession.Model(resource).Update("status_conditions", conditionsJSON).Error
	if err != nil {
		return nil, err
	}

	// Reload to get updated values
	if err := reloadResource(dbSession, resource); err != nil {
		return nil, err
	}
	return resource, nil
}

// NewResourceWithLabels creates a resource with specific labels.
func NewResourceWithLabels(
	f *Factories, dbFactory db.SessionFactory, id, kind string, labels map[string]string,
) (*api.Resource, error) {
	resource, err := f.NewResource(id, kind)
	if err != nil {
		return nil, err
	}

	// Convert labels to JSON and update
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return nil, err
	}

	dbSession := dbFactory.New(context.Background())
	err = dbSession.Model(resource).Update("labels", labelsJSON).Error
	if err != nil {
		return nil, err
	}

	// Reload to get updated values
	if err := reloadResource(dbSession, resource); err != nil {
		return nil, err
	}
	return resource, nil
}

// NewResourceWithStatusAndLabels creates a resource with both status conditions and labels.
func NewResourceWithStatusAndLabels(
	f *Factories, dbFactory db.SessionFactory, id, kind string, isAvailable, isReady bool, labels map[string]string,
) (*api.Resource, error) {
	resource, err := NewResourceWithStatus(f, dbFactory, id, kind, isAvailable, isReady)
	if err != nil {
		return nil, err
	}

	if labels != nil {
		labelsJSON, err := json.Marshal(labels)
		if err != nil {
			return nil, err
		}

		dbSession := dbFactory.New(context.Background())
		err = dbSession.Model(resource).Update("labels", labelsJSON).Error
		if err != nil {
			return nil, err
		}

		if err := reloadResource(dbSession, resource); err != nil {
			return nil, err
		}
	}

	return resource, nil
}
