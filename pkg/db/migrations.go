package db

import (
	"context"
	"os"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/migrations"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// gormigrate is a wrapper for gorm's migration functions that adds schema versioning
// and rollback capabilities. For help writing migration steps, see the gorm documentation
// on migrations: http://doc.gorm.io/database.html#migration

func Migrate(g2 *gorm.DB) error {
	m := newGormigrate(g2)

	if err := m.Migrate(); err != nil {
		return err
	}
	return nil
}

// MigrateTo a specific migration will not seed the database, seeds are up to date with the latest
// schema based on the most recent migration
// This should be for testing purposes mainly
func MigrateTo(sessionFactory SessionFactory, migrationID string) {
	ctx := context.Background()
	g2 := sessionFactory.New(ctx)
	m := newGormigrate(g2)

	if err := m.MigrateTo(migrationID); err != nil {
		logger.With(ctx, logger.FieldMigrationID, migrationID).WithError(err).Error("Could not migrate")
		os.Exit(1)
	}
}

func newGormigrate(g2 *gorm.DB) *gormigrate.Gormigrate {
	return gormigrate.New(g2, gormigrate.DefaultOptions, migrations.MigrationList)
}
