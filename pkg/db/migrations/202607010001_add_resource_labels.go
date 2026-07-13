package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func addResourceLabels() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202607010001",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`CREATE TABLE IF NOT EXISTS resource_labels (
				resource_id		VARCHAR(255) NOT NULL,
				key				VARCHAR(255) NOT NULL,
				value			VARCHAR(255) NOT NULL,
				PRIMARY KEY (resource_id, key),
				FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
			);`).Error; err != nil {
				return err
			}

			if err := tx.Exec(`ALTER TABLE resources DROP COLUMN IF EXISTS labels;`).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
