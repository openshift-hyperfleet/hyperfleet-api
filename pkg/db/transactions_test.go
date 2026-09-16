package db

import (
	"context"
	"database/sql"
	"errors"
	"runtime"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	dto "github.com/prometheus/client_model/go"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_metrics"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/internal/txcontext"
)

type dbSessionFactory struct {
	gormDB *gorm.DB
	sqlDB  *sql.DB
}

func (f *dbSessionFactory) Init(*config.DatabaseConfig)                             {}
func (f *dbSessionFactory) New(_ context.Context) *gorm.DB                          { return f.gormDB }
func (f *dbSessionFactory) CheckConnection() error                                  { return nil }
func (f *dbSessionFactory) Close() error                                            { return nil }
func (f *dbSessionFactory) ResetDB()                                                {}
func (f *dbSessionFactory) NewListener(_ context.Context, _ string, _ func(string)) {}
func (f *dbSessionFactory) GetAdvisoryLockTimeout() int                             { return 300 }
func (f *dbSessionFactory) DirectDB() *sql.DB                                       { return f.sqlDB }

func newDBSessionFactory(t *testing.T, setupMock func(sqlmock.Sqlmock)) *dbSessionFactory {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to create sqlmock: %v", err)
	}
	setupMock(mock)
	mock.ExpectClose()

	gormDB, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		Logger: gormlogger.Discard,
	})
	if err != nil {
		t.Fatalf("failed to open GORM: %v", err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Errorf("close sqlmock database: %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet sqlmock expectations: %v", err)
		}
	})
	return &dbSessionFactory{gormDB: gormDB, sqlDB: sqlDB}
}

func TestTxRunnerBeginFailure(t *testing.T) {
	beginErr := errors.New("begin failed")
	runner := NewTxRunner(newDBSessionFactory(t, func(mock sqlmock.Sqlmock) {
		mock.ExpectBegin().WillReturnError(beginErr)
	}))

	called := false
	err := runner.Do(t.Context(), func(context.Context) error {
		called = true
		return nil
	})
	if !errors.Is(err, beginErr) {
		t.Fatalf("Do() error = %v, want %v", err, beginErr)
	}
	if got, want := err.Error(), "db: begin transaction: begin failed"; got != want {
		t.Fatalf("Do() error = %q, want %q", got, want)
	}
	if called {
		t.Fatal("callback ran after begin failed")
	}
}

func TestTxRunnerCommits(t *testing.T) {
	runner := NewTxRunner(newDBSessionFactory(t, func(mock sqlmock.Sqlmock) {
		mock.ExpectBegin()
		mock.ExpectCommit()
	}))
	if err := runner.Do(t.Context(), func(context.Context) error { return nil }); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
}

func TestTxRunnerCommitFailure(t *testing.T) {
	commitFailures := db_metrics.ErrorsMetric.WithLabelValues("commit", "commit_failed", "api", api.Version)
	var before dto.Metric
	if err := commitFailures.Write(&before); err != nil {
		t.Fatalf("read commit failure counter: %v", err)
	}

	commitErr := errors.New("commit failed")
	runner := NewTxRunner(newDBSessionFactory(t, func(mock sqlmock.Sqlmock) {
		mock.ExpectBegin()
		mock.ExpectCommit().WillReturnError(commitErr)
	}))
	if err := runner.Do(t.Context(), func(context.Context) error { return nil }); !errors.Is(err, commitErr) {
		t.Fatalf("Do() error = %v, want %v", err, commitErr)
	} else if got, want := err.Error(), "db: commit transaction: commit failed"; got != want {
		t.Fatalf("Do() error = %q, want %q", got, want)
	}

	var after dto.Metric
	if err := commitFailures.Write(&after); err != nil {
		t.Fatalf("read commit failure counter: %v", err)
	}
	if got, want := after.GetCounter().GetValue(), before.GetCounter().GetValue()+1; got != want {
		t.Fatalf("commit failure counter = %v, want %v", got, want)
	}
}

