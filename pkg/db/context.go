package db

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	dbContext "github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_context"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_metrics"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

type advisoryLockKey string

const (
	advisoryLock advisoryLockKey = "advisoryLock"
)

type advisoryLockMap map[string]*AdvisoryLock

func (m advisoryLockMap) key(id string, lockType LockType) string {
	return id + ":" + string(lockType)
}

func (m advisoryLockMap) get(id string, lockType LockType) (*AdvisoryLock, bool) {
	lock, ok := m[m.key(id, lockType)]
	return lock, ok
}

func (m advisoryLockMap) set(id string, lockType LockType, lock *AdvisoryLock) {
	m[m.key(id, lockType)] = lock
}

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

// Resolve commits or rolls back the transaction based on the rollback flag.
// Should only be called by TransactionMiddleware for write operations.
func Resolve(ctx context.Context) {
	tx, ok := dbContext.Transaction(ctx)
	if !ok {
		slog.ErrorContext(ctx,
			"Transaction resolution failed: no active transaction in context", "error_type", "missing_transaction",
			"error", "no active transaction found in context",
		)
		return
	}

	if tx.MarkedForRollback() {
		if err := tx.Rollback(); err != nil {
			slog.ErrorContext(ctx, "Could not rollback transaction", "error", err)
			recordTransactionError("rollback", "rollback_failed")
			return
		}
		slog.InfoContext(ctx, "Rolled back transaction")
	} else {
		if err := tx.Commit(); err != nil {
			slog.ErrorContext(ctx, "Could not commit transaction", "error", err)
			recordTransactionError("commit", "commit_failed")
			return
		}
	}
}

// MarkForRollback flags the transaction stored in the context for rollback and logs whatever error caused the rollback
func MarkForRollback(ctx context.Context, err error) {
	transaction, ok := dbContext.Transaction(ctx)
	if !ok {
		slog.ErrorContext(ctx, "failed to mark transaction for rollback: could not retrieve transaction from context")
		return
	}
	transaction.SetRollbackFlag(true)
	slog.InfoContext(ctx, "Marked transaction for rollback", "error", err)
}

// NewAdvisoryLockContext returns a new context with AdvisoryLock stored in it.
// Upon error, the original context is still returned along with an error.
//
// IMPORTANT: Advisory locks are for cross-pod coordination (e.g., migrations, scheduled jobs),
// NOT for database row-level concurrency. For row-level concurrency, use SELECT FOR UPDATE.
//
// CONCURRENCY: The returned context must not be shared across goroutines that call
// NewAdvisoryLockContext or Unlock concurrently, as the internal lock map is not
// protected by a mutex. Each goroutine should derive its own context chain.
func NewAdvisoryLockContext(
	ctx context.Context, connection SessionFactory, id string, lockType LockType,
) (context.Context, string, error) {
	// FAIL-FAST: Detect transaction created before advisory lock
	if _, hasTransaction := dbContext.Transaction(ctx); hasTransaction {
		return ctx, "", errors.New(
			"advisory lock cannot be acquired within an existing transaction.\n" +
				"This causes a race condition where lock is released before transaction commits.\n\n" +
				"Correct patterns:\n" +
				"  1. For pod coordination: Acquire lock BEFORE transaction\n" +
				"  2. For row-level concurrency: Use SELECT FOR UPDATE instead",
		)
	}

	// lockOwnerID will be different for every service function that attempts to start a lock.
	// only the initial call in the stack must unlock.
	// Unlock() will compare UUIDs and ensure only the top level call succeeds.
	lockOwnerID := uuid.New().String()

	locks, found := ctx.Value(advisoryLock).(advisoryLockMap)
	if found {
		if _, ok := locks.get(id, lockType); ok {
			return ctx, lockOwnerID, nil
		}
	} else {
		locks = make(advisoryLockMap)
	}

	lock, err := newAdvisoryLock(ctx, connection, &lockOwnerID, &id, &lockType)
	if err != nil {
		slog.ErrorContext(ctx, "Failed to create advisory lock", "error", err)
		return ctx, lockOwnerID, err
	}

	// obtain the advisory lock (blocking)
	err = lock.lock()
	if err != nil {
		slog.ErrorContext(ctx, "Failed to acquire advisory lock", "error", err)
		lock.g2.Rollback() // clean up the open transaction
		return ctx, lockOwnerID, err
	}

	locks.set(id, lockType, lock)

	ctx = context.WithValue(ctx, advisoryLock, locks)
	slog.InfoContext(ctx, "Acquired advisory lock", logger.FieldLockID, id, logger.FieldLockType, lockType)

	return ctx, lockOwnerID, nil
}

// Unlock searches current locks and unlocks the one matching its owner id.
func Unlock(ctx context.Context, callerUUID string) {
	locks, ok := ctx.Value(advisoryLock).(advisoryLockMap)
	if !ok {
		slog.ErrorContext(ctx, "Could not retrieve locks from context")
		return
	}

	for k, lock := range locks {
		if lock.ownerUUID == nil {
			slog.WarnContext(ctx, "lockOwnerID could not be found in AdvisoryLock", logger.FieldLockID, lock.id)
		} else if *lock.ownerUUID == callerUUID {
			lockID := "<missing>"
			lockType := LockType("<missing>")

			if lock.id != nil {
				lockID = *lock.id
			}
			if lock.lockType != nil {
				lockType = *lock.lockType
			}

			if err := lock.unlock(ctx); err != nil {
				slog.ErrorContext(ctx,
					"Could not unlock lock", logger.FieldLockID, lockID, logger.FieldLockType, lockType, "error", err)
				continue
			}
			slog.InfoContext(ctx, "Unlocked lock", logger.FieldLockID, lockID, logger.FieldLockType, lockType)
			delete(locks, k)
		}
		// Note: if ownerUUID doesn't match callerUUID, the lock belongs to a different
		// service call and is intentionally not unlocked here
	}
}

// recordTransactionError records transaction commit/rollback failures.
func recordTransactionError(operation, errorType string) {
	db_metrics.ErrorsMetric.With(prometheus.Labels{
		"operation":  operation,
		"error_type": errorType,
		"component":  "api",
		"version":    api.Version,
	}).Inc()
}
