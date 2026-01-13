package db

import (
	"context"

	dbContext "github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_context"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// NewContext returns a new context with transaction stored in it.
// Upon error, the original context is still returned along with an error
func NewContext(ctx context.Context, connection SessionFactory) (context.Context, error) {
	tx, err := newTransaction(ctx, connection)
	if err != nil {
		return ctx, err
	}

	ctx = dbContext.WithTransaction(ctx, tx)

	return ctx, nil
}

// Resolve resolves the current transaction according to the rollback flag.
func Resolve(ctx context.Context) {
	tx, ok := dbContext.Transaction(ctx)
	if !ok {
		logger.Error(ctx, "Could not retrieve transaction from context")
		return
	}

	if tx.MarkedForRollback() {
		if err := tx.Rollback(); err != nil {
			logger.WithError(ctx, err).Error("Could not rollback transaction")
			return
		}
		logger.Info(ctx, "Rolled back transaction")
	} else {
		if err := tx.Commit(); err != nil {
			// TODO:  what does the user see when this occurs? seems like they will get a false positive
			logger.WithError(ctx, err).Error("Could not commit transaction")
			return
		}
	}
}

// MarkForRollback flags the transaction stored in the context for rollback and logs whatever error caused the rollback
func MarkForRollback(ctx context.Context, err error) {
	transaction, ok := dbContext.Transaction(ctx)
	if !ok {
		logger.Error(ctx, "failed to mark transaction for rollback: could not retrieve transaction from context")
		return
	}
	transaction.SetRollbackFlag(true)
	logger.WithError(ctx, err).Info("Marked transaction for rollback")
}
