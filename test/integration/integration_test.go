package integration

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/golang/glog"

	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

func TestMain(m *testing.M) {
	flag.Parse()
	glog.Infof("Starting integration test using go version %s", runtime.Version())

	// Set OPENAPI_SCHEMA_PATH for integration tests if not already set
	// This enables schema validation middleware during tests
	if os.Getenv("OPENAPI_SCHEMA_PATH") == "" {
		// Get the repo root directory (2 levels up from test/integration)
		// Use runtime.Caller to find this file's path
		_, filename, _, ok := runtime.Caller(0)
		if !ok {
			glog.Warningf("Failed to determine current file path via runtime.Caller, skipping OPENAPI_SCHEMA_PATH setup")
		} else {
			// filename is like: /path/to/repo/test/integration/integration_test.go
			// Navigate up: integration_test.go -> integration -> test -> repo
			integrationDir := filepath.Dir(filename)  // /path/to/repo/test/integration
			testDir := filepath.Dir(integrationDir)   // /path/to/repo/test
			repoRoot := filepath.Dir(testDir)         // /path/to/repo

			// Build schema path using filepath.Join for cross-platform compatibility
			schemaPath := filepath.Join(repoRoot, "openapi", "openapi.yaml")

			// Verify the schema file exists before setting the env var
			if _, err := os.Stat(schemaPath); err != nil {
				glog.Warningf("Schema file not found at %s: %v, skipping OPENAPI_SCHEMA_PATH setup", schemaPath, err)
			} else {
				os.Setenv("OPENAPI_SCHEMA_PATH", schemaPath)
				glog.Infof("Set OPENAPI_SCHEMA_PATH=%s for integration tests", schemaPath)
			}
		}
	}

	helper := test.NewHelper(&testing.T{})
	exitCode := m.Run()
	helper.Teardown()
	os.Exit(exitCode)
}
