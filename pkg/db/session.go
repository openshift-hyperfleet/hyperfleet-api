package db

import (
	"context"
	"database/sql"

	"gorm.io/gorm"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
)

type SessionFactory interface {
	Init(*config.DatabaseConfig)
	DirectDB() *sql.DB
	New(ctx context.Context) *gorm.DB
	CheckConnection() error
	Close() error
	ResetDB()
	NewListener(ctx context.Context, channel string, callback func(id string))
	GetAdvisoryLockTimeout() int
}

// TxRunner executes one callback inside a database transaction.
type TxRunner interface {
	// Do commits only after callback returns nil. Nested executions are rejected;
	// public mutation methods therefore remain the sole owners of their transaction.
	Do(ctx context.Context, callback func(context.Context) error) error
}
