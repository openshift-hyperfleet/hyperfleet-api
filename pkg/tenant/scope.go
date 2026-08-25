package tenant

import (
	"context"

	"gorm.io/gorm"
)

const tenancyContainsClause = "tenancy @> ?"

// ScopeClause returns the JSONB containment predicate and its bound argument for the
// caller resolved from ctx. Returns an empty clause for system callers and requests
// with no resolved tenant (unscoped). Non-system callers with empty dimensions are
// denied (fail-closed). This is the single source of truth for the scoping decision;
// ScopeDB is a thin wrapper around it.
func ScopeClause(ctx context.Context) (clause string, arg any) {
	t := FromContext(ctx)
	if t == nil || t.System {
		return "", nil
	}
	if len(t.Dimensions) == 0 {
		// Non-system caller with no dimensions must never fall through to
		// unscoped access; the tenant middleware should already reject this case.
		return "1 = 0", nil
	}
	return tenancyContainsClause, TenancyJSON(ctx)
}

// ScopeDB applies ScopeClause to db as a WHERE clause, for callers using GORM's
// query builder instead of raw SQL.
func ScopeDB(db *gorm.DB, ctx context.Context) *gorm.DB {
	if clause, arg := ScopeClause(ctx); clause != "" {
		return db.Where(clause, arg)
	}
	return db
}
