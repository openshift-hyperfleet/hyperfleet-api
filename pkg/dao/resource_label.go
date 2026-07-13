package dao

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
)

//go:generate mockgen-v0.6.0 -source=resource_label.go -package=dao -destination=resource_label_mock.go

type ResourceLabelDao interface {
	ReplaceLabels(ctx context.Context, resourceID string, labels []api.ResourceLabel) error
}

var _ ResourceLabelDao = &sqlResourceLabelDao{}

type sqlResourceLabelDao struct {
	sessionFactory db.SessionFactory
}

func NewResourceLabelDao(sessionFactory db.SessionFactory) ResourceLabelDao {
	return &sqlResourceLabelDao{sessionFactory: sessionFactory}
}

func (d *sqlResourceLabelDao) ReplaceLabels(
	ctx context.Context, resourceID string, labels []api.ResourceLabel,
) error {
	g2 := d.sessionFactory.New(ctx)

	if err := g2.Where("resource_id = ?", resourceID).Delete(&api.ResourceLabel{}).Error; err != nil {
		db.MarkForRollback(ctx, err)
		return err
	}

	if len(labels) > 0 {
		rows := make([]api.ResourceLabel, len(labels))
		copy(rows, labels)
		for i := range rows {
			rows[i].ResourceID = resourceID
		}
		if err := g2.Create(&rows).Error; err != nil {
			db.MarkForRollback(ctx, err)
			return err
		}
	}

	return nil
}
