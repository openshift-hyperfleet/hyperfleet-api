package migrations

// Migrations should NEVER use types from other packages. Types can change
// and then migrations run on a _new_ database will fail or behave unexpectedly.
// Instead of importing types, always re-create the type in the migration.

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
)

func addResources() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202602060001",
		Migrate: func(tx *gorm.DB) error {
			// Create generic resources table
			// This table stores all CRD-based resources using a single schema.
			// The Kind column distinguishes resource types.
			createTableSQL := `
				CREATE TABLE IF NOT EXISTS resources (
					id VARCHAR(255) PRIMARY KEY,
					created_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL,

					-- Core fields
					kind VARCHAR(63) NOT NULL,
					name VARCHAR(63) NOT NULL,
					spec JSONB NOT NULL,
					labels JSONB NULL,
					href VARCHAR(500),

					-- Version control
					generation INTEGER NOT NULL DEFAULT 1,

					-- Owner references (for owned resources)
					owner_id VARCHAR(255) NULL,
					owner_kind VARCHAR(63) NULL,
					owner_href VARCHAR(500) NULL,

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
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_resources_deleted_at ON resources(deleted_at);").Error; err != nil {
				return err
			}

			// Create index on kind for efficient filtering by resource type
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_resources_kind ON resources(kind);").Error; err != nil {
				return err
			}

			// Create index on owner_id for efficient lookup of owned resources
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_resources_owner_id ON resources(owner_id);").Error; err != nil {
				return err
			}

			// Create composite index on kind + owner_id for owned resource queries
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_resources_kind_owner ON resources(kind, owner_id);").Error; err != nil {
				return err
			}

			// Create unique index on (kind, name) for root resources (where owner_id IS NULL)
			// This ensures unique names per kind for root-level resources
			createRootUniqueIndexSQL := `
				CREATE UNIQUE INDEX IF NOT EXISTS idx_resources_root_kind_name
				ON resources(kind, name)
				WHERE deleted_at IS NULL AND owner_id IS NULL;
			`
			if err := tx.Exec(createRootUniqueIndexSQL).Error; err != nil {
				return err
			}

			// Create unique index on (owner_id, kind, name) for owned resources
			// This ensures unique names per kind within each owner
			createOwnedUniqueIndexSQL := `
				CREATE UNIQUE INDEX IF NOT EXISTS idx_resources_owned_kind_name
				ON resources(owner_id, kind, name)
				WHERE deleted_at IS NULL AND owner_id IS NOT NULL;
			`
			if err := tx.Exec(createOwnedUniqueIndexSQL).Error; err != nil {
				return err
			}

			// Create GIN index on status_conditions for efficient condition queries
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_resources_status_conditions ON resources USING GIN(status_conditions);").Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop indexes first
			if err := tx.Exec("DROP INDEX IF EXISTS idx_resources_status_conditions;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_resources_owned_kind_name;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_resources_root_kind_name;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_resources_kind_owner;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_resources_owner_id;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_resources_kind;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_resources_deleted_at;").Error; err != nil {
				return err
			}
			// Drop table
			if err := tx.Exec("DROP TABLE IF EXISTS resources;").Error; err != nil {
				return err
			}
			return nil
		},
	}
}
