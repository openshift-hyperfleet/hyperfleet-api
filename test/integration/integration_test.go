package integration

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

func TestMain(m *testing.M) {
	flag.Parse()
	ctx := context.Background()
	logger.With(ctx, "go_version", runtime.Version()).Info("Starting integration test")

	// Set OpenAPI schema path for integration tests if not already set
	// This enables schema validation middleware during tests
	// Uses HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH (config system standard)
	if os.Getenv("HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH") == "" {
		_, filename, _, ok := runtime.Caller(0)
		if !ok {
			logger.Warn(ctx, "Failed to determine current file path via runtime.Caller, skipping schema path setup")
		} else {
			integrationDir := filepath.Dir(filename)
			testDir := filepath.Dir(integrationDir)
			repoRoot := filepath.Dir(testDir)

			schemaPath := filepath.Join(repoRoot, "test", "validation-schema.yaml")

			if _, err := os.Stat(schemaPath); err != nil {
				logger.With(ctx, logger.FieldSchemaPath, schemaPath).WithError(err).
					Warn("Schema file not found, skipping schema path setup")
			} else {
				_ = os.Setenv("HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH", schemaPath)
				logger.With(ctx, logger.FieldSchemaPath, schemaPath).
					Info("Set HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH for integration tests")
			}
		}
	}

	pgContainer := startTestcontainer(ctx)

	helper := test.NewHelper()
	exitCode := m.Run()

	// Force exit if teardown hangs (e.g., due to a panic leaving resources in a bad state).
	// Without this, hung teardown blocks the process from exiting, causing
	// Prow CI jobs to stay in "pending" state indefinitely (HYPERFLEET-625).
	// 45s allows the testcontainer termination (30s timeout) to complete first.
	localExit := exitCode
	go func() {
		time.Sleep(45 * time.Second)
		logger.Error(ctx, "Teardown timed out after 45s, forcing exit")
		if localExit == 0 {
			localExit = 1
		}
		os.Exit(localExit)
	}()

	helper.Teardown()

	terminateContainer(ctx, pgContainer)
	os.Exit(exitCode)
}

func terminateContainer(ctx context.Context, pgContainer *postgres.PostgresContainer) {
	termCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := pgContainer.Terminate(termCtx); err != nil {
		logger.WithError(ctx, err).Error("Failed to terminate testcontainer")
	}
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func startTestcontainer(ctx context.Context) *postgres.PostgresContainer {
	dbName := envOrDefault("HYPERFLEET_DATABASE_NAME", "hyperfleet_test")
	dbUser := envOrDefault("HYPERFLEET_DATABASE_USERNAME", "test")
	dbPass := envOrDefault("HYPERFLEET_DATABASE_PASSWORD", "test")

	pgContainer, err := postgres.Run(ctx,
		"postgres:14.23",
		postgres.WithDatabase(dbName),
		postgres.WithUsername(dbUser),
		postgres.WithPassword(dbPass),
		testcontainers.WithWaitStrategy(
			wait.ForListeningPort("5432/tcp").
				WithStartupTimeout(60*time.Second)),
	)
	if err != nil {
		logger.WithError(ctx, err).Error("Failed to start PostgreSQL testcontainer")
		os.Exit(1)
	}

	host, err := pgContainer.Host(ctx)
	if err != nil {
		logger.WithError(ctx, err).Error("Failed to get testcontainer host")
		terminateContainer(ctx, pgContainer)
		os.Exit(1)
	}
	mappedPort, err := pgContainer.MappedPort(ctx, "5432/tcp")
	if err != nil {
		logger.WithError(ctx, err).Error("Failed to get testcontainer mapped port")
		terminateContainer(ctx, pgContainer)
		os.Exit(1)
	}

	os.Setenv("HYPERFLEET_DATABASE_HOST", host)
	os.Setenv("HYPERFLEET_DATABASE_PORT", mappedPort.Port())

	logger.With(ctx, "host", host, "port", mappedPort.Port()).Info("PostgreSQL testcontainer started")
	return pgContainer
}
