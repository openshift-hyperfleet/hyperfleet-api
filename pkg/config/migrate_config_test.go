package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMigrateConfig_MinimalRequiredConfig tests that migrate only requires App and Database
func TestMigrateConfig_MinimalRequiredConfig(t *testing.T) {
	// Create minimal config file with only required sections
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configYAML := `
app:
  name: migrate-test
  version: 1.0.0
database:
  dialect: postgres
  host: localhost
  port: 5432
  name: hyperfleet
  username: hyperfleet
  password: secret
`
	err := os.WriteFile(configFile, []byte(configYAML), 0o644)
	require.NoError(t, err)

	// Create flag set
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewMigrateConfig()

	// Create viper and configure flags
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err = flags.Parse([]string{"--config=" + configFile})
	require.NoError(t, err)

	// Load config
	loadedCfg, err := LoadMigrateConfig(v, flags)
	require.NoError(t, err)

	// Verify required fields loaded
	assert.Equal(t, "migrate-test", loadedCfg.App.Name)
	assert.Equal(t, "postgres", loadedCfg.Database.Dialect)
	assert.Equal(t, "localhost", loadedCfg.Database.Host)
}

// TestMigrateConfig_IgnoresExtraConfigSections tests that migrate ignores extra config sections
// This allows using shared config files with serve (backward compatibility)
func TestMigrateConfig_IgnoresExtraConfigSections(t *testing.T) {
	// Create full config file (as used by serve command)
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configYAML := `
app:
  name: full-config-test
  version: 1.0.0
server:
  host: localhost
  port: 8000
metrics:
  host: localhost
  port: 8080
health_check:
  host: localhost
  port: 8083
database:
  dialect: postgres
  host: localhost
  port: 5432
  name: hyperfleet
  username: hyperfleet
  password: secret
ocm:
  base_url: https://api.integration.openshift.com
`
	err := os.WriteFile(configFile, []byte(configYAML), 0o644)
	require.NoError(t, err)

	// Create flag set
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewMigrateConfig()

	// Create viper and configure flags
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err = flags.Parse([]string{"--config=" + configFile})
	require.NoError(t, err)

	// Load config - should not fail even with extra sections (they're ignored)
	loadedCfg, err := LoadMigrateConfig(v, flags)
	require.NoError(t, err)

	// Verify required fields loaded correctly
	assert.Equal(t, "full-config-test", loadedCfg.App.Name)
	assert.Equal(t, "postgres", loadedCfg.Database.Dialect)
	assert.Equal(t, "localhost", loadedCfg.Database.Host)

	// Extra sections (server, metrics, health_check, ocm) are ignored
	// MigrateConfig only has App and Database fields
}

