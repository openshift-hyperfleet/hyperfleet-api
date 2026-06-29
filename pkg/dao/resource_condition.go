package dao

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
)

type ResourceConditionDao interface {
	// UpdateConditions must be called within a write transaction — it performs
	// delete+insert which is not atomic without one. Per ADR-0008, this is called
	// from the adapter status upsert path (PUT), which has a transaction from
	// TransactionMiddleware.
	UpdateConditions(ctx context.Context, resourceID string, conditions []api.ResourceCondition) error
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
		return err
	}

	prevByType := make(map[string]api.ResourceCondition, len(existing))
	for _, c := range existing {
		prevByType[c.Type] = c
	}

	for i := range conditions {
		conditions[i].ResourceID = resourceID
		if prev, ok := prevByType[conditions[i].Type]; ok {
			if prev.Status == conditions[i].Status {
				conditions[i].LastTransitionTime = prev.LastTransitionTime
			}
			conditions[i].CreatedTime = prev.CreatedTime
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
