package dao

import (
	"context"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
)

type ResourceConditionDao interface {
	// UpdateConditions atomically replaces all condition rows for a resource.
	//
	// Prerequisites:
	//   - Write transaction (delete+insert is not atomic without one).
	//   - Caller holds a row lock on the parent resource (e.g. via GetForUpdate).
	//     Without it, concurrent callers read stale state and the loser's
	//     timestamp preservation computes against a pre-delete snapshot.
	//
	// Timestamp contract: for existing condition types, CreatedTime is always
	// preserved and LastTransitionTime is preserved when status is unchanged.
	// For new condition types, both are used as-is from the input — caller
	// must populate them. Zero timestamps are defaulted to now.
	UpdateConditions(ctx context.Context, resourceID string, conditions []api.ResourceCondition) error

	// DeleteByResource removes all condition rows for a resource.
	// Used during hard-delete to clean up before removing the resource row.
	DeleteByResource(ctx context.Context, resourceID string) error
}

var _ ResourceConditionDao = &sqlResourceConditionDao{}

type sqlResourceConditionDao struct {
	sessionFactory db.SessionFactory
}

func NewResourceConditionDao(sessionFactory db.SessionFactory) ResourceConditionDao {
	return &sqlResourceConditionDao{sessionFactory: sessionFactory}
}

func (d *sqlResourceConditionDao) UpdateConditions(
	ctx context.Context, resourceID string, conditions []api.ResourceCondition,
) error {
	g2 := d.sessionFactory.New(ctx)

	var existing []api.ResourceCondition
	if err := g2.Where("resource_id = ?", resourceID).Find(&existing).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}

	prevByType := make(map[string]api.ResourceCondition, len(existing))
	for _, c := range existing {
		prevByType[c.Type] = c
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	for i := range conditions {
		conditions[i].ResourceID = resourceID
		if prev, ok := prevByType[conditions[i].Type]; ok {
			if prev.Status == conditions[i].Status {
				conditions[i].LastTransitionTime = prev.LastTransitionTime
			}
			conditions[i].CreatedTime = prev.CreatedTime
		}
		if conditions[i].CreatedTime.IsZero() {
			conditions[i].CreatedTime = now
		}
		if conditions[i].LastTransitionTime.IsZero() {
			conditions[i].LastTransitionTime = now
		}
		if conditions[i].LastUpdatedTime.IsZero() {
			conditions[i].LastUpdatedTime = now
		}
	}

	if err := g2.Where("resource_id = ?", resourceID).Delete(&api.ResourceCondition{}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}

	if len(conditions) > 0 {
		if err := g2.Create(&conditions).Error; err != nil {
			db.MarkForRollback(ctx, err)
			return err
		}
	}

	return nil
}

func (d *sqlResourceConditionDao) DeleteByResource(ctx context.Context, resourceID string) error {
	g2 := d.sessionFactory.New(ctx)
	if err := g2.Where("resource_id = ?", resourceID).Delete(&api.ResourceCondition{}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}
	return nil
}
