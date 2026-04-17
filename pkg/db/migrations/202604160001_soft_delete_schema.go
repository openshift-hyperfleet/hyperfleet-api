package migrations

// Migrations should NEVER use types from other packages. Types can change
// and then migrations run on a _new_ database will fail or behave unexpectedly.
// Instead of importing types, always re-create the type in the migration, as
// is done here, even though the same type is defined in pkg/api

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
)

func addSoftDeleteSchema() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202604160001",
		Migrate: func(tx *gorm.DB) error {
			// clusters: rename deleted_at → deleted_time, add deleted_by, fix partial index
			if err := tx.Exec("ALTER TABLE clusters RENAME COLUMN deleted_at TO deleted_time;").Error; err != nil {
				return err
			}
			if err := tx.Exec("ALTER TABLE clusters ADD COLUMN IF NOT EXISTS deleted_by VARCHAR(255) NULL;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_clusters_deleted_at;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_clusters_name;").Error; err != nil {
				return err
			}
			if err := tx.Exec("CREATE UNIQUE INDEX idx_clusters_name ON clusters(name) WHERE deleted_time IS NULL;").Error; err != nil { //nolint:lll
				return err
			}

			// node_pools: rename deleted_at → deleted_time, add deleted_by, fix partial index
			if err := tx.Exec("ALTER TABLE node_pools RENAME COLUMN deleted_at TO deleted_time;").Error; err != nil {
				return err
			}
			if err := tx.Exec("ALTER TABLE node_pools ADD COLUMN IF NOT EXISTS deleted_by VARCHAR(255) NULL;").Error; err != nil { //nolint:lll
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_node_pools_deleted_at;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_node_pools_owner_name;").Error; err != nil {
				return err
			}
			if err := tx.Exec("CREATE UNIQUE INDEX idx_node_pools_owner_name ON node_pools(owner_id, name) WHERE deleted_time IS NULL;").Error; err != nil { //nolint:lll
				return err
			}

			// adapter_statuses: drop unused deleted_at column, make unique index unconditional
			if err := tx.Exec("ALTER TABLE adapter_statuses DROP COLUMN IF EXISTS deleted_at;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_adapter_statuses_deleted_at;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_adapter_statuses_unique;").Error; err != nil {
				return err
			}
			if err := tx.Exec("CREATE UNIQUE INDEX idx_adapter_statuses_unique ON adapter_statuses(resource_type, resource_id, adapter);").Error; err != nil { //nolint:lll
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// adapter_statuses: restore deleted_at column and partial unique index
			if err := tx.Exec("DROP INDEX IF EXISTS idx_adapter_statuses_unique;").Error; err != nil {
				return err
			}
			if err := tx.Exec("CREATE UNIQUE INDEX idx_adapter_statuses_unique ON adapter_statuses(resource_type, resource_id, adapter) WHERE deleted_at IS NULL;").Error; err != nil { //nolint:lll
				return err
			}
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_adapter_statuses_deleted_at ON adapter_statuses(deleted_at);").Error; err != nil { //nolint:lll
				return err
			}
			if err := tx.Exec("ALTER TABLE adapter_statuses ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ NULL;").Error; err != nil { //nolint:lll
				return err
			}

			// node_pools: restore deleted_at, remove deleted_by, restore index
			if err := tx.Exec("DROP INDEX IF EXISTS idx_node_pools_owner_name;").Error; err != nil {
				return err
			}
			if err := tx.Exec("CREATE UNIQUE INDEX idx_node_pools_owner_name ON node_pools(owner_id, name) WHERE deleted_at IS NULL;").Error; err != nil { //nolint:lll
				return err
			}
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_node_pools_deleted_at ON node_pools(deleted_at);").Error; err != nil { //nolint:lll
				return err
			}
			if err := tx.Exec("ALTER TABLE node_pools DROP COLUMN IF EXISTS deleted_by;").Error; err != nil {
				return err
			}
			if err := tx.Exec("ALTER TABLE node_pools RENAME COLUMN deleted_time TO deleted_at;").Error; err != nil {
				return err
			}

			// clusters: restore deleted_at, remove deleted_by, restore index
			if err := tx.Exec("DROP INDEX IF EXISTS idx_clusters_name;").Error; err != nil {
				return err
			}
			if err := tx.Exec("CREATE UNIQUE INDEX idx_clusters_name ON clusters(name) WHERE deleted_at IS NULL;").Error; err != nil { //nolint:lll
				return err
			}
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_clusters_deleted_at ON clusters(deleted_at);").Error; err != nil {
				return err
			}
			if err := tx.Exec("ALTER TABLE clusters DROP COLUMN IF EXISTS deleted_by;").Error; err != nil {
				return err
			}
			if err := tx.Exec("ALTER TABLE clusters RENAME COLUMN deleted_time TO deleted_at;").Error; err != nil {
				return err
			}

			return nil
		},
	}
}
