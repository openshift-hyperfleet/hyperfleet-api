package migrations

import (
	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

func addResources() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202605202128",
		Migrate: func(tx *gorm.DB) error {
			// ── resources ────────────────────────────────────────────
			if err := tx.Exec(`CREATE TABLE IF NOT EXISTS resources (
				id            VARCHAR(255) PRIMARY KEY,
				created_time  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				updated_time  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
				deleted_time  TIMESTAMPTZ NULL,
				kind          VARCHAR(100) NOT NULL,
				name          VARCHAR(100) NOT NULL,
				href          VARCHAR(500),
				created_by    VARCHAR(255) NOT NULL,
				updated_by    VARCHAR(255) NOT NULL,
				deleted_by    VARCHAR(255) NULL,
				owner_id      VARCHAR(255) NULL,
				owner_kind    VARCHAR(100) NULL,
				owner_href    VARCHAR(500) NULL,
				spec          JSONB NOT NULL,
				generation    INTEGER NOT NULL DEFAULT 1
			);`).Error; err != nil {
				return err
			}

			for _, idx := range []string{
				"CREATE INDEX IF NOT EXISTS idx_resources_kind ON resources (kind);",

				"CREATE UNIQUE INDEX IF NOT EXISTS idx_resources_kind_name " +
					"ON resources (kind, name) " +
					"WHERE owner_id IS NULL AND deleted_time IS NULL;",

				"CREATE UNIQUE INDEX IF NOT EXISTS idx_resources_kind_owner_name " +
					"ON resources (kind, owner_id, name) " +
					"WHERE owner_id IS NOT NULL AND deleted_time IS NULL;",

				"CREATE INDEX IF NOT EXISTS idx_resources_owner_id " +
					"ON resources (owner_id) WHERE owner_id IS NOT NULL;",

				"CREATE INDEX IF NOT EXISTS idx_resources_deleted_time " +
					"ON resources (deleted_time) WHERE deleted_time IS NOT NULL;",
			} {
				if err := tx.Exec(idx).Error; err != nil {
					return err
				}
			}

			// ── resource_conditions ──────────────────────────────────
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

			// ── resource_labels ──────────────────────────────────────
			if err := tx.Exec(`CREATE TABLE IF NOT EXISTS resource_labels (
				resource_id VARCHAR(255) NOT NULL,
				key         VARCHAR(255) NOT NULL,
				value       VARCHAR(255) NOT NULL,
				PRIMARY KEY (resource_id, key),
				FOREIGN KEY (resource_id) REFERENCES resources(id) ON DELETE CASCADE
			);`).Error; err != nil {
				return err
			}

			// ── resource_references ──────────────────────────────────
			if err := tx.Exec(`CREATE TABLE IF NOT EXISTS resource_references (
				source_id   VARCHAR(255) NOT NULL,
				ref_type    VARCHAR(255) NOT NULL,
				target_id   VARCHAR(255) NOT NULL,
				target_kind VARCHAR(100) NOT NULL,
				PRIMARY KEY (source_id, ref_type, target_id),
				FOREIGN KEY (source_id) REFERENCES resources(id) ON DELETE CASCADE,
				FOREIGN KEY (target_id) REFERENCES resources(id) ON DELETE RESTRICT
			);`).Error; err != nil {
				return err
			}

			if err := tx.Exec(
				"CREATE INDEX IF NOT EXISTS idx_resource_references_target " +
					"ON resource_references (target_id);",
			).Error; err != nil {
				return err
			}

			return nil
		},
	}
}
