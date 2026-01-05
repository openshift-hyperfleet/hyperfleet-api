package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testLoadConfig is a helper that loads config (flags must already be configured and parsed)
func testLoadConfig(v *viper.Viper, flags *pflag.FlagSet) (*ApplicationConfig, error) {
	return LoadConfig(v, flags)
}

// TestConfigPrecedence_CommandLineOverridesEnvVar tests that command-line flags
// have higher precedence than environment variables
func TestConfigPrecedence_CommandLineOverridesEnvVar(t *testing.T) {
	// Set environment variable
	os.Setenv("HYPERFLEET_APP_NAME", "env-name")
	defer os.Unsetenv("HYPERFLEET_APP_NAME")

	// Create flag set
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewApplicationConfig()

	// Configure flags (define and bind)
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	// Parse command-line flag (simulating --name=cli-name)
	err := flags.Parse([]string{"--name=cli-name"})
	require.NoError(t, err)

	// Load config
	loadedCfg, err := testLoadConfig(v, flags)
	require.NoError(t, err)

	// Command-line flag should win
	assert.Equal(t, "cli-name", loadedCfg.App.Name, "Command-line flag should override environment variable")
}

// TestConfigPrecedence_EnvVarOverridesConfigFile tests that environment variables
// have higher precedence than config file values
func TestConfigPrecedence_EnvVarOverridesConfigFile(t *testing.T) {
	// Create temporary config file
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

	// Set environment variable
	os.Setenv("HYPERFLEET_APP_NAME", "env-name")
	defer os.Unsetenv("HYPERFLEET_APP_NAME")

	// Create flag set and specify config file
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewApplicationConfig()

	// Configure flags (define and bind)
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err = flags.Parse([]string{"--config=" + configFile})
	require.NoError(t, err)

	// Load config
	loadedCfg, err := testLoadConfig(v, flags)
	require.NoError(t, err)

	// Environment variable should win over config file
	assert.Equal(t, "env-name", loadedCfg.App.Name, "Environment variable should override config file")
}

