package db_session

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/lib/pq"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_metrics"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/internal/txcontext"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

const slowQueryThreshold = 200 * time.Millisecond

type Default struct {
	config *config.DatabaseConfig

	g2 *gorm.DB
	// Direct database connection.
	// It is used:
	// - to setup/close connection because GORM V2 removed gorm.Close()
	// - to work with pq.CopyIn because connection returned by GORM V2 gorm.DB() in "not the same"
	db *sql.DB
}

var _ db.SessionFactory = &Default{}

func NewProdFactory(config *config.DatabaseConfig) *Default {
	conn := &Default{}
	conn.Init(config)
	return conn
}

// Init will initialize a singleton connection as needed and return the same instance.
// Go includes database connection pooling in the platform. Gorm uses the same and provides a method to
// clone a connection via New(), which is safe for use by concurrent Goroutines.
func (f *Default) Init(config *config.DatabaseConfig) {
	// Only the first time
	once.Do(func() {
		var (
			dbx *sql.DB
			g2  *gorm.DB
			err error
		)

		// Open connection to DB via standard library
		dbx, err = sql.Open(config.Dialect, config.ConnectionString(config.SSL.Mode != disable))
		if err != nil {
			dbx, err = sql.Open(config.Dialect, config.ConnectionString(false))
			if err != nil {
				panic(fmt.Sprintf(
					"SQL failed to connect to %s database %s with connection string: %s\nError: %s",
					config.Dialect,
					config.Name,
					config.LogSafeConnectionString(config.SSL.Mode != disable),
					err.Error(),
				))
			}
		}
		applyPoolSettings(dbx, config)

		var gormLog gormlogger.Interface
		if config.Debug {
			gormLog = logger.NewGormLogger(gormlogger.Info, slowQueryThreshold)
		} else {
			gormLog = logger.NewGormLogger(gormlogger.Warn, slowQueryThreshold)
		}

		conf := &gorm.Config{
			PrepareStmt:          false,
			FullSaveAssociations: false,
			Logger:               gormLog,
		}
		pgConfig := postgres.Config{
			Conn: dbx,
			// Disable implicit prepared statement usage (GORM V2 uses pgx as database/sql driver and it enables prepared
			/// statement cache by default)
			// In migrations we both change tables' structure and running SQLs to modify data.
			// This way all prepared statements becomes invalid.
			PreferSimpleProtocol: true,
		}

		// Retry GORM connection to handle sidecar startup races (e.g. pgbouncer)
		for attempt := 0; ; attempt++ {
			g2, err = gorm.Open(postgres.New(pgConfig), conf)
			if err == nil {
				break
			}
			if attempt >= config.Pool.ConnRetryAttempts {
				panic(fmt.Sprintf(
					"GORM failed to connect to %s database %s with connection string: %s\nError: %s",
					config.Dialect,
					config.Name,
					config.LogSafeConnectionString(config.SSL.Mode != disable),
					err.Error(),
				))
			}
			slog.WarnContext(context.Background(),
				"Database connection failed, retrying...", "retry", attempt+1,
				"max_retries", config.Pool.ConnRetryAttempts,
				"retry_interval", config.Pool.ConnRetryInterval, "error", err)
			time.Sleep(config.Pool.ConnRetryInterval)

			// Close the existing handle before re-opening to avoid leaking connections
			if dbx != nil {
				_ = dbx.Close()
			}
			// Re-open sql.DB for the next attempt since the previous handle was closed above
			dbx, err = sql.Open(config.Dialect, config.ConnectionString(config.SSL.Mode != disable))
			if err != nil {
				dbx, err = sql.Open(config.Dialect, config.ConnectionString(false))
				if err != nil {
					panic(fmt.Sprintf(
						"SQL failed to reconnect to %s database %s: %s",
						config.Dialect, config.Name, err.Error(),
					))
				}
			}
			applyPoolSettings(dbx, config)
			pgConfig.Conn = dbx
		}

		// Register database metrics GORM plugin
		if err = db_metrics.RegisterPlugin(g2); err != nil {
			slog.WarnContext(context.Background(), "Failed to register database metrics plugin", "error", err)
		}

		// Register connection pool metrics collector
		if err = db_metrics.RegisterPoolCollector(dbx); err != nil {
			slog.WarnContext(context.Background(), "Failed to register pool metrics collector", "error", err)
		}

		f.config = config
		f.g2 = g2
		f.db = dbx
	})
}

