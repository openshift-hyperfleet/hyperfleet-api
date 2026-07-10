package migrations

// Migrations should NEVER use types from other packages. Types can change
// and then migrations run on a _new_ database will fail or behave unexpectedly.
// Instead of importing types, always re-create the type in the migration, as
// is done here, even though the same type is defined in pkg/api

import (
	"fmt"

	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
)

func addAdapterStatus() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202511111105",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`CREATE TABLE IF NOT EXISTS adapter_statuses (
				id                 VARCHAR(255) PRIMARY KEY,
				created_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_time       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				resource_type      VARCHAR(20) NOT NULL,
				resource_id        VARCHAR(255) NOT NULL,
				adapter            VARCHAR(255) NOT NULL,
				observed_generation INTEGER NOT NULL,
				last_report_time   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				conditions         JSONB NOT NULL,
				data               JSONB NULL,
				metadata           JSONB NULL
			);`).Error; err != nil {
				return fmt.Errorf("create adapter_statuses table: %w", err)
			}

			if err := tx.Exec(
				"CREATE INDEX IF NOT EXISTS idx_adapter_statuses_resource " +
					"ON adapter_statuses(resource_type, resource_id);",
			).Error; err != nil {
				return fmt.Errorf("create idx_adapter_statuses_resource index: %w", err)
			}

			if err := tx.Exec(
				"CREATE UNIQUE INDEX IF NOT EXISTS idx_adapter_statuses_unique " +
					"ON adapter_statuses(resource_type, resource_id, adapter);",
			).Error; err != nil {
				return fmt.Errorf("create idx_adapter_statuses_unique index: %w", err)
			}

			return nil
		},
	}
}
