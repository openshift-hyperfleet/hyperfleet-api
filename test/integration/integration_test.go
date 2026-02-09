package integration

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

func TestMain(m *testing.M) {
	flag.Parse()
	ctx := context.Background()
	logger.With(ctx, "go_version", runtime.Version()).Info("Starting integration test")

	// Set adapter configuration for integration tests if not already set
	// These are required by the NodePool plugin
	if os.Getenv("HYPERFLEET_CLUSTER_ADAPTERS") == "" {
		_ = os.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", `["validation","dns","pullsecret","hypershift"]`)
	}
	if os.Getenv("HYPERFLEET_NODEPOOL_ADAPTERS") == "" {
		_ = os.Setenv("HYPERFLEET_NODEPOOL_ADAPTERS", `["validation","hypershift"]`)
	}

	// Set OPENAPI_SCHEMA_PATH for integration tests if not already set
	// This enables schema validation middleware during tests
	if os.Getenv("OPENAPI_SCHEMA_PATH") == "" {
		// Get the repo root directory (2 levels up from test/integration)
		// Use runtime.Caller to find this file's path
		_, filename, _, ok := runtime.Caller(0)
		if !ok {
			logger.Warn(ctx, "Failed to determine current file path via runtime.Caller, skipping OPENAPI_SCHEMA_PATH setup")
		} else {
			// filename is like: /path/to/repo/test/integration/integration_test.go
			// Navigate up: integration_test.go -> integration -> test -> repo
			integrationDir := filepath.Dir(filename) // /path/to/repo/test/integration
			testDir := filepath.Dir(integrationDir)  // /path/to/repo/test
			repoRoot := filepath.Dir(testDir)        // /path/to/repo

			// Build schema path using filepath.Join for cross-platform compatibility
			schemaPath := filepath.Join(repoRoot, "openapi", "openapi.yaml")

			// Verify the schema file exists before setting the env var
			if _, err := os.Stat(schemaPath); err != nil {
				logger.With(ctx, logger.FieldSchemaPath, schemaPath).WithError(err).
				Warn("Schema file not found, skipping OPENAPI_SCHEMA_PATH setup")
			} else {
				_ = os.Setenv("OPENAPI_SCHEMA_PATH", schemaPath)
				logger.With(ctx, logger.FieldSchemaPath, schemaPath).Info("Set OPENAPI_SCHEMA_PATH for integration tests")
			}
		}
	}

	helper := test.NewHelper(&testing.T{})
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
	os.Exit(exitCode)
}
