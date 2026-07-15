package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func addConditionStatusIndex() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202607140001",
		Migrate: func(tx *gorm.DB) error {
			return tx.Exec(
				"CREATE INDEX IF NOT EXISTS idx_resource_conditions_type_status " +
					"ON resource_conditions (type, status) " +
					"WHERE status = 'False';",
			).Error
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(
				"DROP INDEX IF EXISTS idx_resource_conditions_type_status;",
			).Error
		},
	}
}
