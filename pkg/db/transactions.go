package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_metrics"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/internal/txcontext"
)

// TransactionRunner executes callbacks inside database transactions.
type TransactionRunner struct {
	connection SessionFactory
}

// NewTxRunner creates the transaction dependency used by mutation services.
// Container construction guarantees connection is initialized.
func NewTxRunner(connection SessionFactory) *TransactionRunner {
	return &TransactionRunner{connection: connection}
}

// Do runs callback in a transaction scoped to a derived context. Returning an
// error rolls back; returning nil commits before this method returns.
func (r *TransactionRunner) Do(ctx context.Context, callback func(context.Context) error) error {
	if txcontext.Has(ctx) {
		return errors.New("db: nested transactions are not supported")
	}

	gormTx := r.connection.New(ctx).Begin()
	if gormTx.Error != nil {
		slog.ErrorContext(ctx, "Could not begin transaction", "error", gormTx.Error)
		recordTransactionError("begin", "begin_failed")
		return fmt.Errorf("db: begin transaction: %w", gormTx.Error)
	}
	txCtx := txcontext.Attach(ctx, gormTx)
	completed := false
	defer func() {
		if !completed {
			rollbackErr := gormTx.Rollback().Error
			if rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
				slog.ErrorContext(txCtx, "Could not rollback transaction", "error", rollbackErr)
				recordTransactionError("rollback", "rollback_failed")
			}
		}
	}()

	if err := callback(txCtx); err != nil {
		return fmt.Errorf("db: transaction callback: %w", err)
	}
	if err := gormTx.Commit().Error; err != nil {
		slog.ErrorContext(txCtx, "Could not commit transaction", "error", err)
		recordTransactionError("commit", "commit_failed")
		return fmt.Errorf("db: commit transaction: %w", err)
	}
	completed = true
	return nil
}

// recordTransactionError records transaction completion failures.
func recordTransactionError(operation, errorType string) {
	db_metrics.ErrorsMetric.With(prometheus.Labels{
		"operation":  operation,
		"error_type": errorType,
		"component":  "api",
		"version":    api.Version,
	}).Inc()
}
