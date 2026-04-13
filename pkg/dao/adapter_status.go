package dao

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
)

type AdapterStatusDao interface {
	Get(ctx context.Context, id string) (*api.AdapterStatus, error)
	Create(ctx context.Context, adapterStatus *api.AdapterStatus) (*api.AdapterStatus, error)
	Replace(ctx context.Context, adapterStatus *api.AdapterStatus) (*api.AdapterStatus, error)
	Upsert(ctx context.Context, adapterStatus *api.AdapterStatus) (*api.AdapterStatus, error)
	RequestDeletionByResource(ctx context.Context, resourceType, resourceID string, t time.Time) error
	Delete(ctx context.Context, id string) error
	FindByResource(ctx context.Context, resourceType, resourceID string) (api.AdapterStatusList, error)
	FindByResourcePaginated(
		ctx context.Context, resourceType, resourceID string, offset, limit int,
	) (api.AdapterStatusList, int64, error)
	FindByResourceAndAdapter(
		ctx context.Context, resourceType, resourceID, adapter string,
	) (*api.AdapterStatus, error)
	All(ctx context.Context) (api.AdapterStatusList, error)
}

var _ AdapterStatusDao = &sqlAdapterStatusDao{}

type sqlAdapterStatusDao struct {
	sessionFactory *db.SessionFactory
}

func NewAdapterStatusDao(sessionFactory *db.SessionFactory) AdapterStatusDao {
	return &sqlAdapterStatusDao{sessionFactory: sessionFactory}
}

func (d *sqlAdapterStatusDao) Get(ctx context.Context, id string) (*api.AdapterStatus, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var adapterStatus api.AdapterStatus
	if err := g2.Take(&adapterStatus, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &adapterStatus, nil
}

func (d *sqlAdapterStatusDao) Create(
	ctx context.Context, adapterStatus *api.AdapterStatus,
) (*api.AdapterStatus, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Create(adapterStatus).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return adapterStatus, nil
}

func (d *sqlAdapterStatusDao) Replace(
	ctx context.Context, adapterStatus *api.AdapterStatus,
) (*api.AdapterStatus, error) {
	g2 := (*d.sessionFactory).New(ctx)
	if err := g2.Omit(clause.Associations).Save(adapterStatus).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return nil, err
	}
	return adapterStatus, nil
}

// Upsert creates or updates an adapter status based on resource_type, resource_id, and adapter
// This implements the upsert semantic required by the new API spec
func (d *sqlAdapterStatusDao) Upsert(
	ctx context.Context, adapterStatus *api.AdapterStatus,
) (*api.AdapterStatus, error) {
	g2 := (*d.sessionFactory).New(ctx)

	// Keep deterministic observed time from the incoming report when provided (observed_time).
	if adapterStatus.LastReportTime.IsZero() {
		adapterStatus.LastReportTime = time.Now()
	}

	existing, err := d.FindByResourceAndAdapter(
		ctx, adapterStatus.ResourceType, adapterStatus.ResourceID, adapterStatus.Adapter,
	)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		db.MarkForRollback(ctx, err)
		return nil, err
	}

	if err == nil && existing != nil {
		// Preserve LastTransitionTime for conditions whose status hasn't changed.
		adapterStatus.Conditions = preserveLastTransitionTime(existing.Conditions, adapterStatus.Conditions)

		updateResult := g2.Model(&api.AdapterStatus{}).
			Where("resource_type = ? AND resource_id = ? AND adapter = ?",
				adapterStatus.ResourceType, adapterStatus.ResourceID, adapterStatus.Adapter).
			Where(
				"observed_generation < ? OR (observed_generation = ? AND last_report_time < ?)",
				adapterStatus.ObservedGeneration,
				adapterStatus.ObservedGeneration,
				adapterStatus.LastReportTime,
			).
			Updates(map[string]interface{}{
				"conditions":          adapterStatus.Conditions,
				"data":                adapterStatus.Data,
				"metadata":            adapterStatus.Metadata,
				"observed_generation": adapterStatus.ObservedGeneration,
				"last_report_time":    adapterStatus.LastReportTime,
			})
		if updateResult.Error != nil {
			db.MarkForRollback(ctx, updateResult.Error)
			return nil, updateResult.Error
		}

		// No-op when the stored row is fresher or equal.
		if updateResult.RowsAffected == 0 {
			return d.FindByResourceAndAdapter(ctx, adapterStatus.ResourceType, adapterStatus.ResourceID, adapterStatus.Adapter)
		}

		return d.FindByResourceAndAdapter(ctx, adapterStatus.ResourceType, adapterStatus.ResourceID, adapterStatus.Adapter)
	}

	createResult := g2.Omit(clause.Associations).Clauses(clause.OnConflict{DoNothing: true}).Create(adapterStatus)
	if createResult.Error != nil {
		db.MarkForRollback(ctx, createResult.Error)
		return nil, createResult.Error
	}
	if createResult.RowsAffected > 0 {
		return adapterStatus, nil
	}

	// A row was inserted concurrently; return the latest stored row without overwriting it.
	return d.FindByResourceAndAdapter(ctx, adapterStatus.ResourceType, adapterStatus.ResourceID, adapterStatus.Adapter)
}

