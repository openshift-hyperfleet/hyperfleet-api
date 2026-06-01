package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// removeReadyCondition migrates the deprecated Ready condition out of status_conditions JSONB:
//  1. Backfill: copy Ready → Reconciled for any row that has Ready but not Reconciled.
//  2. Strip: remove all Ready entries from the JSONB arrays.
//  3. Drop the Ready-specific indexes created by 202601210001.
func removeReadyCondition() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605280001",
		Migrate: func(tx *gorm.DB) error {
			for _, table := range []string{"clusters", "node_pools"} {
				// Backfill: if a row has Ready but no Reconciled, copy Ready as Reconciled.
				if err := tx.Exec(`
					UPDATE ` + table + `
					SET status_conditions = status_conditions || (
						SELECT jsonb_agg(
							jsonb_set(elem, '{type}', '"Reconciled"')
						)
						FROM jsonb_array_elements(status_conditions) AS elem
						WHERE elem->>'type' = 'Ready'
					)
					WHERE status_conditions @> '[{"type":"Ready"}]'
					  AND NOT status_conditions @> '[{"type":"Reconciled"}]'
				`).Error; err != nil {
					return err
				}

				// Strip all Ready entries from the JSONB array.
				if err := tx.Exec(`
					UPDATE ` + table + `
					SET status_conditions = (
						SELECT COALESCE(jsonb_agg(elem), '[]'::jsonb)
						FROM jsonb_array_elements(status_conditions) AS elem
						WHERE elem->>'type' != 'Ready'
					)
					WHERE status_conditions @> '[{"type":"Ready"}]'
				`).Error; err != nil {
					return err
				}
			}

			// Drop the Ready-specific indexes created by 202601210001.
			if err := tx.Exec(`DROP INDEX IF EXISTS idx_clusters_ready_status`).Error; err != nil {
				return err
			}
			if err := tx.Exec(`DROP INDEX IF EXISTS idx_node_pools_ready_status`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
