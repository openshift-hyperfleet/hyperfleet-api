package db_session

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	pkglogger "github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
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
	log := pkglogger.NewOCMLogger(ctx)

	log.Info("Starting PostgreSQL testcontainer...")

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
		log.Extra("error", err.Error()).Error("Failed to start PostgreSQL testcontainer")
		os.Exit(1)
	}

	f.container = container

	// Get connection string from container
	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Extra("error", err.Error()).Error("Failed to get connection string from testcontainer")
		os.Exit(1)
	}

	// Log sanitized connection info (without credentials)
	if parsedURL, parseErr := url.Parse(connStr); parseErr == nil {
		log.Extra("host", parsedURL.Host).Extra("database", config.Name).Info("PostgreSQL testcontainer started")
	} else {
		log.Info("PostgreSQL testcontainer started")
	}

	// Open SQL connection
	f.sqlDB, err = sql.Open("postgres", connStr)
	if err != nil {
		log.Extra("error", err.Error()).Error("Failed to connect to testcontainer database")
		os.Exit(1)
	}

	// Configure connection pool
	f.sqlDB.SetMaxOpenConns(config.MaxOpenConnections)

	// Connect GORM to use the same connection
	conf := &gorm.Config{
		PrepareStmt:            false,
		FullSaveAssociations:   false,
		SkipDefaultTransaction: true,
		Logger:                 logger.Default.LogMode(logger.Silent),
	}

	if config.Debug {
		conf.Logger = logger.Default.LogMode(logger.Info)
	}

	f.g2, err = gorm.Open(gormpostgres.New(gormpostgres.Config{
		Conn:                 f.sqlDB,
		PreferSimpleProtocol: true,
	}), conf)
	if err != nil {
		log.Extra("error", err.Error()).Error("Failed to connect GORM to testcontainer database")
		os.Exit(1)
	}

	// Run migrations
	log.Info("Running database migrations on testcontainer...")
	if err := db.Migrate(f.g2); err != nil {
		log.Extra("error", err.Error()).Error("Failed to run migrations on testcontainer")
		os.Exit(1)
	}

	log.Info("Testcontainer database initialized successfully")
}

func (f *Testcontainer) DirectDB() *sql.DB {
	return f.sqlDB
}

func (f *Testcontainer) New(ctx context.Context) *gorm.DB {
	conn := f.g2.Session(&gorm.Session{
		Context: ctx,
		Logger:  f.g2.Logger.LogMode(logger.Silent),
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
	log := pkglogger.NewOCMLogger(ctx)

	// Close SQL connection
	if f.sqlDB != nil {
		if err := f.sqlDB.Close(); err != nil {
			log.Extra("error", err.Error()).Error("Error closing SQL connection")
		}
	}

	// Terminate container
	if f.container != nil {
		log.Info("Stopping PostgreSQL testcontainer...")
		if err := f.container.Terminate(ctx); err != nil {
			return fmt.Errorf("failed to terminate testcontainer: %s", err)
		}
		log.Info("PostgreSQL testcontainer stopped")
	}

	return nil
}

func (f *Testcontainer) ResetDB() {
	// For testcontainers, we can just truncate all tables
	ctx := context.Background()
	log := pkglogger.NewOCMLogger(ctx)
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
				log.Extra("table", table).Extra("error", err.Error()).Error("Error truncating table")
			}
		}
	}
}

func (f *Testcontainer) NewListener(ctx context.Context, channel string, callback func(id string)) {
	log := pkglogger.NewOCMLogger(ctx)

	// Get the connection string for the listener
	connStr, err := f.container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		log.Extra("error", err.Error()).Error("Failed to get connection string for listener")
		return
	}

	newListener(ctx, connStr, channel, callback)
}
