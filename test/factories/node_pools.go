package factories

import (
	"context"
	"fmt"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
)

func (f *Factories) NewNodePool(id string) (*api.Resource, error) {
	svc := resourceService()
	ctx := context.Background()

	// Create parent cluster
	cluster := &api.Resource{
		Kind:      "Cluster",
		Name:      fmt.Sprintf("parent-%s", id),
		Spec:      []byte(`{"region": "us-central1", "provider": "gcp"}`),
		CreatedBy: "test@example.com",
		UpdatedBy: "test@example.com",
	}
	cluster, svcErr := svc.Create(ctx, "Cluster", cluster, nil)
	if svcErr != nil {
		return nil, fmt.Errorf("create parent cluster: %w", svcErr)
	}

	// Create nodepool
	ownerKind := "Cluster"
	np := &api.Resource{
		Kind:      "NodePool",
		Name:      id,
		OwnerID:   &cluster.ID,
		OwnerKind: &ownerKind,
		Spec:      []byte(`{"machine_type": "n1-standard-4", "replicas": 3}`),
		CreatedBy: "test@example.com",
		UpdatedBy: "test@example.com",
	}
	result, svcErr := svc.Create(ctx, "NodePool", np, nil)
	if svcErr != nil {
		return nil, fmt.Errorf("create nodepool: %w", svcErr)
	}
	return result, nil
}

func (f *Factories) NewNodePoolList(name string, count int) ([]*api.Resource, error) {
	svc := resourceService()
	ctx := context.Background()

	// Create shared parent cluster
	cluster := &api.Resource{
		Kind:      "Cluster",
		Name:      fmt.Sprintf("parent-%s", name),
		Spec:      []byte(`{"region": "us-central1", "provider": "gcp"}`),
		CreatedBy: "test@example.com",
		UpdatedBy: "test@example.com",
	}
	cluster, svcErr := svc.Create(ctx, "Cluster", cluster, nil)
	if svcErr != nil {
		return nil, fmt.Errorf("create parent cluster: %w", svcErr)
	}

	ownerKind := "Cluster"
	result := make([]*api.Resource, 0, count)
	for i := range count {
		np := &api.Resource{
			Kind:      "NodePool",
			Name:      fmt.Sprintf("%s-%d", name, i),
			OwnerID:   &cluster.ID,
			OwnerKind: &ownerKind,
			Spec:      []byte(`{"machine_type": "n1-standard-4", "replicas": 3}`),
			CreatedBy: "test@example.com",
			UpdatedBy: "test@example.com",
		}
		np, svcErr = svc.Create(ctx, "NodePool", np, nil)
		if svcErr != nil {
			return nil, fmt.Errorf("create nodepool: %w", svcErr)
		}
		result = append(result, np)
	}
	return result, nil
}

// Aliases for test compatibility
func (f *Factories) NewNodePools(id string) (*api.Resource, error) {
	return f.NewNodePool(id)
}

func (f *Factories) NewNodePoolsList(name string, count int) ([]*api.Resource, error) {
	return f.NewNodePoolList(name, count)
}

// NewNodePoolWithStatus creates a node pool with specific status conditions using the current time.
func NewNodePoolWithStatus(
	f *Factories, dbFactory db.SessionFactory, id string, isAvailable, isReconciled bool,
) (*api.Resource, error) {
	return NewNodePoolWithStatusAtTime(f, dbFactory, id, isAvailable, isReconciled, time.Now())
}

// NewNodePoolWithStatusAtTime creates a node pool with specific status conditions and custom timestamps.
func NewNodePoolWithStatusAtTime(
	f *Factories, dbFactory db.SessionFactory, id string,
	isAvailable, isReconciled bool, conditionTime time.Time,
) (*api.Resource, error) {
	nodePool, err := f.NewNodePool(id)
	if err != nil {
		return nil, err
	}

	conditions := buildConditionsWithGeneration(nodePool.ID, nodePool.Generation, isAvailable, isReconciled, conditionTime)

	dbSession := dbFactory.New(context.Background())
	// Delete conditions seeded by Create before inserting test-specific ones.
	if err := dbSession.Where("resource_id = ?", nodePool.ID).Delete(&api.ResourceCondition{}).Error; err != nil {
		return nil, err
	}
	if err := dbSession.Create(&conditions).Error; err != nil {
		return nil, err
	}

	if err := reloadResource(dbSession, nodePool); err != nil {
		return nil, err
	}
	return nodePool, nil
}

// NewNodePoolWithLabels creates a node pool with specific labels.
func NewNodePoolWithLabels(
	f *Factories, dbFactory db.SessionFactory, id string, labels map[string]string,
) (*api.Resource, error) {
	nodePool, err := f.NewNodePool(id)
	if err != nil {
		return nil, err
	}

	labelDao := dao.NewResourceLabelDao(dbFactory)
	if err := labelDao.ReplaceLabels(context.Background(), nodePool.ID, mapToLabels(labels)); err != nil {
		return nil, err
	}

	dbSession := dbFactory.New(context.Background())
	if err := reloadResource(dbSession, nodePool); err != nil {
		return nil, err
	}
	return nodePool, nil
}

// NewNodePoolWithStatusAndLabels creates a node pool with both status conditions and labels.
func NewNodePoolWithStatusAndLabels(
	f *Factories, dbFactory db.SessionFactory, id string, isAvailable, isReconciled bool, labels map[string]string,
) (*api.Resource, error) {
	nodePool, err := NewNodePoolWithStatus(f, dbFactory, id, isAvailable, isReconciled)
	if err != nil {
		return nil, err
	}

	if labels != nil {
		labelDao := dao.NewResourceLabelDao(dbFactory)
		if err := labelDao.ReplaceLabels(context.Background(), nodePool.ID, mapToLabels(labels)); err != nil {
			return nil, err
		}

		dbSession := dbFactory.New(context.Background())
		if err := reloadResource(dbSession, nodePool); err != nil {
			return nil, err
		}
	}

	return nodePool, nil
}