// TestMigrateConfig_Precedence tests command-line flags override environment variables
func TestMigrateConfig_Precedence(t *testing.T) {
	// Set environment variable
	os.Setenv("HYPERFLEET_DATABASE_HOST", "env-host")
	defer os.Unsetenv("HYPERFLEET_DATABASE_HOST")

	// Create minimal config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configYAML := `
app:
  name: precedence-test
  version: 1.0.0
database:
  dialect: postgres
  host: file-host
  port: 5432
  name: hyperfleet
  username: hyperfleet
  password: secret
`
	err := os.WriteFile(configFile, []byte(configYAML), 0o644)
	require.NoError(t, err)

	// Create flag set with command-line value
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewMigrateConfig()

	// Create viper and configure flags
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err = flags.Parse([]string{
		"--config=" + configFile,
		"--db-host=cli-host",
	})
	require.NoError(t, err)

	// Load config
	loadedCfg, err := LoadMigrateConfig(v, flags)
	require.NoError(t, err)

	// CLI should win
	assert.Equal(t, "cli-host", loadedCfg.Database.Host, "CLI flag should override env and file")
}

// TestMigrateConfig_EnvVarOverridesFile tests environment variable precedence
func TestMigrateConfig_EnvVarOverridesFile(t *testing.T) {
	// Create minimal config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configYAML := `
app:
  name: file-name
  version: 1.0.0
database:
  dialect: postgres
  host: localhost
  port: 5432
  name: hyperfleet
  username: hyperfleet
  password: file-password
`
	err := os.WriteFile(configFile, []byte(configYAML), 0o644)
	require.NoError(t, err)

	// Set environment variable
	os.Setenv("HYPERFLEET_DATABASE_PASSWORD", "env-password")
	defer os.Unsetenv("HYPERFLEET_DATABASE_PASSWORD")

	// Create flag set
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewMigrateConfig()

	// Create viper and configure flags
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err = flags.Parse([]string{"--config=" + configFile})
	require.NoError(t, err)

	// Load config
	loadedCfg, err := LoadMigrateConfig(v, flags)
	require.NoError(t, err)

	// Env var should override file
	assert.Equal(t, "env-password", loadedCfg.Database.Password, "Environment variable should override config file")
}

// TestMigrateConfig_ValidatesRequiredDatabaseDialect tests that database dialect is validated
func TestMigrateConfig_ValidatesRequiredDatabaseDialect(t *testing.T) {
	// Create config file with database section but missing dialect
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configYAML := `
app:
  name: incomplete-test
  version: 1.0.0
database:
  dialect: ""
  host: localhost
  port: 5432
`
	err := os.WriteFile(configFile, []byte(configYAML), 0o644)
	require.NoError(t, err)

	// Create flag set
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewMigrateConfig()

	// Create viper and configure flags
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err = flags.Parse([]string{"--config=" + configFile})
	require.NoError(t, err)

	// Load config - should fail validation due to empty dialect
	_, err = LoadMigrateConfig(v, flags)
	require.Error(t, err, "Should fail when database dialect is empty")
	assert.Contains(t, err.Error(), "Dialect")
}

// TestMigrateConfig_FlagsOnly tests loading from flags without config file
func TestMigrateConfig_FlagsOnly(t *testing.T) {
	// Create flag set with all required flags
	// Note: dialect doesn't have a flag, it uses the default value "postgres"
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewMigrateConfig()

	// Create viper and configure flags
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err := flags.Parse([]string{
		"--name=flags-only-test",
		"--version=2.0.0",
		"--db-host=localhost",
		"--db-port=5432",
		"--db-name=hyperfleet",
		"--db-username=admin",
		"--db-password=secret123",
	})
	require.NoError(t, err)

	// Load config
	loadedCfg, err := LoadMigrateConfig(v, flags)
	require.NoError(t, err)

	// Verify config loaded from flags
	assert.Equal(t, "flags-only-test", loadedCfg.App.Name)
	assert.Equal(t, "postgres", loadedCfg.Database.Dialect) // Uses default value
	assert.Equal(t, "localhost", loadedCfg.Database.Host)
	assert.Equal(t, "admin", loadedCfg.Database.Username)
}

// TestMigrateConfig_DisplayConfig tests that DisplayConfig doesn't panic
func TestMigrateConfig_DisplayConfig(t *testing.T) {
	// Create minimal config
	cfg := NewMigrateConfig()
	cfg.App.Name = "display-test"
	cfg.Database.Dialect = "postgres"
	cfg.Database.Password = "secret-password"

	// Should not panic
	assert.NotPanics(t, func() {
		cfg.DisplayConfig()
	})
}

// TestMigrateConfig_GetJSONConfig tests JSON output with redaction
func TestMigrateConfig_GetJSONConfig(t *testing.T) {
	// Create minimal config with sensitive data
	cfg := NewMigrateConfig()
	cfg.App.Name = "json-test"
	cfg.Database.Dialect = "postgres"
	cfg.Database.Password = "super-secret-password"

	// Get JSON
	jsonStr, err := cfg.GetJSONConfig()
	require.NoError(t, err)

	// Verify JSON contains app name but password is redacted
	assert.Contains(t, jsonStr, "json-test")
	assert.NotContains(t, jsonStr, "super-secret-password")
	assert.Contains(t, jsonStr, "***") // Redacted password
}
