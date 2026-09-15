package dao

import (
	"context"

	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
)

type Where struct {
	sql    string
	values []any
}

func NewWhere(sql string, values []any) Where {
	return Where{
		sql:    sql,
		values: values,
	}
}

type GenericDao interface {
	Fetch(offset int, limit int, resourceList interface{}) error

	GetInstanceDao(ctx context.Context, model interface{}) GenericDao
	Preload(preload string)
	OrderBy(orderBy string)
	Where(where Where)
	Count(model interface{}, total *int64) error
	Validate(resourceList interface{}) error

	GetTableName() string
}

var _ GenericDao = &sqlGenericDao{}

type sqlGenericDao struct {
	sessionFactory db.SessionFactory
	g2             *gorm.DB
}

func NewGenericDao(sessionFactory db.SessionFactory) GenericDao {
	return &sqlGenericDao{sessionFactory: sessionFactory}
}

func (d *sqlGenericDao) GetInstanceDao(ctx context.Context, model interface{}) GenericDao {
	return &sqlGenericDao{
		sessionFactory: d.sessionFactory,
		g2:             d.sessionFactory.New(ctx).Model(model),
	}
}

func (d *sqlGenericDao) Fetch(offset int, limit int, resourceList interface{}) error {
	return d.g2.Offset(offset).Limit(limit).Find(resourceList).Error
}

func (d *sqlGenericDao) Preload(preload string) {
	d.g2 = d.g2.Preload(preload)
}

func (d *sqlGenericDao) OrderBy(orderBy string) {
	d.g2 = d.g2.Order(orderBy)
}

func (d *sqlGenericDao) Where(where Where) {
	d.g2 = d.g2.Where(where.sql, where.values...)
}

func (d *sqlGenericDao) Count(model interface{}, total *int64) error {
	// Creates new session which already clears all statement clauses
	g2 := d.g2.Session(&gorm.Session{DryRun: false}).Model(model)
	// Considers existing joins and search params from previous session
	if len(d.g2.Statement.Joins) > 0 {
		g2.Statement.Joins = d.g2.Statement.Joins
	}
	if where, ok := d.g2.Statement.Clauses["WHERE"]; ok {
		g2.Statement.Clauses["WHERE"] = where
	}
	return g2.Count(total).Error
}

// Gorm finishers (Take, First, Last, etc.) are not idempotent
// Use a new session to execute these checks
func (d *sqlGenericDao) Validate(resourceList interface{}) error {
	if err := d.g2.Session(&gorm.Session{DryRun: false}).Take(resourceList).Error; err != nil {
		return err
	}
	return nil
}

func (d *sqlGenericDao) GetTableName() string {
	return db.GetTableName(d.g2)
}
