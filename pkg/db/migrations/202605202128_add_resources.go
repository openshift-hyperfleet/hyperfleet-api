package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func addResources() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605202128",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`CREATE TABLE IF NOT EXISTS resources (
    								id				VARCHAR(255) PRIMARY KEY,
    								created_time	TIMESTAMPTZ NOT NULL DEFAULT NOW(),
									updated_time	TIMESTAMPTZ NOT NULL DEFAULT NOW(),
									deleted_time	TIMESTAMPTZ NULL,
									kind			VARCHAR(100) NOT NULL,
									name			VARCHAR(100) NOT NULL,
    								href			VARCHAR(500),
									created_by		VARCHAR(255) NOT NULL,
									updated_by		VARCHAR(255) NOT NULL,
									deleted_by		VARCHAR(255) NULL,
									owner_id		VARCHAR(255) NULL,
									owner_kind		VARCHAR(100) NULL,
									owner_href		VARCHAR(500) NULL,
									spec			JSONB NOT NULL,
									labels			JSONB NULL,
									generation		INTEGER NOT NULL DEFAULT 1
									);`).Error; err != nil {
				return err
			}

			if err := tx.Exec(
				"CREATE INDEX IF NOT EXISTS idx_resources_kind ON resources (kind);",
			).Error; err != nil {
				return err
			}

			if err := tx.Exec(
				"CREATE UNIQUE INDEX IF NOT EXISTS idx_resources_kind_name " +
					"ON resources (kind, name) " +
					"WHERE owner_id IS NULL AND deleted_time IS NULL;",
			).Error; err != nil {
				return err
			}

			if err := tx.Exec(
				"CREATE UNIQUE INDEX IF NOT EXISTS idx_resources_kind_owner_name " +
					"ON resources (kind, owner_id, name) " +
					"WHERE owner_id IS NOT NULL AND deleted_time IS NULL;",
			).Error; err != nil {
				return err
			}

			if err := tx.Exec(
				"CREATE INDEX IF NOT EXISTS idx_resources_owner_id " +
					"ON resources (owner_id) WHERE owner_id IS NOT NULL;",
			).Error; err != nil {
				return err
			}

			if err := tx.Exec(
				"CREATE INDEX IF NOT EXISTS idx_resources_deleted_time " +
					"ON resources (deleted_time) WHERE deleted_time IS NOT NULL;",
			).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
