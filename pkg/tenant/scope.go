package tenant

import (
	"context"

	"gorm.io/gorm"
)

const tenancyContainsClause = "tenancy @> ?"

// ScopeClause returns the JSONB containment predicate and its bound arguments for
// the caller resolved from ctx: empty clause with nil args for system callers and
// requests with no resolved tenant (unscoped); "1 = 0" with nil args for non-system
// callers with no dimensions (fail-closed, see below); or the containment clause
// with one bound argument for a resolved tenant. Callers must spread args (e.g.
// db.Where(clause, args...)) rather than assume exactly one value.
func ScopeClause(ctx context.Context) (clause string, args []any) {
	t := FromContext(ctx)
	if t == nil || t.System {
		return "", nil
	}
	if len(t.Dimensions) == 0 {
		// Non-system caller with no dimensions must never fall through to
		// unscoped access; the tenant middleware should already reject this case.
		return "1 = 0", nil
	}
	return tenancyContainsClause, []any{TenancyJSON(ctx)}
}

// ScopeDB applies ScopeClause to db as a WHERE clause, for callers using GORM's
// query builder instead of raw SQL.
func ScopeDB(db *gorm.DB, ctx context.Context) *gorm.DB {
	clause, args := ScopeClause(ctx)
	if clause == "" {
		return db
	}
	return db.Where(clause, args...)
}
