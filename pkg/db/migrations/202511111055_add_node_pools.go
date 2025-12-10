package migrations

// Migrations should NEVER use types from other packages. Types can change
// and then migrations run on a _new_ database will fail or behave unexpectedly.
// Instead of importing types, always re-create the type in the migration, as
// is done here, even though the same type is defined in pkg/api

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
)

func addNodePools() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202511111055",
		Migrate: func(tx *gorm.DB) error {
			// Create node_pools table
			createTableSQL := `
				CREATE TABLE IF NOT EXISTS node_pools (
					id VARCHAR(255) PRIMARY KEY,
					created_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					updated_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
					deleted_at TIMESTAMPTZ NULL,

					-- Core fields
					kind VARCHAR(255) NOT NULL DEFAULT 'NodePool',
					name VARCHAR(255) NOT NULL,
					spec JSONB NOT NULL,
					labels JSONB NULL,
					href VARCHAR(500),

					-- Owner References (flattened)
					owner_id VARCHAR(255) NOT NULL,
					owner_kind VARCHAR(50) NOT NULL,
					owner_href VARCHAR(500) NULL,

					-- Version control
					generation INTEGER NOT NULL DEFAULT 1,

					-- Status fields (flattened for efficient querying)
					status_phase VARCHAR(50) NOT NULL DEFAULT 'NotReady',
					status_last_transition_time TIMESTAMPTZ NULL,
					status_observed_generation INTEGER NOT NULL DEFAULT 0,
					status_last_updated_time TIMESTAMPTZ NULL,
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
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_node_pools_deleted_at ON node_pools(deleted_at);").Error; err != nil {
				return err
			}

			// Create index on owner_id for foreign key lookups
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_node_pools_owner_id ON node_pools(owner_id);").Error; err != nil {
				return err
			}

			// Create index on status_phase for filtering
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_node_pools_status_phase ON node_pools(status_phase);").Error; err != nil {
				return err
			}

			// Create index on status_last_updated_time for search optimization
			// Sentinel queries frequently filter by this field to find stale resources
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_node_pools_status_last_updated_time ON node_pools(status_last_updated_time);").Error; err != nil {
				return err
			}

			// Add foreign key constraint to clusters
			addFKSQL := `
				ALTER TABLE node_pools
				ADD CONSTRAINT fk_node_pools_clusters
				FOREIGN KEY (owner_id) REFERENCES clusters(id)
				ON DELETE RESTRICT ON UPDATE RESTRICT;
			`
			if err := tx.Exec(addFKSQL).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop foreign key constraint first
			if err := tx.Exec("ALTER TABLE node_pools DROP CONSTRAINT IF EXISTS fk_node_pools_clusters;").Error; err != nil {
				return err
			}

			// Drop indexes
			if err := tx.Exec("DROP INDEX IF EXISTS idx_node_pools_status_last_updated_time;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_node_pools_status_phase;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_node_pools_owner_id;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_node_pools_deleted_at;").Error; err != nil {
				return err
			}

			// Drop table
			if err := tx.Exec("DROP TABLE IF EXISTS node_pools;").Error; err != nil {
				return err
			}

			return nil
		},
	}
}
