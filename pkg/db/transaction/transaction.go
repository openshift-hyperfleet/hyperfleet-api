package transaction

import (
	"errors"

	"gorm.io/gorm"
)

// By default do no roll back transaction.
// only perform rollback if explicitly set by g2.g2.MarkForRollback(ctx, err)
const defaultRollbackPolicy = false

// Transaction represents a database transaction.
// Contains GORM transaction to ensure all DAO operations participate in the transaction.
type Transaction struct {
	DB           *gorm.DB
	rollbackFlag bool
}

// BuildWithGORM creates a new transaction object with GORM transaction.
func BuildWithGORM(db *gorm.DB) *Transaction {
	return &Transaction{
		DB:           db,
		rollbackFlag: defaultRollbackPolicy,
	}
}

// MarkedForRollback returns true if a transaction is flagged for rollback and false otherwise.
func (tx *Transaction) MarkedForRollback() bool {
	return tx.rollbackFlag
}

func (tx *Transaction) Commit() error {
	if tx.DB == nil {
		return errors.New("db: transaction hasn't been started yet")
	}

	err := tx.DB.Commit().Error
	tx.DB = nil
	return err
}

// Rollback ends the transaction by rolling back
func (tx *Transaction) Rollback() error {
	if tx.DB == nil {
		return errors.New("db: transaction hasn't been started yet")
	}

	err := tx.DB.Rollback().Error
	tx.DB = nil
	return err
}

func (tx *Transaction) SetRollbackFlag(flag bool) {
	tx.rollbackFlag = flag
}
