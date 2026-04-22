package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// addReconciledIndex adds expression indexes on the Reconciled condition
// within status_conditions JSONB columns for efficient lookups.
func addReconciledIndex() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202604211859",
		Migrate: func(tx *gorm.DB) error {
			// Create expression index on clusters for Reconciled condition lookups
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_clusters_reconciled_status
				ON clusters USING BTREE ((
					jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")')
				));
			`).Error; err != nil {
				return err
			}

			// Create expression index on node_pools for Reconciled condition lookups
			if err := tx.Exec(`
				CREATE INDEX IF NOT EXISTS idx_node_pools_reconciled_status
				ON node_pools USING BTREE ((
					jsonb_path_query_first(status_conditions, '$[*] ? (@.type == "Reconciled")')
				));
			`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
