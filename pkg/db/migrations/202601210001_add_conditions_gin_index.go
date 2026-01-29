package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// addConditionsGinIndex adds expression indexes on the Ready condition
// within status_conditions JSONB columns for efficient lookups.
func addConditionsGinIndex() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202601210001",
		Migrate: func(tx *gorm.DB) error {
			// Create expression index on clusters for Ready condition lookups
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_clusters_ready_status
				ON clusters USING BTREE ((
					jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Ready")')
				));
			`).Error; err != nil {
				return err
			}

			// Create expression index on node_pools for Ready condition lookups
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_node_pools_ready_status
				ON node_pools USING BTREE ((
					jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Ready")')
				));
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec("DROP INDEX IF EXISTS idx_clusters_ready_status;").Error; err != nil {
				return err
			}
			if err := tx.Exec("DROP INDEX IF EXISTS idx_node_pools_ready_status;").Error; err != nil {
				return err
			}
			return nil
		},
	}
}