// Delete permanently removes the adapter status row from the database (hard delete, phase 2).
//
// NOTE: Because Meta.DeletedAt is *time.Time (not gorm.DeletedAt), GORM does not apply
// its built-in soft-delete behaviour here — this issues a real DELETE FROM adapter_statuses statement.
// Pending deletion (phase 1) is handled by RequestDeletionByResource.
//
// TODO(HYPERFLEET-904): See ClusterDao.Delete for the broader discussion on whether to stay
// with the explicit UPDATE approach or adopt gorm.DeletedAt for automatic soft-delete filtering.
func (d *sqlAdapterStatusDao) Delete(ctx context.Context, id string) error {
	g2 := (*d.sessionFactory).New(ctx)
	adapterStatus := &api.AdapterStatus{Meta: api.Meta{ID: id}}
	if err := g2.Omit(clause.Associations).Delete(adapterStatus).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}

func (d *sqlAdapterStatusDao) FindByResource(
	ctx context.Context, resourceType, resourceID string,
) (api.AdapterStatusList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	statuses := api.AdapterStatusList{}
	query := g2.Where("resource_type = ? AND resource_id = ?", resourceType, resourceID)
	if err := query.Find(&statuses).Error; err != nil {
		return nil, err
	}
	return statuses, nil
}

func (d *sqlAdapterStatusDao) FindByResourcePaginated(
	ctx context.Context, resourceType, resourceID string, offset, limit int,
) (api.AdapterStatusList, int64, error) {
	g2 := (*d.sessionFactory).New(ctx)
	statuses := api.AdapterStatusList{}
	var total int64

	// Base query
	query := g2.Where("resource_type = ? AND resource_id = ?", resourceType, resourceID)

	// Get total count for pagination metadata
	if err := query.Model(&api.AdapterStatus{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination using OFFSET and LIMIT
	if err := query.Offset(offset).Limit(limit).Find(&statuses).Error; err != nil {
		return nil, 0, err
	}

	return statuses, total, nil
}

func (d *sqlAdapterStatusDao) FindByResourceAndAdapter(
	ctx context.Context, resourceType, resourceID, adapter string,
) (*api.AdapterStatus, error) {
	g2 := (*d.sessionFactory).New(ctx)
	var adapterStatus api.AdapterStatus
	query := g2.Where("resource_type = ? AND resource_id = ? AND adapter = ?", resourceType, resourceID, adapter)
	if err := query.Take(&adapterStatus).Error; err != nil {
		return nil, err
	}
	return &adapterStatus, nil
}

func (d *sqlAdapterStatusDao) All(ctx context.Context) (api.AdapterStatusList, error) {
	g2 := (*d.sessionFactory).New(ctx)
	statuses := api.AdapterStatusList{}
	if err := g2.Find(&statuses).Error; err != nil {
		return nil, err
	}
	return statuses, nil
}

func (d *sqlAdapterStatusDao) RequestDeletionByResource(
	ctx context.Context, resourceType, resourceID string, t time.Time,
) error {
	g2 := (*d.sessionFactory).New(ctx)
	result := g2.Model(&api.AdapterStatus{}).
		Where("resource_type = ? AND resource_id = ? AND deleted_at IS NULL", resourceType, resourceID).
		Update("deleted_at", t)
	if result.Error != nil {
		db.MarkForRollback(ctx, result.Error)
		return result.Error
	}
	return nil
}

// preserveLastTransitionTime preserves LastTransitionTime for conditions whose status hasn't changed
// This implements the Kubernetes condition semantic where LastTransitionTime is only updated when status changes
func preserveLastTransitionTime(oldConditionsJSON, newConditionsJSON datatypes.JSON) datatypes.JSON {
	// Unmarshal old conditions
	var oldConditions []openapi.AdapterCondition
	if len(oldConditionsJSON) > 0 {
		if err := json.Unmarshal(oldConditionsJSON, &oldConditions); err != nil {
			// If we can't unmarshal old conditions, return new conditions as-is
			return newConditionsJSON
		}
	}

	// Unmarshal new conditions
	var newConditions []openapi.AdapterCondition
	if len(newConditionsJSON) > 0 {
		if err := json.Unmarshal(newConditionsJSON, &newConditions); err != nil {
			// If we can't unmarshal new conditions, return new conditions as-is
			return newConditionsJSON
		}
	}

	// Build a map of old conditions by type for quick lookup
	oldConditionsMap := make(map[string]openapi.AdapterCondition)
	for _, oldCond := range oldConditions {
		oldConditionsMap[oldCond.Type] = oldCond
	}

	// Update new conditions: preserve LastTransitionTime if status hasn't changed
	for i := range newConditions {
		if oldCond, exists := oldConditionsMap[newConditions[i].Type]; exists {
			// If status hasn't changed, preserve the old LastTransitionTime
			if oldCond.Status == newConditions[i].Status {
				newConditions[i].LastTransitionTime = oldCond.LastTransitionTime
			}
			// If status changed, keep the new LastTransitionTime (already set to now)
		}
		// If this is a new condition type, keep the new LastTransitionTime
	}

	// Marshal back to JSON
	updatedJSON, err := json.Marshal(newConditions)
	if err != nil {
		// If we can't marshal, return new conditions as-is
		return newConditionsJSON
	}

	return updatedJSON
}
