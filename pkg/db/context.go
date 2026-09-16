package db

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/internal/txcontext"
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
	if txcontext.Has(ctx) {
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
		logger.WithError(ctx, err).Error("Failed to create advisory lock")
		return ctx, lockOwnerID, err
	}

	// obtain the advisory lock (blocking)
	err = lock.lock()
	if err != nil {
		logger.WithError(ctx, err).Error("Failed to acquire advisory lock")
		lock.g2.Rollback() // clean up the open transaction
		return ctx, lockOwnerID, err
	}

	locks.set(id, lockType, lock)

	ctx = context.WithValue(ctx, advisoryLock, locks)
	logger.With(ctx, logger.FieldLockID, id, logger.FieldLockType, lockType).Info("Acquired advisory lock")

	return ctx, lockOwnerID, nil
}

// Unlock searches current locks and unlocks the one matching its owner id.
func Unlock(ctx context.Context, callerUUID string) {
	locks, ok := ctx.Value(advisoryLock).(advisoryLockMap)
	if !ok {
		logger.Error(ctx, "Could not retrieve locks from context")
		return
	}

	for k, lock := range locks {
		if lock.ownerUUID == nil {
			logger.With(ctx, logger.FieldLockID, lock.id).Warn("lockOwnerID could not be found in AdvisoryLock")
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
				logger.With(ctx, logger.FieldLockID, lockID, logger.FieldLockType, lockType).
					WithError(err).Error("Could not unlock lock")
				continue
			}
			logger.With(ctx, logger.FieldLockID, lockID, logger.FieldLockType, lockType).Info("Unlocked lock")
			delete(locks, k)
		}
		// Note: if ownerUUID doesn't match callerUUID, the lock belongs to a different
		// service call and is intentionally not unlocked here
	}
}
