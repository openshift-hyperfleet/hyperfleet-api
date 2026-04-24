package migrations

// Migrations should NEVER use types from other packages. Types can change
// and then migrations run on a _new_ database will fail or behave unexpectedly.
// Instead of importing types, always re-create the type in the migration, as
// is done here, even though the same type is defined in pkg/api

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
)

func addNodePoolOwnerDeletedIndex() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202604230001",
		Migrate: func(tx *gorm.DB) error {
			// Add composite index on (owner_id, deleted_time) for efficient queries
			// when fetching soft-deleted nodepools by owner.
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_node_pools_owner_deleted ON node_pools(owner_id, deleted_time);").Error; err != nil { //nolint:lll
				return err
			}
			return nil
		},
	}
}
