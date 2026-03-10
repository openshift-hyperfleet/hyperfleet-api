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

// TestConfigLoader_FlagOverridesEnvAndFile tests that CLI flags have highest priority
func TestConfigLoader_FlagOverridesEnvAndFile(t *testing.T) {
	RegisterTestingT(t)

	// Create config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  host: "file-host"
  port: 7000
database:
  host: "file-db-host"
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

	// Set config path and env vars
	t.Setenv("HYPERFLEET_CONFIG", configPath)
	t.Setenv("HYPERFLEET_SERVER_HOST", "env-host")
	t.Setenv("HYPERFLEET_SERVER_PORT", "8000")
	t.Setenv("HYPERFLEET_DATABASE_HOST", "env-db-host")

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	AddAllConfigFlags(cmd)

	// Set CLI flags (should have highest priority)
	cmd.Flags().Set("server-host", "flag-host")       //nolint:errcheck,gosec
	cmd.Flags().Set("server-port", "9000")            //nolint:errcheck,gosec
	// Note: not setting db-host flag, so env var should win

	ctx := context.Background()
	cfg, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())

	// CLI flag should win
	Expect(cfg.Server.Host).To(Equal("flag-host"),
		"CLI flag should override env var and config file")
	Expect(cfg.Server.Port).To(Equal(9000),
		"CLI flag should override env var and config file")

	// Env var should win (no flag set)
	Expect(cfg.Database.Host).To(Equal("env-db-host"),
		"Env var should override config file when no flag is set")
}

// TestConfigLoader_CompletePriorityChain tests all priority levels
func TestConfigLoader_CompletePriorityChain(t *testing.T) {
	RegisterTestingT(t)

	// Create config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `
server:
  host: "file-host"
  port: 7000
database:
  host: "file-db"
  port: 5432
  name: "filedb"
  username: "test"
  password: "test"
logging:
  level: "info"
  format: "file-format"
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

	t.Setenv("HYPERFLEET_CONFIG", configPath)
	t.Setenv("HYPERFLEET_SERVER_PORT", "8000")      // Env overrides file for port
	t.Setenv("HYPERFLEET_DATABASE_NAME", "env-db") // Env overrides file for db name
	t.Setenv("HYPERFLEET_LOGGING_FORMAT", "text")  // Env overrides file for log format (must be "json" or "text")

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	AddAllConfigFlags(cmd)

	// Flag overrides everything for server host
	cmd.Flags().Set("server-host", "flag-host") //nolint:errcheck,gosec
	// Note: not setting other flags, so env and file values should be used

	ctx := context.Background()
	cfg, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())

	// Priority 1: CLI Flag
	Expect(cfg.Server.Host).To(Equal("flag-host"), "Flag > Env > File > Default")

	// Priority 2: Env Var
	Expect(cfg.Server.Port).To(Equal(8000), "Env > File > Default")
	Expect(cfg.Database.Name).To(Equal("env-db"), "Env > File > Default")
	Expect(cfg.Logging.Format).To(Equal("text"), "Env > File > Default")

	// Priority 3: Config File
	Expect(cfg.Database.Host).To(Equal("file-db"), "File > Default (no flag or env)")
	Expect(cfg.Database.Port).To(Equal(5432), "File > Default (no flag or env)")
	Expect(cfg.Logging.Level).To(Equal("info"), "File > Default (no flag or env)")

	// Priority 4: Default (from NewApplicationConfig)
	// These fields are not set in file, env, or flags
	Expect(cfg.Server.Timeouts.Read.Seconds()).To(Equal(float64(5)), "Default value")
	Expect(cfg.Server.JWT.Enabled).To(BeTrue(), "Default value")
}

// TestConfigLoader_DefaultValues tests that defaults work when nothing is set
func TestConfigLoader_DefaultValues(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	AddAllConfigFlags(cmd)

	ctx := context.Background()
	cfg, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())

	// Verify default values from NewApplicationConfig
	Expect(cfg.Server.Host).To(Equal("localhost"), "Default server host")
	Expect(cfg.Server.Port).To(Equal(8000), "Default server port")
	Expect(cfg.Server.Timeouts.Read.Seconds()).To(Equal(float64(5)), "Default read timeout")
	Expect(cfg.Server.Timeouts.Write.Seconds()).To(Equal(float64(30)), "Default write timeout")
	Expect(cfg.Server.TLS.Enabled).To(BeFalse(), "Default TLS disabled")
	Expect(cfg.Server.JWT.Enabled).To(BeTrue(), "Default JWT enabled")
	Expect(cfg.Database.Dialect).To(Equal("postgres"), "Default database dialect")
	Expect(cfg.Database.Port).To(Equal(5432), "Default database port")
	Expect(cfg.Logging.Level).To(Equal("info"), "Default log level")
	Expect(cfg.Logging.Format).To(Equal("json"), "Default log format")
}

// TestConfigLoader_FlagOnlyOverridesDefaults tests flags work without file or env
func TestConfigLoader_FlagOnlyOverridesDefaults(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	AddAllConfigFlags(cmd)

	// Set only flags, no config file or env vars
	cmd.Flags().Set("server-host", "0.0.0.0") //nolint:errcheck,gosec
	cmd.Flags().Set("server-port", "9000")    //nolint:errcheck,gosec
	cmd.Flags().Set("log-level", "debug")     //nolint:errcheck,gosec

	ctx := context.Background()
	cfg, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())

	// Flags should override defaults
	Expect(cfg.Server.Host).To(Equal("0.0.0.0"), "Flag overrides default")
	Expect(cfg.Server.Port).To(Equal(9000), "Flag overrides default")
	Expect(cfg.Logging.Level).To(Equal("debug"), "Flag overrides default")

	// Other values should still be defaults
	Expect(cfg.Database.Port).To(Equal(5432), "Default value (no flag)")
	Expect(cfg.Logging.Format).To(Equal("json"), "Default value (no flag)")
}

