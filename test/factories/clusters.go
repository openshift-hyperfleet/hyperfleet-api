package factories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/resources"
)

// resourceService retrieves the ResourceService for creating resources.
func resourceService() services.ResourceService {
	return resources.Service(&environments.Environment().Services)
}

// reloadResource reloads a resource from the database to ensure all fields are current.
func reloadResource(dbSession *gorm.DB, resource *api.Resource) error {
	return dbSession.Preload("Conditions").Preload("Labels").First(resource, "id = ?", resource.ID).Error
}

// mapToLabels converts a string map to ResourceLabel slice for test factories.
func mapToLabels(m map[string]string) []api.ResourceLabel {
	labels := make([]api.ResourceLabel, 0, len(m))
	for k, v := range m {
		labels = append(labels, api.ResourceLabel{Key: k, Value: v})
	}
	return labels
}

func buildConditionsWithGeneration(
	resourceID string, observedGeneration int32, isAvailable, isReconciled bool, t time.Time,
) []api.ResourceCondition {
	availableStatus := api.ConditionFalse
	if isAvailable {
		availableStatus = api.ConditionTrue
	}
	reconciledStatus := api.ConditionFalse
	if isReconciled {
		reconciledStatus = api.ConditionTrue
	}

	return []api.ResourceCondition{
		{
			ResourceID:         resourceID,
			Type:               api.ResourceConditionTypeAvailable,
			Status:             availableStatus,
			ObservedGeneration: observedGeneration,
			LastTransitionTime: t,
			CreatedTime:        t,
			LastUpdatedTime:    t,
		},
		{
			ResourceID:         resourceID,
			Type:               api.ResourceConditionTypeReconciled,
			Status:             reconciledStatus,
			ObservedGeneration: observedGeneration,
			LastTransitionTime: t,
			CreatedTime:        t,
			LastUpdatedTime:    t,
		},
	}
}

func (f *Factories) NewCluster(id string) (*api.Resource, error) {
	svc := resourceService()
	ctx := context.Background()

	cluster := &api.Resource{
		Kind:      "Cluster",
		Name:      id,
		Spec:      []byte(`{"region": "us-central1", "provider": "gcp"}`),
		CreatedBy: "test@example.com",
		UpdatedBy: "test@example.com",
	}

	result, svcErr := svc.Create(ctx, "Cluster", cluster, nil)
	if svcErr != nil {
		return nil, fmt.Errorf("create cluster: %w", svcErr)
	}
	return result, nil
}

func (f *Factories) NewClusterList(name string, count int) ([]*api.Resource, error) {
	result := make([]*api.Resource, 0, count)
	for i := range count {
		cluster, err := f.NewCluster(fmt.Sprintf("%s-%d", name, i))
		if err != nil {
			return nil, err
		}
		result = append(result, cluster)
	}
	return result, nil
}

// Aliases for test compatibility
func (f *Factories) NewClusters(id string) (*api.Resource, error) {
	return f.NewCluster(id)
}

func (f *Factories) NewClustersList(name string, count int) ([]*api.Resource, error) {
	return f.NewClusterList(name, count)
}

// NewClusterWithStatus creates a cluster with specific status conditions using the current time.
func NewClusterWithStatus(
	f *Factories, dbFactory db.SessionFactory, id string, isAvailable, isReconciled bool,
) (*api.Resource, error) {
	return NewClusterWithStatusAtTime(f, dbFactory, id, isAvailable, isReconciled, time.Now())
}

// NewClusterWithStatusAtTime creates a cluster with specific status conditions and custom timestamps.
func NewClusterWithStatusAtTime(
	f *Factories, dbFactory db.SessionFactory, id string,
	isAvailable, isReconciled bool, conditionTime time.Time,
) (*api.Resource, error) {
	cluster, err := f.NewCluster(id)
	if err != nil {
		return nil, err
	}

	conditions := buildConditionsWithGeneration(cluster.ID, cluster.Generation, isAvailable, isReconciled, conditionTime)

	dbSession := dbFactory.New(context.Background())
	// Delete conditions seeded by Create before inserting test-specific ones.
	if err := dbSession.Where("resource_id = ?", cluster.ID).Delete(&api.ResourceCondition{}).Error; err != nil {
		return nil, err
	}
	if err := dbSession.Create(&conditions).Error; err != nil {
		return nil, err
	}

	if err := reloadResource(dbSession, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}

// NewClusterWithObservedGeneration creates a cluster with a specific observed_generation on its conditions.
func NewClusterWithObservedGeneration(
	f *Factories, dbFactory db.SessionFactory, id string,
	isAvailable, isReconciled bool, observedGeneration int32,
) (*api.Resource, error) {
	cluster, err := f.NewCluster(id)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	conditions := buildConditionsWithGeneration(cluster.ID, observedGeneration, isAvailable, isReconciled, now)

	dbSession := dbFactory.New(context.Background())
	// Delete conditions seeded by Create before inserting test-specific ones.
	if err := dbSession.Where("resource_id = ?", cluster.ID).Delete(&api.ResourceCondition{}).Error; err != nil {
		return nil, err
	}
	if err := dbSession.Create(&conditions).Error; err != nil {
		return nil, err
	}

	if err := reloadResource(dbSession, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}

// NewClusterWithLabels creates a cluster with specific labels.
func NewClusterWithLabels(
	f *Factories, dbFactory db.SessionFactory, id string, labels map[string]string,
) (*api.Resource, error) {
	cluster, err := f.NewCluster(id)
	if err != nil {
		return nil, err
	}

	labelDao := dao.NewResourceLabelDao(dbFactory)
	if err := labelDao.ReplaceLabels(context.Background(), cluster.ID, mapToLabels(labels)); err != nil {
		return nil, err
	}

	dbSession := dbFactory.New(context.Background())
	if err := reloadResource(dbSession, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}

// NewClusterWithSpec creates a cluster with a specific spec JSON value.
func NewClusterWithSpec(
	f *Factories, dbFactory db.SessionFactory, id string, spec map[string]interface{},
) (*api.Resource, error) {
	cluster, err := f.NewCluster(id)
	if err != nil {
		return nil, err
	}

	specJSON, err := json.Marshal(spec)
	if err != nil {
		return nil, err
	}

	dbSession := dbFactory.New(context.Background())
	if err := dbSession.Model(cluster).Update("spec", specJSON).Error; err != nil {
		return nil, err
	}

	if err := reloadResource(dbSession, cluster); err != nil {
		return nil, err
	}
	return cluster, nil
}

// NewClusterWithStatusAndLabels creates a cluster with both status conditions and labels.
func NewClusterWithStatusAndLabels(
	f *Factories, dbFactory db.SessionFactory, id string, isAvailable, isReconciled bool, labels map[string]string,
) (*api.Resource, error) {
	cluster, err := NewClusterWithStatus(f, dbFactory, id, isAvailable, isReconciled)
	if err != nil {
		return nil, err
	}

	if labels != nil {
		labelDao := dao.NewResourceLabelDao(dbFactory)
		if err := labelDao.ReplaceLabels(context.Background(), cluster.ID, mapToLabels(labels)); err != nil {
			return nil, err
		}

		dbSession := dbFactory.New(context.Background())
		if err := reloadResource(dbSession, cluster); err != nil {
			return nil, err
		}
	}

	return cluster, nil
}
