package migrations

// This migration drops the legacy clusters and node_pools tables.
// These tables are replaced by the generic 'resources' table that
// handles all CRD-based resource types.

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
)

func dropClusterNodePoolTables() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202602070001",
		Migrate: func(tx *gorm.DB) error {
			// Drop FK constraint from node_pools to clusters first
			if err := tx.Exec("ALTER TABLE IF EXISTS node_pools DROP CONSTRAINT IF EXISTS fk_node_pools_clusters;").Error; err != nil {
				return err
			}

			// Drop indexes on node_pools
			if err := tx.Exec("DROP INDEX IF EXISTS idx_node_pools_owner_id;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_node_pools_deleted_at;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_node_pools_status_conditions;").Error; err != nil {
				return err
			}

			// Drop node_pools table
			if err := tx.Exec("DROP TABLE IF EXISTS node_pools;").Error; err != nil {
				return err
			}

			// Drop indexes on clusters
			if err := tx.Exec("DROP INDEX IF EXISTS idx_clusters_name;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_clusters_deleted_at;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_clusters_status_conditions;").Error; err != nil {
				return err
			}

			// Drop clusters table
			if err := tx.Exec("DROP TABLE IF EXISTS clusters;").Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Rollback would recreate the tables, but since we're removing this functionality,
			// we don't provide a rollback. The resources table is the new canonical storage.
			// If you need to rollback, restore from backup or re-run the old migrations.
			return nil
		},
	}
}
