// Package txcontext carries the active transaction session between the runner
// and session factories. It is internal so application packages cannot manage
// transaction lifecycle directly.
package txcontext

import (
	"context"

	"gorm.io/gorm"
)

type contextKey struct{}

// Attach returns a derived context with an immutable transaction session. A
// retained context continues to refer to the completed SQL transaction, whose
// operations fail with sql.ErrTxDone instead of using a new session.
func Attach(ctx context.Context, db *gorm.DB) context.Context {
	return context.WithValue(ctx, contextKey{}, db)
}

// Session returns the participating database session when ctx has a transaction.
func Session(ctx context.Context) (*gorm.DB, bool) {
	db, ok := ctx.Value(contextKey{}).(*gorm.DB)
	return db, ok
}

// Has reports whether ctx belongs to a transaction, including a completed one.
func Has(ctx context.Context) bool {
	_, ok := ctx.Value(contextKey{}).(*gorm.DB)
	return ok
}
