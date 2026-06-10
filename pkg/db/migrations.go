package db

import (
	"context"

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

// MigrateWithLock runs migrations with an advisory lock to prevent concurrent migrations
func MigrateWithLock(ctx context.Context, factory SessionFactory) error {
	// Acquire advisory lock for migrations
	ctx, lockOwnerID, err := NewAdvisoryLockContext(ctx, factory, MigrationsLockID, Migrations)
	if err != nil {
		logger.WithError(ctx, err).Error("Could not lock migrations")
		return err
	}
	defer Unlock(ctx, lockOwnerID)

	// Run migrations with the locked context
	g2 := factory.New(ctx)
	if err := Migrate(g2); err != nil {
		logger.WithError(ctx, err).Error("Could not migrate")
		return err
	}

	logger.Info(ctx, "Migration completed successfully")
	return nil
}

func newGormigrate(g2 *gorm.DB) *gormigrate.Gormigrate {
	return gormigrate.New(g2, gormigrate.DefaultOptions, migrations.MigrationList)
}
