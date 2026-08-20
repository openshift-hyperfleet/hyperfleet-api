package migrations

import (
	"fmt"

	"github.com/go-gormigrate/gormigrate/v2"
	"gorm.io/gorm"
)

// idxResourcesKindName is the root-resource uniqueness index this migration replaces
// in place (same name, new definition).
const idxResourcesKindName = "idx_resources_kind_name"

// addScopeResourceNameByTenant makes root-resource name uniqueness tenant-scoped.
//
// Previously idx_resources_kind_name enforced UNIQUE (kind, name) globally for root
// resources (owner_id IS NULL). That leaked existence across tenants: a create colliding
// with another tenant's resource returned 409, revealing that the name was already taken
// somewhere. This recreates the index to include the tenancy document, so uniqueness is
// scoped to the caller's tenancy: two tenants can both own a "prod" cluster, while a
// same-tenancy collision still returns 409.
//
// This intentionally does NOT use CREATE INDEX CONCURRENTLY / DROP INDEX CONCURRENTLY.
// MigrateWithLock (pkg/db/migrations.go) holds a PostgreSQL advisory lock for the entire
// migration run via a transaction that stays open until all migrations finish
// (pkg/db/advisory_locks.go). CONCURRENTLY must wait for every transaction open at its
// start — including that lock-holding transaction on another connection in the same
// process — to complete before it can finish, which can never happen while it's the one
// blocking that same migration run. The result is a self-deadlock: the CONCURRENTLY
// statement hangs, no other pod can acquire the advisory lock, and waiters fail with a
// statement_timeout once it elapses. A plain, non-concurrent index rebuild avoids this;
// the brief write lock on `resources` is acceptable pre-production (see
// multi-tenant-identity-authz-design.md: "HyperFleet is pre-production. Schema changes
// do not require a migration strategy for existing production data.").
//
// The tenancy column is JSONB and normalized by Postgres (key order is irrelevant), so a
// btree unique index over it compares the whole document by value. Unscoped and system
// callers get an empty tenancy ('{}'), which preserves the previous global behavior among
// themselves. Child resources (owner_id IS NOT NULL) are already tenant-isolated via their
// globally-unique owner_id, so idx_resources_kind_owner_name is left unchanged.
//
// The index stores the whole normalized tenancy document in each tuple. Tenancy keys are
// config-bounded and values come from short JWT-derived dimensions, so documents stay well
// under the btree tuple limit (~2704 bytes). If tenancy ever grows unbounded, switch this to
// an expression index over a hashed/normalized form (e.g. md5(tenancy::text)) to stay safe.
//
// The drop and create run inside an explicit tx.Transaction so they succeed or fail
// together: if the CREATE fails, the DROP is rolled back too, and root resources are never
// left without a uniqueness index. This is independent of gormigrate's own UseTransaction
// setting (false project-wide) and is not a migration Rollback — per this package's
// fix-forward policy, Migrate always runs forward; this only makes its own two statements
// atomic within a single run.
func addScopeResourceNameByTenant() *gormigrate.Migration {
	return &gormigrate.Migration{
		ID: "202608181200",
		Migrate: func(tx *gorm.DB) error {
			return tx.Transaction(func(tx *gorm.DB) error {
				if err := tx.Exec(
					"DROP INDEX IF EXISTS " + idxResourcesKindName + ";",
				).Error; err != nil {
					return fmt.Errorf("drop legacy resource-name index: %w", err)
				}

				if err := tx.Exec(
					"CREATE UNIQUE INDEX IF NOT EXISTS " + idxResourcesKindName + " " +
						"ON resources (kind, name, tenancy) " +
						"WHERE owner_id IS NULL AND deleted_time IS NULL;",
				).Error; err != nil {
					return fmt.Errorf("create tenant-scoped resource-name index: %w", err)
				}
				return nil
			})
		},
	}
}
