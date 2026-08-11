package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func addResourceTenancy() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202608111200",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(
				"ALTER TABLE resources ADD COLUMN IF NOT EXISTS tenancy JSONB NOT NULL DEFAULT '{}'::jsonb;",
			).Error; err != nil {
				return err
			}

			return tx.Exec(
				"CREATE INDEX IF NOT EXISTS idx_resources_tenancy " +
					"ON resources USING GIN (tenancy jsonb_path_ops);",
			).Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(
				"DROP INDEX IF EXISTS idx_resources_tenancy;",
			).Error; err != nil {
				return err
			}

			return tx.Exec(
				"ALTER TABLE resources DROP COLUMN IF EXISTS tenancy;",
			).Error
		},
	}
}