func TestTxRunnerCallbackErrorRollsBack(t *testing.T) {
	callbackErr := errors.New("callback failed")
	runner := NewTxRunner(newDBSessionFactory(t, func(mock sqlmock.Sqlmock) {
		mock.ExpectBegin()
		mock.ExpectRollback()
	}))
	if err := runner.Do(t.Context(), func(context.Context) error { return callbackErr }); !errors.Is(err, callbackErr) {
		t.Fatalf("Do() error = %v, want %v", err, callbackErr)
	} else if got, want := err.Error(), "db: transaction callback: callback failed"; got != want {
		t.Fatalf("Do() error = %q, want %q", got, want)
	}
}

func TestTxRunnerPreservesCallbackErrorWhenRollbackFails(t *testing.T) {
	callbackErr := errors.New("callback failed")
	runner := NewTxRunner(newDBSessionFactory(t, func(mock sqlmock.Sqlmock) {
		mock.ExpectBegin()
		mock.ExpectRollback().WillReturnError(errors.New("rollback failed"))
	}))
	if err := runner.Do(t.Context(), func(context.Context) error { return callbackErr }); !errors.Is(err, callbackErr) {
		t.Fatalf("Do() error = %v, want %v", err, callbackErr)
	}
}

func TestTxRunnerPanicRollsBack(t *testing.T) {
	runner := NewTxRunner(newDBSessionFactory(t, func(mock sqlmock.Sqlmock) {
		mock.ExpectBegin()
		mock.ExpectRollback()
	}))
	defer func() {
		if recovered := recover(); recovered != "panic value" {
			t.Fatalf("panic = %v, want panic value", recovered)
		}
	}()
	_ = runner.Do(t.Context(), func(context.Context) error { panic("panic value") })
}

func TestTxRunnerPropagatesContextAndRejectsNestedExecution(t *testing.T) {
	type contextKey string
	parent := context.WithValue(t.Context(), contextKey("key"), "value")
	runner := NewTxRunner(newDBSessionFactory(t, func(mock sqlmock.Sqlmock) {
		mock.ExpectBegin()
		mock.ExpectRollback()
	}))

	innerCalled := false
	err := runner.Do(parent, func(txCtx context.Context) error {
		if txCtx.Value(contextKey("key")) != "value" {
			t.Fatal("transaction context did not inherit parent values")
		}
		if _, ok := txcontext.Session(txCtx); !ok {
			t.Fatal("transaction context has no participating session")
		}
		return runner.Do(txCtx, func(context.Context) error {
			innerCalled = true
			return nil
		})
	})
	if innerCalled {
		t.Fatal("nested callback ran")
	}
	if err == nil || err.Error() != "db: transaction callback: db: nested transactions are not supported" {
		t.Fatalf("nested transaction error = %v", err)
	}
}

func TestTxRunnerCancellationRollsBack(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	runner := NewTxRunner(newDBSessionFactory(t, func(mock sqlmock.Sqlmock) {
		mock.ExpectBegin()
		mock.ExpectRollback()
	}))
	err := runner.Do(ctx, func(txCtx context.Context) error {
		cancel()
		return txCtx.Err()
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Do() error = %v, want context.Canceled", err)
	}
}

func TestTxRunnerRetainedContextKeepsCompletedTransaction(t *testing.T) {
	runner := NewTxRunner(newDBSessionFactory(t, func(mock sqlmock.Sqlmock) {
		mock.ExpectBegin()
		mock.ExpectCommit()
	}))
	var completed context.Context
	if err := runner.Do(t.Context(), func(ctx context.Context) error {
		completed = ctx
		return nil
	}); err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	transaction, ok := txcontext.Session(completed)
	if !ok {
		t.Fatal("Session() did not retain the transaction session")
	}
	if err := transaction.Exec("SELECT 1").Error; !errors.Is(err, sql.ErrTxDone) {
		t.Fatalf("completed transaction error = %v, want %v", err, sql.ErrTxDone)
	}
}

func TestTxRunnerGoexitRollsBack(t *testing.T) {
	runner := NewTxRunner(newDBSessionFactory(t, func(mock sqlmock.Sqlmock) {
		mock.ExpectBegin()
		mock.ExpectRollback()
	}))

	var workers sync.WaitGroup
	workers.Go(func() {
		_ = runner.Do(t.Context(), func(context.Context) error {
			runtime.Goexit()
			return nil
		})
	})
	workers.Wait()
}
