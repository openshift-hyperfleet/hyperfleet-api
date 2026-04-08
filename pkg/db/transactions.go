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
		_ = gormTx.Rollback() // Best-effort cleanup
		return nil, gormTx.Error
	}

	return transaction.BuildWithGORM(gormTx), nil
}
