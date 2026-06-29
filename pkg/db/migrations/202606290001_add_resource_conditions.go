package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func addResourceConditions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202606290001",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`CREATE TABLE IF NOT EXISTS resource_conditions (
				resource_id			VARCHAR(255) NOT NULL,
				type				VARCHAR(100) NOT NULL,
				status				VARCHAR(10) NOT NULL,
				reason				TEXT,
				message				TEXT,
				observed_generation	INTEGER NOT NULL DEFAULT 0,
				created_time		TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				last_updated_time	TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				last_transition_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (resource_id, type),
				FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
			);`).Error; err != nil {
				return err
			}

			if err := tx.Exec(
				"CREATE INDEX IF NOT EXISTS idx_resource_conditions_resource_id " +
					"ON resource_conditions (resource_id);",
			).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
