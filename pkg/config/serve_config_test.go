package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestServeConfig_FullConfig tests that serve requires all configuration sections
func TestServeConfig_FullConfig(t *testing.T) {
	// Create full config file with all required sections
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configYAML := `
app:
  name: serve-test
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
	cfg := NewServeConfig()

	// Create viper and configure flags
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err = flags.Parse([]string{"--config=" + configFile})
	require.NoError(t, err)

	// Load config
	loadedCfg, err := LoadServeConfig(v, flags)
	require.NoError(t, err)

	// Verify all fields loaded
	assert.Equal(t, "serve-test", loadedCfg.App.Name)
	assert.Equal(t, 8000, loadedCfg.Server.Port)
	assert.Equal(t, 8080, loadedCfg.Metrics.Port)
	assert.Equal(t, 8083, loadedCfg.HealthCheck.Port)
	assert.Equal(t, "postgres", loadedCfg.Database.Dialect)
	assert.Equal(t, "https://api.integration.openshift.com", loadedCfg.OCM.BaseURL)
}

// TestServeConfig_Precedence tests command-line flags override environment variables
func TestServeConfig_Precedence(t *testing.T) {
	// Set environment variables
	os.Setenv("HYPERFLEET_SERVER_PORT", "9999")
	defer os.Unsetenv("HYPERFLEET_SERVER_PORT")

	// Create full config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configYAML := `
app:
  name: precedence-test
  version: 1.0.0
server:
  host: localhost
  port: 7000
metrics:
  host: localhost
  port: 8080
health_check:
  host: localhost
  port: 8083
database:
  dialect: postgres
ocm:
  base_url: https://api.integration.openshift.com
`
	err := os.WriteFile(configFile, []byte(configYAML), 0o644)
	require.NoError(t, err)

	// Create flag set with command-line value
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewServeConfig()

	// Create viper and configure flags
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err = flags.Parse([]string{
		"--config=" + configFile,
		"--server-port=8888",
	})
	require.NoError(t, err)

	// Load config
	loadedCfg, err := LoadServeConfig(v, flags)
	require.NoError(t, err)

	// CLI should win
	assert.Equal(t, 8888, loadedCfg.Server.Port, "CLI flag should override env and file")
}

// TestServeConfig_EnvVarOverridesFile tests environment variable precedence
func TestServeConfig_EnvVarOverridesFile(t *testing.T) {
	// Create full config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configYAML := `
app:
  name: file-name
  version: 1.0.0
server:
  host: localhost
  port: 8000
metrics:
  host: localhost
  port: 7070
health_check:
  host: localhost
  port: 8083
database:
  dialect: postgres
ocm:
  base_url: https://api.integration.openshift.com
`
	err := os.WriteFile(configFile, []byte(configYAML), 0o644)
	require.NoError(t, err)

	// Set environment variable
	os.Setenv("HYPERFLEET_METRICS_PORT", "9090")
	defer os.Unsetenv("HYPERFLEET_METRICS_PORT")

	// Create flag set
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewServeConfig()

	// Create viper and configure flags
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err = flags.Parse([]string{"--config=" + configFile})
	require.NoError(t, err)

	// Load config
	loadedCfg, err := LoadServeConfig(v, flags)
	require.NoError(t, err)

	// Env var should override file
	assert.Equal(t, 9090, loadedCfg.Metrics.Port, "Environment variable should override config file")
}

// TestServeConfig_ToApplicationConfig tests conversion to ApplicationConfig
func TestServeConfig_ToApplicationConfig(t *testing.T) {
	// Create serve config
	cfg := NewServeConfig()
	cfg.App.Name = "conversion-test"
	cfg.Server.Port = 8000
	cfg.Metrics.Port = 8080
	cfg.HealthCheck.Port = 8083
	cfg.Database.Dialect = "postgres"
	cfg.OCM.BaseURL = "https://api.test.com"

	// Convert to ApplicationConfig
	appConfig := cfg.ToApplicationConfig()

	// Verify conversion
	assert.Equal(t, "conversion-test", appConfig.App.Name)
	assert.Equal(t, 8000, appConfig.Server.Port)
	assert.Equal(t, 8080, appConfig.Metrics.Port)
	assert.Equal(t, 8083, appConfig.HealthCheck.Port)
	assert.Equal(t, "postgres", appConfig.Database.Dialect)
	assert.Equal(t, "https://api.test.com", appConfig.OCM.BaseURL)

	// Verify they share the same underlying config objects
	assert.Same(t, cfg.App, appConfig.App)
	assert.Same(t, cfg.Server, appConfig.Server)
	assert.Same(t, cfg.Metrics, appConfig.Metrics)
	assert.Same(t, cfg.HealthCheck, appConfig.HealthCheck)
	assert.Same(t, cfg.Database, appConfig.Database)
	assert.Same(t, cfg.OCM, appConfig.OCM)
}

// TestServeConfig_DisplayConfig tests that DisplayConfig doesn't panic
func TestServeConfig_DisplayConfig(t *testing.T) {
	// Create full config
	cfg := NewServeConfig()
	cfg.App.Name = "display-test"
	cfg.Server.Port = 8000
	cfg.Database.Password = "secret-password"
	cfg.OCM.ClientSecret = "secret-token"

	// Should not panic
	assert.NotPanics(t, func() {
		cfg.DisplayConfig()
	})
}

// TestServeConfig_GetJSONConfig tests JSON output with redaction
func TestServeConfig_GetJSONConfig(t *testing.T) {
	// Create full config with sensitive data
	cfg := NewServeConfig()
	cfg.App.Name = "json-test"
	cfg.Server.Port = 8000
	cfg.Database.Password = "super-secret-password"
	cfg.OCM.ClientSecret = "super-secret-token"

	// Get JSON
	jsonStr, err := cfg.GetJSONConfig()
	require.NoError(t, err)

	// Verify JSON contains app name but sensitive data is redacted
	assert.Contains(t, jsonStr, "json-test")
	assert.NotContains(t, jsonStr, "super-secret-password")
	assert.NotContains(t, jsonStr, "super-secret-token")
	assert.Contains(t, jsonStr, "***") // Redacted values
}

// TestServeConfig_SameFileAsMapping tests that same config file works for serve
func TestServeConfig_SameFileAsMigrate(t *testing.T) {
	// Create full config file that would be used by both commands
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configYAML := `
app:
  name: shared-config
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

	// Load as ServeConfig
	serveFlags := pflag.NewFlagSet("serve", pflag.ContinueOnError)
	serveCfg := NewServeConfig()
	serveViper := NewCommandConfig()
	serveCfg.ConfigureFlags(serveViper, serveFlags)
	err = serveFlags.Parse([]string{"--config=" + configFile})
	require.NoError(t, err)
	serveConfig, err := LoadServeConfig(serveViper, serveFlags)
	require.NoError(t, err)

	// Load as MigrateConfig
	migrateFlags := pflag.NewFlagSet("migrate", pflag.ContinueOnError)
	migrateCfg := NewMigrateConfig()
	migrateViper := NewCommandConfig()
	migrateCfg.ConfigureFlags(migrateViper, migrateFlags)
	err = migrateFlags.Parse([]string{"--config=" + configFile})
	require.NoError(t, err)
	migrateConfig, err := LoadMigrateConfig(migrateViper, migrateFlags)
	require.NoError(t, err)

	// Verify both loaded the same shared values (App and Database)
	assert.Equal(t, "shared-config", serveConfig.App.Name)
	assert.Equal(t, "shared-config", migrateConfig.App.Name)
	assert.Equal(t, "postgres", serveConfig.Database.Dialect)
	assert.Equal(t, "postgres", migrateConfig.Database.Dialect)

	// Serve has all sections loaded
	assert.NotNil(t, serveConfig.Server)
	assert.NotNil(t, serveConfig.Metrics)
	assert.NotNil(t, serveConfig.HealthCheck)
	assert.NotNil(t, serveConfig.OCM)

	// Migrate only has App and Database (other sections are ignored from config file)
	assert.NotNil(t, migrateConfig.App)
	assert.NotNil(t, migrateConfig.Database)
}
