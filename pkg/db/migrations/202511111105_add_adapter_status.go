package migrations

// Migrations should NEVER use types from other packages. Types can change
// and then migrations run on a _new_ database will fail or behave unexpectedly.
// Instead of importing types, always re-create the type in the migration, as
// is done here, even though the same type is defined in pkg/api

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
)

func addAdapterStatus() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202511111105",
		Migrate: func(tx *gorm.DB) error {
			// Create adapter_statuses table
			// This table uses polymorphic association to link to either clusters or node_pools
			createTableSQL := `
				CREATE TABLE IF NOT EXISTS adapter_statuses (
					id VARCHAR(255) PRIMARY KEY,
					created_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL,

					-- Polymorphic association
					resource_type VARCHAR(20) NOT NULL,
					resource_id VARCHAR(255) NOT NULL,

					-- Adapter information
					adapter VARCHAR(255) NOT NULL,
					observed_generation INTEGER NOT NULL,

					-- API-managed timestamps
					last_report_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),

					-- Stored as JSONB
					conditions JSONB NOT NULL,
					data JSONB NULL,
					metadata JSONB NULL
				);
			`

			if err := tx.Exec(createTableSQL).Error; err != nil {
				return err
			}

			// Create index on deleted_at for soft deletes
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_adapter_statuses_deleted_at ON adapter_statuses(deleted_at);").Error; err != nil {
				return err
			}

			// Create composite index on resource_type and resource_id for lookups
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_adapter_statuses_resource ON adapter_statuses(resource_type, resource_id);").Error; err != nil {
				return err
			}

			// Create unique index on resource_type, resource_id, and adapter
			// This ensures one adapter status per resource per adapter
			if err := tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_adapter_statuses_unique ON adapter_statuses(resource_type, resource_id, adapter) WHERE deleted_at IS NULL;").Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop indexes
			if err := tx.Exec("DROP INDEX IF EXISTS idx_adapter_statuses_unique;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_adapter_statuses_resource;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_adapter_statuses_deleted_at;").Error; err != nil {
				return err
			}

			// Drop table
			if err := tx.Exec("DROP TABLE IF EXISTS adapter_statuses;").Error; err != nil {
				return err
			}

			return nil
		},
	}
}
