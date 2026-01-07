package db_session

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

type Testcontainer struct {
	config    *config.DatabaseConfig
	container *postgres.PostgresContainer
	g2        *gorm.DB
	sqlDB     *sql.DB
}

var _ db.SessionFactory = &Testcontainer{}

// NewTestcontainerFactory creates a SessionFactory using testcontainers.
// This starts a real PostgreSQL container for integration testing.
func NewTestcontainerFactory(config *config.DatabaseConfig) *Testcontainer {
	conn := &Testcontainer{
		config: config,
	}
	conn.Init(config)
	return conn
}

func (f *Testcontainer) Init(config *config.DatabaseConfig) {
	ctx := context.Background()

	logger.Info(ctx, "Starting PostgreSQL testcontainer...")

	// Create PostgreSQL container
	container, err := postgres.Run(ctx,
		"postgres:14.2",
		postgres.WithDatabase(config.Name),
		postgres.WithUsername(config.Username),
		postgres.WithPassword(config.Password),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").
				WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		logger.Errorf(ctx, "Failed to start PostgreSQL testcontainer: %s", err)
		os.Exit(1)
	}

	f.container = container

	// Get connection string from container
	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		logger.Errorf(ctx, "Failed to get connection string from testcontainer: %s", err)
		os.Exit(1)
	}

	logger.Infof(ctx, "PostgreSQL testcontainer started at: %s", connStr)

	// Open SQL connection
	f.sqlDB, err = sql.Open("postgres", connStr)
	if err != nil {
		logger.Errorf(ctx, "Failed to connect to testcontainer database: %s", err)
		os.Exit(1)
	}

	// Configure connection pool
	f.sqlDB.SetMaxOpenConns(config.MaxOpenConnections)

	// Connect GORM to use the same connection
	conf := &gorm.Config{
		PrepareStmt:            false,
		FullSaveAssociations:   false,
		SkipDefaultTransaction: true,
		Logger:                 gormlogger.Default.LogMode(gormlogger.Silent),
	}

	if config.Debug {
		conf.Logger = gormlogger.Default.LogMode(gormlogger.Info)
	}

	f.g2, err = gorm.Open(gormpostgres.New(gormpostgres.Config{
		Conn:                 f.sqlDB,
		PreferSimpleProtocol: true,
	}), conf)
	if err != nil {
		logger.Errorf(ctx, "Failed to connect GORM to testcontainer database: %s", err)
		os.Exit(1)
	}

	// Run migrations
	logger.Info(ctx, "Running database migrations on testcontainer...")
	if err := db.Migrate(f.g2); err != nil {
		logger.Errorf(ctx, "Failed to run migrations on testcontainer: %s", err)
		os.Exit(1)
	}

	logger.Info(ctx, "Testcontainer database initialized successfully")
}

func (f *Testcontainer) DirectDB() *sql.DB {
	return f.sqlDB
}

func (f *Testcontainer) New(ctx context.Context) *gorm.DB {
	conn := f.g2.Session(&gorm.Session{
		Context: ctx,
		Logger:  f.g2.Logger.LogMode(gormlogger.Silent),
	})
	if f.config.Debug {
		conn = conn.Debug()
	}
	return conn
}

func (f *Testcontainer) CheckConnection() error {
	_, err := f.sqlDB.Exec("SELECT 1")
	return err
}

func (f *Testcontainer) Close() error {
	ctx := context.Background()

	// Close SQL connection
	if f.sqlDB != nil {
		if err := f.sqlDB.Close(); err != nil {
			logger.Errorf(ctx, "Error closing SQL connection: %s", err)
		}
	}

	// Terminate container
	if f.container != nil {
		logger.Info(ctx, "Stopping PostgreSQL testcontainer...")
		if err := f.container.Terminate(ctx); err != nil {
			return fmt.Errorf("failed to terminate testcontainer: %s", err)
		}
		logger.Info(ctx, "PostgreSQL testcontainer stopped")
	}

	return nil
}

func (f *Testcontainer) ResetDB() {
	// For testcontainers, we can just truncate all tables
	ctx := context.Background()
	g2 := f.New(ctx)

	// Truncate all business tables in the correct order (respecting FK constraints)
	// Using CASCADE to handle foreign key constraints automatically
	tables := []string{
		"adapter_statuses", // Polymorphic table, no FK constraints
		"node_pools",       // Has FK to clusters
		"clusters",         // Referenced by node_pools
		"events",           // Independent table
	}
	for _, table := range tables {
		if g2.Migrator().HasTable(table) {
			if err := g2.Exec(fmt.Sprintf("TRUNCATE TABLE %s CASCADE", table)).Error; err != nil {
				logger.Error(ctx, "Error truncating table", "table", table, "error", err)
			}
		}
	}
}

func (f *Testcontainer) NewListener(ctx context.Context, channel string, callback func(id string)) {
	// Get the connection string for the listener
	connStr, err := f.container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		logger.Errorf(ctx, "Failed to get connection string for listener: %s", err)
		return
	}

	newListener(ctx, connStr, channel, callback)
}
