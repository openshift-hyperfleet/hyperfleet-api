package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func dropResourceConditions() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202607300001",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec("DROP INDEX IF EXISTS idx_resource_conditions_type_status;").Error; err != nil {
				return err
			}
			return tx.Exec("DROP TABLE IF EXISTS resource_conditions;").Error
		},
		Rollback: func(tx *gorm.DB) error {
			if err := tx.Exec(`CREATE TABLE IF NOT EXISTS resource_conditions (
				resource_id          VARCHAR(255) NOT NULL,
				type                 VARCHAR(100) NOT NULL,
				status               VARCHAR(10) NOT NULL,
				reason               TEXT,
				message              TEXT,
				observed_generation  INTEGER NOT NULL DEFAULT 0,
				created_time         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				last_updated_time    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				last_transition_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				PRIMARY KEY (resource_id, type),
				FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
			);`).Error; err != nil {
				return err
			}
			return tx.Exec(
				"CREATE INDEX IF NOT EXISTS idx_resource_conditions_type_status " +
					"ON resource_conditions (type, status) " +
					"WHERE status = 'False';",
			).Error
		},
	}
}
