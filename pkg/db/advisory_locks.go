package db

import (
	"context"
	"errors"
	"hash/fnv"
	"time"

	"gorm.io/gorm"
)

// LockType represents the type of advisory lock
type LockType string

const (
	// Migrations lock type for database migrations
	Migrations LockType = "Migrations"
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
	g2        *gorm.DB
	txid      int64
	ownerUUID *string
	id        *string
	lockType  *LockType
	startTime time.Time
}

// newAdvisoryLock constructs a new AdvisoryLock object.
func newAdvisoryLock(ctx context.Context, connection SessionFactory, ownerUUID *string, id *string, locktype *LockType) (*AdvisoryLock, error) {
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

	// current transaction ID set by postgres.  these are *not* distinct across time
	// and do get reset after postgres performs "vacuuming" to reclaim used IDs.
	var txid struct{ ID int64 }
	err := tx.Raw("select txid_current() as id").Scan(&txid).Error

	return &AdvisoryLock{
		txid:      txid.ID,
		ownerUUID: ownerUUID,
		id:        id,
		lockType:  locktype,
		g2:        tx,
		startTime: time.Now(),
	}, err
}

// lock calls select pg_advisory_xact_lock(id, lockType) to obtain the lock defined by (id, lockType).
// it is blocked if some other thread currently is holding the same lock (id, lockType).
// if blocked, it can be unblocked or timed out when overloaded.
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

	idAsInt := hash(*l.id)
	typeAsInt := hash(string(*l.lockType))
	err := l.g2.Exec("select pg_advisory_xact_lock(?, ?)", idAsInt, typeAsInt).Error
	return err
}

func (l *AdvisoryLock) unlock() error {
	if l.g2 == nil {
		return errors.New("AdvisoryLock: transaction is missing")
	}

	// it ends the Tx and implicitly releases the lock.
	err := l.g2.Commit().Error
	l.g2 = nil
	return err
}

// hash string to int32 (postgres integer)
// https://pkg.go.dev/math#pkg-constants
// https://www.postgresql.org/docs/12/datatype-numeric.html
func hash(s string) int32 {
	h := fnv.New32a()
	h.Write([]byte(s))
	// Sum32() returns uint32. needs conversion.
	return int32(h.Sum32())
}
