package config

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
)

// SetMinimalTestEnv sets minimal required environment variables for testing
// This allows tests to focus on specific configuration aspects without
// needing to provide all required fields
//
// # Uses t.Setenv which automatically restores prior values when test completes
//
// IMPORTANT: Uses new configuration structure (host+port separated, not bind_address)
func SetMinimalTestEnv(t *testing.T) {
	// Server config - using new structure (host + port)
	t.Setenv("HYPERFLEET_SERVER_HOST", "localhost")
	t.Setenv("HYPERFLEET_SERVER_PORT", "8000")

	// Database config - minimal required
	t.Setenv("HYPERFLEET_DATABASE_HOST", "localhost")
	t.Setenv("HYPERFLEET_DATABASE_PORT", "5432")
	t.Setenv("HYPERFLEET_DATABASE_NAME", "test")
	t.Setenv("HYPERFLEET_DATABASE_USERNAME", "test")
	t.Setenv("HYPERFLEET_DATABASE_PASSWORD", "test")

	// Logging config - minimal required
	t.Setenv("HYPERFLEET_LOGGING_LEVEL", "info")

	// OCM config - minimal required
	t.Setenv("HYPERFLEET_OCM_BASE_URL", "https://api.example.com")

	// Metrics config - using new structure (host + port)
	t.Setenv("HYPERFLEET_METRICS_HOST", "localhost")
	t.Setenv("HYPERFLEET_METRICS_PORT", "9090")

	// Health config - using new structure (host + port)
	t.Setenv("HYPERFLEET_HEALTH_HOST", "localhost")
	t.Setenv("HYPERFLEET_HEALTH_PORT", "8080")

	// Adapters config - empty arrays are valid
	t.Setenv("HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER", `[]`)
	t.Setenv("HYPERFLEET_ADAPTERS_REQUIRED_NODEPOOL", `[]`)
}

// LoadTestConfig loads a minimal valid configuration for testing
func LoadTestConfig(t *testing.T) (*ApplicationConfig, error) {
	SetMinimalTestEnv(t)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	return loader.Load(ctx, cmd)
}