// applyPoolSettings configures database connection pool settings
// Uses configuration from HYPERFLEET-694 for connection lifecycle management
func applyPoolSettings(db *sql.DB, cfg *config.DatabaseConfig) {
	db.SetMaxOpenConns(cfg.Pool.MaxConnections)
	db.SetMaxIdleConns(cfg.Pool.MaxIdleConnections)
	db.SetConnMaxLifetime(cfg.Pool.ConnMaxLifetime)
	db.SetConnMaxIdleTime(cfg.Pool.ConnMaxIdleTime)
}

func (f *Default) DirectDB() *sql.DB {
	return f.db
}

func waitForNotification(ctx context.Context, l *pq.Listener, callback func(id string)) {
	for {
		select {
		case n := <-l.Notify:
			slog.InfoContext(ctx, "Received data from channel",
				logger.FieldChannel, n.Channel, logger.FieldData, n.Extra)
			callback(n.Extra)
			return
		case <-time.After(10 * time.Second):
			slog.DebugContext(ctx, "Received no events on channel during interval. Pinging source")
			go func() {
				if err := l.Ping(); err != nil {
					slog.DebugContext(ctx, "Ping failed", "error", err)
				}
			}()
			return
		}
	}
}

func newListener(ctx context.Context, connstr, channel string, callback func(id string)) {
	plog := func(ev pq.ListenerEventType, err error) {
		if err != nil {
			slog.ErrorContext(ctx, "PostgreSQL listener error", "error", err)
		}
	}
	listener := pq.NewListener(connstr, 10*time.Second, time.Minute, plog)
	err := listener.Listen(channel)
	if err != nil {
		panic(err)
	}

	slog.InfoContext(ctx, "Starting channeling monitor", logger.FieldChannel, channel)
	for {
		waitForNotification(ctx, listener, callback)
	}
}

func (f *Default) NewListener(ctx context.Context, channel string, callback func(id string)) {
	newListener(ctx, f.config.ConnectionString(true), channel, callback)
}

// New returns a GORM DB session.
// If a transaction exists in context, returns that transaction so DAO operations participate.
// Otherwise returns a non-transactional session.
func (f *Default) New(ctx context.Context) *gorm.DB {
	if tx, ok := txcontext.Session(ctx); ok {
		return tx
	}

	if f.g2 == nil {
		panic("SessionFactory not initialized - ensure Init() was called before serving requests")
	}

	return f.g2.Session(&gorm.Session{
		Context: ctx,
	})
}

func (f *Default) CheckConnection() error {
	return f.g2.Exec("SELECT 1").Error
}

// Close will close the connection to the database.
// THIS MUST **NOT** BE CALLED UNTIL THE SERVER/PROCESS IS EXITING!!
// This should only ever be called once for the entire duration of the application and only at the end.
func (f *Default) Close() error {
	return f.db.Close()
}

func (f *Default) ResetDB() {
	panic("ResetDB is not implemented for non-integration-test env")
}

// ReconfigureLogger changes the GORM logger level at runtime
func (f *Default) ReconfigureLogger(level gormlogger.LogLevel) {
	if f.g2 == nil {
		return
	}
	newLogger := logger.NewGormLogger(level, slowQueryThreshold)
	f.g2.Logger = newLogger
}

func (f *Default) GetAdvisoryLockTimeout() int {
	timeout := int(f.config.Pool.AdvisoryLockTimeout.Seconds())
	if timeout == 0 {
		return 300 // Default: 5 minutes if not configured
	}
	return timeout
}