// TestConfigPrecedence_ConfigFileOverridesDefaults tests that config file values
// have higher precedence than default values
func TestConfigPrecedence_ConfigFileOverridesDefaults(t *testing.T) {
	// Create temporary config file with custom port
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configYAML := `
app:
  name: file-name
  version: 2.0.0
server:
  host: localhost
  port: 9999
  auth:
    jwt:
      enabled: false
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

	// Create flag set and specify config file
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewApplicationConfig()

	// Configure flags (define and bind)
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err = flags.Parse([]string{"--config=" + configFile})
	require.NoError(t, err)

	// Load config
	loadedCfg, err := testLoadConfig(v, flags)
	require.NoError(t, err)

	// Config file values should override defaults
	assert.Equal(t, "file-name", loadedCfg.App.Name, "Config file should override default app name")
	assert.Equal(t, "2.0.0", loadedCfg.App.Version, "Config file should override default version")
	assert.Equal(t, 9999, loadedCfg.Server.Port, "Config file should override default port")
	assert.Equal(t, false, loadedCfg.Server.Auth.JWT.Enabled, "Config file should override default auth")
}

// TestConfigPrecedence_FullPrecedenceChain tests the complete precedence chain:
// CLI > Env Var > Config File > Defaults
func TestConfigPrecedence_FullPrecedenceChain(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configYAML := `
app:
  name: file-name
  version: 2.0.0
server:
  host: file-host
  port: 7000
metrics:
  host: localhost
  port: 7070
health_check:
  host: localhost
  port: 8083
database:
  dialect: postgres
  port: 5432
ocm:
  base_url: https://api.integration.openshift.com
`
	err := os.WriteFile(configFile, []byte(configYAML), 0o644)
	require.NoError(t, err)

	// Set environment variables
	os.Setenv("HYPERFLEET_APP_VERSION", "env-version")
	os.Setenv("HYPERFLEET_SERVER_PORT", "8888")
	defer os.Unsetenv("HYPERFLEET_APP_VERSION")
	defer os.Unsetenv("HYPERFLEET_SERVER_PORT")

	// Create flag set with command-line values
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewApplicationConfig()

	// Configure flags (define and bind)
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err = flags.Parse([]string{
		"--config=" + configFile,
		"--name=cli-name", // CLI overrides all
		// version: env var should override file
		// server-host: file should override default
		// server-port: env var should override file
		"--metrics-port=9090", // CLI overrides all
	})
	require.NoError(t, err)

	// Load config
	loadedCfg, err := testLoadConfig(v, flags)
	require.NoError(t, err)

	// Verify precedence
	assert.Equal(t, "cli-name", loadedCfg.App.Name, "CLI should have highest precedence")
	assert.Equal(t, "env-version", loadedCfg.App.Version, "Env var should override config file")
	assert.Equal(t, "file-host", loadedCfg.Server.Host, "Config file should override default")
	assert.Equal(t, 8888, loadedCfg.Server.Port, "Env var should override config file")
	assert.Equal(t, 9090, loadedCfg.Metrics.Port, "CLI should override all")
}

// TestConfigFile_SpecifiedByFlag tests that config file can be specified via --config flag
func TestConfigFile_SpecifiedByFlag(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "custom-config.yaml")

	configYAML := `
app:
  name: custom-app
  version: 3.0.0
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
ocm:
  base_url: https://api.integration.openshift.com
`
	err := os.WriteFile(configFile, []byte(configYAML), 0o644)
	require.NoError(t, err)

	// Create flag set with --config flag
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewApplicationConfig()

	// Configure flags (define and bind)
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err = flags.Parse([]string{"--config=" + configFile})
	require.NoError(t, err)

	// Load config
	loadedCfg, err := testLoadConfig(v, flags)
	require.NoError(t, err)

	// Verify config was loaded from the specified file
	assert.Equal(t, "custom-app", loadedCfg.App.Name)
	assert.Equal(t, "3.0.0", loadedCfg.App.Version)
}

// TestConfigFile_SpecifiedByEnvVar tests that config file can be specified via HYPERFLEET_CONFIG env var
func TestConfigFile_SpecifiedByEnvVar(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "env-config.yaml")

	configYAML := `
app:
  name: env-config-app
  version: 4.0.0
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
ocm:
  base_url: https://api.integration.openshift.com
`
	err := os.WriteFile(configFile, []byte(configYAML), 0o644)
	require.NoError(t, err)

	// Set HYPERFLEET_CONFIG environment variable
	os.Setenv("HYPERFLEET_CONFIG", configFile)
	defer os.Unsetenv("HYPERFLEET_CONFIG")

	// Create flag set without --config flag
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewApplicationConfig()

	// Configure flags (define and bind)
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err = flags.Parse([]string{})
	require.NoError(t, err)

	// Load config
	loadedCfg, err := testLoadConfig(v, flags)
	require.NoError(t, err)

	// Verify config was loaded from the env-specified file
	assert.Equal(t, "env-config-app", loadedCfg.App.Name)
	assert.Equal(t, "4.0.0", loadedCfg.App.Version)
}

// TestConfigFile_FlagOverridesEnvVar tests that --config flag takes precedence over HYPERFLEET_CONFIG env var
func TestConfigFile_FlagOverridesEnvVar(t *testing.T) {
	tmpDir := t.TempDir()

	// Create config file for env var
	envConfigFile := filepath.Join(tmpDir, "env-config.yaml")
	envConfigYAML := `
app:
  name: env-config
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
ocm:
  base_url: https://api.integration.openshift.com
`
	err := os.WriteFile(envConfigFile, []byte(envConfigYAML), 0o644)
	require.NoError(t, err)

	// Create config file for flag
	flagConfigFile := filepath.Join(tmpDir, "flag-config.yaml")
	flagConfigYAML := `
app:
  name: flag-config
  version: 2.0.0
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
ocm:
  base_url: https://api.integration.openshift.com
`
	err = os.WriteFile(flagConfigFile, []byte(flagConfigYAML), 0o644)
	require.NoError(t, err)

	// Set env var to one file
	os.Setenv("HYPERFLEET_CONFIG", envConfigFile)
	defer os.Unsetenv("HYPERFLEET_CONFIG")

	// Create flag set with different config file
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewApplicationConfig()

	// Configure flags (define and bind)
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err = flags.Parse([]string{"--config=" + flagConfigFile})
	require.NoError(t, err)

	// Load config
	loadedCfg, err := testLoadConfig(v, flags)
	require.NoError(t, err)

	// Flag should win over env var
	assert.Equal(t, "flag-config", loadedCfg.App.Name, "--config flag should override HYPERFLEET_CONFIG env var")
}

// TestConfigPrecedence_DatabasePassword tests password precedence specifically
func TestConfigPrecedence_DatabasePassword(t *testing.T) {
	// Create temporary config file
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, "config.yaml")

	configYAML := `
app:
  name: test
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
  password: file-password
ocm:
  base_url: https://api.integration.openshift.com
`
	err := os.WriteFile(configFile, []byte(configYAML), 0o644)
	require.NoError(t, err)

	// Set environment variable
	os.Setenv("HYPERFLEET_DATABASE_PASSWORD", "env-password")
	defer os.Unsetenv("HYPERFLEET_DATABASE_PASSWORD")

	// Create flag set
	flags := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg := NewApplicationConfig()

	// Configure flags (define and bind)
	v := NewCommandConfig()
	cfg.ConfigureFlags(v, flags)

	err = flags.Parse([]string{
		"--config=" + configFile,
		"--db-password=cli-password",
	})
	require.NoError(t, err)

	// Load config
	loadedCfg, err := testLoadConfig(v, flags)
	require.NoError(t, err)

	// CLI password should win
	assert.Equal(t, "cli-password", loadedCfg.Database.Password, "CLI password should override env and file")
}
