package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

// ==============================================================
// Configuration File Resolution Tests
// ==============================================================

// TestConfigLoader_ExplicitConfigFlag tests loading from config file
func TestConfigLoader_ExplicitConfigFlag(t *testing.T) {
	RegisterTestingT(t)

	// Create temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test-config.yaml")
	configContent := `
server:
  host: "config-file-host"
  port: 9999
database:
  host: "localhost"
  port: 5432
  name: "testdb"
  username: "testuser"
  password: "testpass"
logging:
  level: "debug"
ocm:
  base_url: "https://config.example.com"
metrics:
  host: "localhost"
  port: 9090
health:
  host: "localhost"
  port: 8080
adapters:
  required:
    cluster: []
    nodepool: []
`
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	Expect(err).NotTo(HaveOccurred())

	// Set config path via environment variable (not flag, to avoid BindPFlags issue)
	t.Setenv("HYPERFLEET_CONFIG", configPath)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	cfg, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())
	Expect(cfg.Server.Host).To(Equal("config-file-host"),
		"Config file value should be loaded")
	Expect(cfg.Server.Port).To(Equal(9999))
	Expect(cfg.Logging.Level).To(Equal("debug"))
}

// TestConfigLoader_ConfigFileNotFound tests error when explicit config is missing
func TestConfigLoader_ConfigFileNotFound(t *testing.T) {
	RegisterTestingT(t)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "config file path")
	_ = cmd.Flags().Set("config", "/nonexistent/config.yaml")
	ctx := context.Background()

	_, err := loader.Load(ctx, cmd)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("explicitly specified config file not found"))
}

// TestConfigLoader_NoConfigFile tests loading with only env vars
func TestConfigLoader_NoConfigFile(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	cfg, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())
	Expect(cfg.Server.Host).To(Equal("localhost"))
	Expect(cfg.Server.Port).To(Equal(8000))
}

// ==============================================================
// File Secrets (*_FILE) Tests
// ==============================================================

// TestConfigLoader_FileSecrets tests reading secrets from files
func TestConfigLoader_FileSecrets(t *testing.T) {
	RegisterTestingT(t)

	tmpDir := t.TempDir()

	// Create secret files
	passwordFile := filepath.Join(tmpDir, "db-password")
	err := os.WriteFile(passwordFile, []byte("secret-password\n"), 0600)
	Expect(err).NotTo(HaveOccurred())

	usernameFile := filepath.Join(tmpDir, "db-username")
	err = os.WriteFile(usernameFile, []byte("  admin  \n"), 0600)
	Expect(err).NotTo(HaveOccurred())

	SetMinimalTestEnv(t)

	// Unset password and username from env (so file secrets will be used)
	t.Setenv("HYPERFLEET_DATABASE_PASSWORD", "")
	t.Setenv("HYPERFLEET_DATABASE_USERNAME", "")

	// Set *_FILE environment variables
	t.Setenv("HYPERFLEET_DATABASE_PASSWORD_FILE", passwordFile)
	t.Setenv("HYPERFLEET_DATABASE_USERNAME_FILE", usernameFile)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	cfg, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())
	Expect(cfg.Database.Password).To(Equal("secret-password"),
		"Password should be read from file and trimmed")
	Expect(cfg.Database.Username).To(Equal("admin"),
		"Username should be read from file and whitespace trimmed")
}

// TestConfigLoader_FileSecretNotFound tests error when secret file is missing
func TestConfigLoader_FileSecretNotFound(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	t.Setenv("HYPERFLEET_DATABASE_PASSWORD_FILE", "/nonexistent/secret")

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	_, err := loader.Load(ctx, cmd)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to read file secret"))
	Expect(err.Error()).To(ContainSubstring("/nonexistent/secret"))
}

// TestConfigLoader_EnvVarTakesPrecedenceOverFileSecret tests priority: env var > file secret
// When both HYPERFLEET_DATABASE_PASSWORD and HYPERFLEET_DATABASE_PASSWORD_FILE are set,
// the env var takes precedence because file secrets only set values if not already set.
func TestConfigLoader_EnvVarTakesPrecedenceOverFileSecret(t *testing.T) {
	RegisterTestingT(t)

	tmpDir := t.TempDir()
	passwordFile := filepath.Join(tmpDir, "password")
	err := os.WriteFile(passwordFile, []byte("file-password"), 0600)
	Expect(err).NotTo(HaveOccurred())

	SetMinimalTestEnv(t)

	t.Setenv("HYPERFLEET_DATABASE_PASSWORD", "env-password")
	t.Setenv("HYPERFLEET_DATABASE_PASSWORD_FILE", passwordFile)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	appConfig, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())
	// Explicitly assert that env var takes precedence over file secret
	Expect(appConfig.Database.Password).To(Equal("env-password"),
		"env var should take precedence over file secret")
}

// ==============================================================
// Validation Tests
// ==============================================================

// TestConfigLoader_MissingRequiredField tests validation of required fields
// Note: Most fields have defaults, so we test by setting invalid value instead
func TestConfigLoader_MissingRequiredField(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	// Set server port to invalid value (out of range) to trigger validation
	t.Setenv("HYPERFLEET_SERVER_PORT", "0") // Port must be min=1

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	_, err := loader.Load(ctx, cmd)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("Configuration validation failed"))
	Expect(err.Error()).To(ContainSubstring("Port"))
}