// TestConfigLoader_EnvOnlyOverridesDefaults tests env vars work without file or flags
func TestConfigLoader_EnvOnlyOverridesDefaults(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	// Set only env vars, no config file or flags
	t.Setenv("HYPERFLEET_SERVER_HOST", "0.0.0.0")
	t.Setenv("HYPERFLEET_SERVER_PORT", "9000")
	t.Setenv("HYPERFLEET_LOGGING_LEVEL", "debug")

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	AddAllConfigFlags(cmd)

	ctx := context.Background()
	cfg, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())

	// Env vars should override defaults
	Expect(cfg.Server.Host).To(Equal("0.0.0.0"), "Env overrides default")
	Expect(cfg.Server.Port).To(Equal(9000), "Env overrides default")
	Expect(cfg.Logging.Level).To(Equal("debug"), "Env overrides default")

	// Other values should still be defaults
	Expect(cfg.Database.Port).To(Equal(5432), "Default value (no env)")
	Expect(cfg.Logging.Format).To(Equal("json"), "Default value (no env)")
}

// TestConfigLoader_MultipleFlags tests setting multiple flags
func TestConfigLoader_MultipleFlags(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	AddAllConfigFlags(cmd)

	// Set multiple flags
	cmd.Flags().Set("server-host", "api.example.com") //nolint:errcheck,gosec
	cmd.Flags().Set("server-port", "9000")            //nolint:errcheck,gosec
	cmd.Flags().Set("db-host", "db.example.com")      //nolint:errcheck,gosec
	cmd.Flags().Set("db-port", "3306")                //nolint:errcheck,gosec
	cmd.Flags().Set("log-level", "warn")              //nolint:errcheck,gosec
	cmd.Flags().Set("log-format", "text")             //nolint:errcheck,gosec

	ctx := context.Background()
	cfg, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())

	// All flags should be respected
	Expect(cfg.Server.Host).To(Equal("api.example.com"))
	Expect(cfg.Server.Port).To(Equal(9000))
	Expect(cfg.Database.Host).To(Equal("db.example.com"))
	Expect(cfg.Database.Port).To(Equal(3306))
	Expect(cfg.Logging.Level).To(Equal("warn"))
	Expect(cfg.Logging.Format).To(Equal("text"))
}

// TestConfigLoader_FlagParsing tests that flag values are correctly parsed
func TestConfigLoader_FlagParsing(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	AddAllConfigFlags(cmd)

	// Test different types
	cmd.Flags().Set("server-port", "9999")                    //nolint:errcheck,gosec // int
	cmd.Flags().Set("server-read-timeout", "10s")             //nolint:errcheck,gosec // duration
	cmd.Flags().Set("server-jwt-enabled", "false")            //nolint:errcheck,gosec // bool
	cmd.Flags().Set("db-max-open-connections", "50")          //nolint:errcheck,gosec // int
	cmd.Flags().Set("log-otel-sampling-rate", "0.5")          //nolint:errcheck,gosec // float64

	ctx := context.Background()
	cfg, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())

	// Verify types are correctly parsed
	Expect(cfg.Server.Port).To(Equal(9999), "int parsing")
	Expect(cfg.Server.Timeouts.Read.Seconds()).To(Equal(float64(10)), "duration parsing")
	Expect(cfg.Server.JWT.Enabled).To(BeFalse(), "bool parsing")
	Expect(cfg.Database.Pool.MaxConnections).To(Equal(50), "int parsing")
	Expect(cfg.Logging.OTel.SamplingRate).To(Equal(0.5), "float64 parsing")
}

// TestConfigLoader_OldEnvVarsAreLowestPriority verifies that old environment variables
// (LOG_LEVEL, DB_DEBUG, etc.) use SetDefault() and thus have lowest priority.
// This test validates that the KNOWN LIMITATION has been resolved:
// CLI flags and new env vars should override old env vars.
func TestConfigLoader_OldEnvVarsAreLowestPriority(t *testing.T) {
	RegisterTestingT(t)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	AddAllConfigFlags(cmd)

	// Set required database fields
	t.Setenv("HYPERFLEET_DATABASE_HOST", "localhost")
	t.Setenv("HYPERFLEET_DATABASE_PORT", "5432")
	t.Setenv("HYPERFLEET_DATABASE_NAME", "testdb")

	// Set old environment variable (should be lowest priority)
	t.Setenv("LOG_LEVEL", "debug")
	t.Setenv("DB_DEBUG", "true")

	// Set CLI flag (should override old env var)
	cmd.Flags().Set("log-level", "warn") //nolint:errcheck,gosec

	ctx := context.Background()
	cfg, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())

	// CLI flag should win over old env var
	Expect(cfg.Logging.Level).To(Equal("warn"), "CLI flag should override old env var LOG_LEVEL")

	// Old env var should still apply when no CLI flag is set
	Expect(cfg.Database.Debug).To(Equal(true), "Old env var DB_DEBUG should apply as fallback")
}
