package db

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"gorm.io/gorm"
)

// LockType represents the type of advisory lock
type LockType string

const (
	// Migrations lock type for database migrations
	Migrations LockType = "Migrations"

	// MigrationsLockID is the advisory lock ID used for migration coordination
	MigrationsLockID = "migrations"
)

// AdvisoryLock represents a postgres advisory lock
//
//	begin                                       # start a Tx
//	select pg_advisory_xact_lock(id, lockType)  # obtain the lock (blocking)
//	end                                         # end the Tx and release the lock
//
// ownerUUID is a way to own the lock. Only the very first
// service call that owns the lock will have the correct ownerUUID. This is necessary
// to allow functions to call other service functions as part of the same lock (id, lockType).
type AdvisoryLock struct {
	g2             *gorm.DB
	ownerUUID      *string
	id             *string
	lockType       *LockType
	timeoutSeconds int
	startTime      time.Time
}

// newAdvisoryLock constructs a new AdvisoryLock object.
func newAdvisoryLock(
	ctx context.Context, connection SessionFactory, ownerUUID *string, id *string, locktype *LockType,
) (*AdvisoryLock, error) {
	if connection == nil {
		return nil, errors.New("AdvisoryLock: connection factory is missing")
	}

	// it requires a new DB session to start the advisory lock.
	g2 := connection.New(ctx)

	// start a Tx to ensure gorm will obtain/release the lock using a same connection.
	tx := g2.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	return &AdvisoryLock{
		ownerUUID:      ownerUUID,
		id:             id,
		lockType:       locktype,
		timeoutSeconds: connection.GetAdvisoryLockTimeout(),
		g2:             tx,
		startTime:      time.Now(),
	}, nil
}

// lock calls select pg_advisory_xact_lock(id, lockType) to obtain the lock defined by (id, lockType).
// It blocks until the lock is acquired or the statement timeout is reached.
// The timeout prevents indefinite blocking if a pod hangs while holding the lock.
func (l *AdvisoryLock) lock() error {
	if l.g2 == nil {
		return errors.New("AdvisoryLock: transaction is missing")
	}
	if l.id == nil {
		return errors.New("AdvisoryLock: id is missing")
	}
	if l.lockType == nil {
		return errors.New("AdvisoryLock: lockType is missing")
	}

	// Set statement timeout to prevent indefinite blocking.
	// This is transaction-scoped (SET LOCAL), so it only affects this lock acquisition.
	// Note: We cannot use parameter binding (?) for SET commands in PostgreSQL
	timeoutMs := l.timeoutSeconds * 1000
	if err := l.g2.Exec(fmt.Sprintf("SET LOCAL statement_timeout = %d", timeoutMs)).Error; err != nil {
		return err
	}

	idAsInt := hash(*l.id)
	typeAsInt := hash(string(*l.lockType))
	err := l.g2.Exec("select pg_advisory_xact_lock(?, ?)", idAsInt, typeAsInt).Error
	return err
}

func (l *AdvisoryLock) unlock(ctx context.Context) error {
	if l.g2 == nil {
		return errors.New("AdvisoryLock: transaction is missing")
	}

	duration := time.Since(l.startTime)

	// it ends the Tx and implicitly releases the lock.
	err := l.g2.Commit().Error
	l.g2 = nil

	if err == nil {
		logger.With(ctx, logger.FieldLockDurationMs, duration.Milliseconds()).Info("Released advisory lock")
	}

	return err
}

// hash string to int32 (postgres integer)
// https://pkg.go.dev/math#pkg-constants
// https://www.postgresql.org/docs/12/datatype-numeric.html
func hash(s string) int32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s)) // hash.Write never returns error
	// Sum32() returns uint32. needs conversion.
	return int32(h.Sum32())
}
