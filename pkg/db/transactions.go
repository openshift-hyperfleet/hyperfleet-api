package db

import (
	"context"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/transaction"
)

// newTransaction constructs a new Transaction object with GORM transaction.
// Creates GORM transaction so that DAO operations participate in the transaction.
func newTransaction(ctx context.Context, connection SessionFactory) (*transaction.Transaction, error) {
	if connection == nil {
		// This happens in non-integration tests
		return nil, nil
	}

	// Get GORM session and start transaction
	g2 := connection.New(ctx)
	gormTx := g2.Begin()
	if gormTx.Error != nil {
		// Best-effort cleanup: safe no-op if transaction wasn't started.
		// Matches error handling pattern used later in this function (line 30).
		_ = gormTx.Rollback()
		return nil, gormTx.Error
	}

	// Get current transaction ID from PostgreSQL
	// Note: txid_current() is executed within the GORM transaction
	var txid int64
	err := gormTx.Raw("SELECT txid_current()").Scan(&txid).Error
	if err != nil {
		// Rollback on error to avoid leaking the transaction
		gormTx.Rollback()
		return nil, err
	}

	// Create transaction object with GORM transaction
	// The DB field is what DAOs will use via sessionFactory.New(ctx)
	return transaction.BuildWithGORM(gormTx, txid), nil
}
