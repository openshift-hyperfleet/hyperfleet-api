package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// addClusterUUID adds RFC4122 UUID field to clusters table.
// UUIDs are immutable identifiers for platform integrations requiring standard UUID format.
func addClusterUUID() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202603230001",
		Migrate: func(tx *gorm.DB) error {
			// Step 1: Add uuid column (nullable initially for backfill)
			if err := tx.Exec(`
				ALTER TABLE clusters
				ADD COLUMN uuid VARCHAR(36);
			`).Error; err != nil {
				return err
			}

			// Step 2: Backfill UUIDs for existing clusters using PostgreSQL's gen_random_uuid()
			if err := tx.Exec(`
				UPDATE clusters
				SET uuid = gen_random_uuid()::text
				WHERE uuid IS NULL;
			`).Error; err != nil {
				return err
			}

			// Step 3: Make column NOT NULL after backfill
			if err := tx.Exec(`
				ALTER TABLE clusters
				ALTER COLUMN uuid SET NOT NULL;
			`).Error; err != nil {
				return err
			}

			// Step 4: Add unique constraint (only for non-deleted records)
			if err := tx.Exec(`
				CREATE UNIQUE INDEX IF NOT EXISTS idx_clusters_uuid
				ON clusters(uuid) WHERE deleted_at IS NULL;
			`).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			// Drop index first
			if err := tx.Exec("DROP INDEX IF EXISTS idx_clusters_uuid;").Error; err != nil {
				return err
			}

			// Drop column
			if err := tx.Exec("ALTER TABLE clusters DROP COLUMN IF EXISTS uuid;").Error; err != nil {
				return err
			}

			return nil
		},
	}
}
