package migrations

// Migrations should NEVER use types from other packages. Types can change
// and then migrations run on a _new_ database will fail or behave unexpectedly.
// Instead of importing types, always re-create the type in the migration, as
// is done here, even though the same type is defined in pkg/api

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
)

func addClusters() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202511111044",
		Migrate: func(tx *gorm.DB) error {
			// Create clusters table
			// ClusterStatus is stored as JSONB in status_conditions, and status fields
			// are flattened for efficient querying
			createTableSQL := `
				CREATE TABLE IF NOT EXISTS clusters (
					id VARCHAR(255) PRIMARY KEY,
					created_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL,

					-- Core fields
					kind VARCHAR(255) NOT NULL DEFAULT 'Cluster',
					name VARCHAR(63) NOT NULL,
					spec JSONB NOT NULL,
					labels JSONB NULL,
					href VARCHAR(500),

					-- Version control
					generation INTEGER NOT NULL DEFAULT 1,

					-- Status (conditions-only model)
					status_conditions JSONB NULL,

					-- Audit fields
					created_by VARCHAR(255) NOT NULL,
					updated_by VARCHAR(255) NOT NULL
				);
			`

			if err := tx.Exec(createTableSQL).Error; err != nil {
				return err
			}

			// Create index on deleted_at for soft deletes
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_clusters_deleted_at ON clusters(deleted_at);").Error; err != nil {
				return err
			}

			// Create unique index on name (only for non-deleted records)
			if err := tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_clusters_name ON clusters(name) WHERE deleted_at IS NULL;").Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop indexes first
			if err := tx.Exec("DROP INDEX IF EXISTS idx_clusters_name;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_clusters_deleted_at;").Error; err != nil {
				return err
			}
			// Drop table
			if err := tx.Exec("DROP TABLE IF EXISTS clusters;").Error; err != nil {
				return err
			}
			return nil
		},
	}
}
