package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func addResourceReferences() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202607021925",
		Migrate: func(tx *gorm.DB) error {
			if err := tx.Exec(`CREATE TABLE IF NOT EXISTS resource_references (
				source_id		VARCHAR(255) NOT NULL,
				ref_type		VARCHAR(255) NOT NULL,
				target_id		VARCHAR(255) NOT NULL,
				target_kind		VARCHAR(100) NOT NULL,
				PRIMARY KEY (source_id, ref_type, target_id),
				FOREIGN KEY (source_id) REFERENCES resources(id) ON DELETE CASCADE,
				FOREIGN KEY (target_id) REFERENCES resources(id) ON DELETE RESTRICT
			);`).Error; err != nil {
				return err
			}

			if err := tx.Exec(
				`CREATE INDEX IF NOT EXISTS idx_resource_references_target ON resource_references (target_id);`,
			).Error; err != nil {
				return err
			}

			return nil
		},
		Rollback: func(tx *gorm.DB) error {
			return tx.Exec(`DROP TABLE IF EXISTS resource_references`).Error
		},
	}
}
