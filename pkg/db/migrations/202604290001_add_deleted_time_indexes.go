package migrations

import (
	"gorm.io/gorm"

	"github.com/go-gormigrate/gormigrate/v2"
)

func addDeletedTimeIndexes() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202604290001",
		Migrate: func(tx *gorm.DB) error {
			// Partial indexes for metrics collector queries:
			//   SELECT COUNT(*) FROM clusters WHERE deleted_time IS NOT NULL AND deleted_time < $1
			//   SELECT COUNT(*) FROM node_pools WHERE deleted_time IS NOT NULL AND deleted_time < $1
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_clusters_deleted_time ON clusters(deleted_time) WHERE deleted_time IS NOT NULL;").Error; err != nil { //nolint:lll
				return err
			}
			if err := tx.Exec("CREATE INDEX IF NOT EXISTS idx_node_pools_deleted_time ON node_pools(deleted_time) WHERE deleted_time IS NOT NULL;").Error; err != nil { //nolint:lll
				return err
			}
			return nil
		},
	}
}