// TestConfigLoader_InvalidHostname tests hostname validation
func TestConfigLoader_InvalidHostname(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	// Set invalid hostname
	t.Setenv("HYPERFLEET_DATABASE_HOST", "invalid!@#$%")

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	_, err := loader.Load(ctx, cmd)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("validation failed"))
	Expect(err.Error()).To(ContainSubstring("hostname|ip"))
}

// TestConfigLoader_InvalidPortRange tests port validation
func TestConfigLoader_InvalidPortRange(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	// Set invalid port (out of range)
	t.Setenv("HYPERFLEET_SERVER_PORT", "99999")

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	_, err := loader.Load(ctx, cmd)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("validation failed"))
}

// TestConfigLoader_UnmarshalExactCatchesTyros tests that typos are caught
func TestConfigLoader_UnmarshalExactCatchesTyros(t *testing.T) {
	RegisterTestingT(t)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "typo-config.yaml")
	configContent := `
server:
  host: "localhost"
  port: 8000
  typo_field: "this should fail"  # Unknown field
database:
  host: "localhost"
  port: 5432
  name: "test"
  username: "test"
  password: "test"
logging:
  level: "info"
ocm:
  base_url: "https://api.example.com"
metrics:
  host: "localhost"
  port: 9090
health:
  host: "localhost"
  port: 8080
adapters:
  required:
    cluster: []
    nodepool: []
`
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	Expect(err).NotTo(HaveOccurred())

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	cmd.Flags().String("config", "", "config file path")
	_ = cmd.Flags().Set("config", configPath)
	ctx := context.Background()

	_, err = loader.Load(ctx, cmd)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("configuration unmarshal failed"))
	Expect(err.Error()).To(ContainSubstring("unknown/misspelled fields"))
}

// ==============================================================
// Backward Compatibility Tests
// ==============================================================

// TestConfigLoader_DeprecatedEnvVars tests backward compatibility
func TestConfigLoader_DeprecatedEnvVars(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	// Unset new env vars first so deprecated ones will be used
	t.Setenv("HYPERFLEET_LOGGING_LEVEL", "")
	t.Setenv("HYPERFLEET_LOGGING_OTEL_ENABLED", "")

	// Use deprecated environment variables
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("OTEL_ENABLED", "true")

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	cfg, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())
	Expect(cfg.Logging.Level).To(Equal("debug"),
		"Deprecated LOG_LEVEL should still work")
	Expect(cfg.Logging.OTel.Enabled).To(BeTrue(),
		"Deprecated OTEL_ENABLED should still work")
}

// TestConfigLoader_NewEnvVarOverridesOld tests that new vars take precedence
func TestConfigLoader_NewEnvVarOverridesOld(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	// Set both old and new environment variables
	t.Setenv("LOG_LEVEL", "debug")                      // Old
	t.Setenv("HYPERFLEET_LOGGING_LEVEL", "error")      // New (should win)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	cfg, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())
	Expect(cfg.Logging.Level).To(Equal("error"),
		"New environment variable should take precedence over deprecated one")
}

// ==============================================================
// JSON Array Parsing Tests
// ==============================================================

// TestConfigLoader_JSONArrayParsing tests adapter array parsing
func TestConfigLoader_JSONArrayParsing(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	t.Setenv("HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER", `["validation","dns","networking"]`)
	t.Setenv("HYPERFLEET_ADAPTERS_REQUIRED_NODEPOOL", `["validation"]`)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	cfg, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())
	Expect(cfg.Adapters.RequiredClusterAdapters()).To(Equal([]string{"validation", "dns", "networking"}))
	Expect(cfg.Adapters.RequiredNodePoolAdapters()).To(Equal([]string{"validation"}))
}

// TestConfigLoader_InvalidJSONArray tests error on malformed JSON
func TestConfigLoader_InvalidJSONArray(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	t.Setenv("HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER", `["unclosed`)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	_, err := loader.Load(ctx, cmd)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to parse"))
	Expect(err.Error()).To(ContainSubstring("JSON array"))
}

// ==============================================================
// Configuration Priority Tests
// ==============================================================

// TestConfigLoader_EnvVarOverridesFile tests env var > file priority
func TestConfigLoader_EnvVarOverridesFile(t *testing.T) {
	RegisterTestingT(t)

	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  host: "file-host"
  port: 7777
database:
  host: "localhost"
  port: 5432
  name: "test"
  username: "test"
  password: "test"
logging:
  level: "info"
ocm:
  base_url: "https://api.example.com"
metrics:
  host: "localhost"
  port: 9090
health:
  host: "localhost"
  port: 8080
adapters:
  required:
    cluster: []
    nodepool: []
`
	err := os.WriteFile(configPath, []byte(configContent), 0600)
	Expect(err).NotTo(HaveOccurred())

	// Set config path via env var
	t.Setenv("HYPERFLEET_CONFIG", configPath)

	// Environment variable (should override file)
	t.Setenv("HYPERFLEET_SERVER_HOST", "env-host")

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	cfg, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())
	Expect(cfg.Server.Host).To(Equal("env-host"),
		"Environment variable should override config file value")
	Expect(cfg.Server.Port).To(Equal(7777),
		"File value should be used when no env var")
}
