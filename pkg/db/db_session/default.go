package db_session

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/lib/pq"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_metrics"
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
		dbx, err = sql.Open(config.Dialect, config.ConnectionString(config.SSLMode != disable))
		if err != nil {
			dbx, err = sql.Open(config.Dialect, config.ConnectionString(false))
			if err != nil {
				panic(fmt.Sprintf(
					"SQL failed to connect to %s database %s with connection string: %s\nError: %s",
					config.Dialect,
					config.Name,
					config.LogSafeConnectionString(config.SSLMode != disable),
					err.Error(),
				))
			}
		}
		dbx.SetMaxOpenConns(config.MaxOpenConnections)

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
		g2, err = gorm.Open(postgres.New(postgres.Config{
			Conn: dbx,
			// Disable implicit prepared statement usage (GORM V2 uses pgx as database/sql driver and it enables prepared
			/// statement cache by default)
			// In migrations we both change tables' structure and running SQLs to modify data.
			// This way all prepared statements becomes invalid.
			PreferSimpleProtocol: true,
		}), conf)
		if err != nil {
			panic(fmt.Sprintf(
				"GORM failed to connect to %s database %s with connection string: %s\nError: %s",
				config.Dialect,
				config.Name,
				config.LogSafeConnectionString(config.SSLMode != disable),
				err.Error(),
			))
		}

		// Register database metrics GORM plugin
		if err = db_metrics.RegisterPlugin(g2); err != nil {
			logger.WithError(context.Background(), err).Warn("Failed to register database metrics plugin")
		}

		// Register connection pool metrics collector
		if err = db_metrics.RegisterPoolCollector(dbx); err != nil {
			logger.WithError(context.Background(), err).Warn("Failed to register pool metrics collector")
		}

		f.config = config
		f.g2 = g2
		f.db = dbx
	})
}

func (f *Default) DirectDB() *sql.DB {
	return f.db
}

func waitForNotification(l *pq.Listener, callback func(id string)) {
	ctx := context.Background()
	for {
		select {
		case n := <-l.Notify:
			logger.With(ctx, logger.FieldChannel, n.Channel).With(logger.FieldData, n.Extra).Info("Received data from channel")
			callback(n.Extra)
			return
		case <-time.After(10 * time.Second):
			logger.Debug(ctx, "Received no events on channel during interval. Pinging source")
			go func() {
				if err := l.Ping(); err != nil {
					logger.WithError(ctx, err).Debug("Ping failed")
				}
			}()
			return
		}
	}
}

func newListener(ctx context.Context, connstr, channel string, callback func(id string)) {
	plog := func(ev pq.ListenerEventType, err error) {
		if err != nil {
			logger.WithError(ctx, err).Error("PostgreSQL listener error")
		}
	}
	listener := pq.NewListener(connstr, 10*time.Second, time.Minute, plog)
	err := listener.Listen(channel)
	if err != nil {
		panic(err)
	}

	logger.With(ctx, logger.FieldChannel, channel).Info("Starting channeling monitor")
	for {
		waitForNotification(listener, callback)
	}
}

func (f *Default) NewListener(ctx context.Context, channel string, callback func(id string)) {
	newListener(ctx, f.config.ConnectionString(true), channel, callback)
}

func (f *Default) New(ctx context.Context) *gorm.DB {
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
