package integration

import (
	"context"
	"flag"
	"os"
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
	if os.Getenv("HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER") == "" {
		_ = os.Setenv("HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER", `["validation","dns","pullsecret","hypershift"]`)
	}
	if os.Getenv("HYPERFLEET_ADAPTERS_REQUIRED_NODEPOOL") == "" {
		_ = os.Setenv("HYPERFLEET_ADAPTERS_REQUIRED_NODEPOOL", `["validation","hypershift"]`)
	}

	// JWT config - required for validation; integration tests use local test keys
	if os.Getenv("HYPERFLEET_SERVER_JWT_ISSUER_URL") == "" {
		_ = os.Setenv("HYPERFLEET_SERVER_JWT_ISSUER_URL", "https://test-idp.example.com/auth/realms/test")
	}
	if os.Getenv("HYPERFLEET_SERVER_JWK_CERT_URL") == "" {
		_ = os.Setenv("HYPERFLEET_SERVER_JWK_CERT_URL", "https://test-idp.example.com/certs")
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
