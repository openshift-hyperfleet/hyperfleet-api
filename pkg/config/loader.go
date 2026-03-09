package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// ConfigLoader handles loading and validating application configuration
// following the HyperFleet Configuration Standard.
type ConfigLoader struct {
	viper               *viper.Viper
	validator           *validator.Validate
	deprecationWarnings []string
}

// NewConfigLoader creates a new configuration loader
func NewConfigLoader() *ConfigLoader {
	return &ConfigLoader{
		viper:               viper.New(),
		validator:           validator.New(),
		deprecationWarnings: make([]string, 0),
	}
}

// Load loads configuration from all sources according to priority:
// 1. Command-line flags (highest priority)
// 2. Environment variables
// 3. Configuration files
// 4. Defaults (lowest priority)
//
// Returns validated ApplicationConfig or error if validation fails.
func (l *ConfigLoader) Load(ctx context.Context, cmd *cobra.Command) (*ApplicationConfig, error) {
	// Step 1: Resolve and read config file (if exists)
	if err := l.resolveAndReadConfigFile(ctx, cmd); err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Step 2: Setup environment variable handling with HYPERFLEET_ prefix
	l.viper.SetEnvPrefix("HYPERFLEET")
	l.viper.AutomaticEnv()
	l.viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// Bind all environment variables explicitly (required for Unmarshal to work)
	l.bindAllEnvVars()

	// Step 3: Handle backward compatibility for old environment variables
	l.handleBackwardCompatibility(ctx)

	// Step 4: Bind command-line flags to Viper (maps flag names to nested config keys)
	l.bindFlags(cmd)

	// Step 5: Handle file-based secrets (env vars ending with _FILE)
	if err := l.handleFileSecrets(ctx); err != nil {
		return nil, fmt.Errorf("failed to handle file secrets: %w", err)
	}

	// Step 5.5: Handle JSON array environment variables (for adapters)
	if err := l.handleJSONArrayEnvVars(ctx); err != nil {
		return nil, fmt.Errorf("failed to handle JSON array env vars: %w", err)
	}

	// Step 6: Unmarshal into ApplicationConfig struct
	// Start with defaults, then overlay config file/env vars/flags
	config := NewApplicationConfig()
	if err := l.viper.UnmarshalExact(config); err != nil {
		return nil, fmt.Errorf(
			"configuration unmarshal failed: %w\nThis usually means unknown/misspelled fields in config file",
			err)
	}

	// Step 7: Validate configuration
	if err := l.validateConfig(config); err != nil {
		return nil, err
	}

	// Step 8: Log deprecation warnings once at startup
	l.logDeprecationWarnings(ctx)

	return config, nil
}

// resolveAndReadConfigFile resolves config file path and reads it into Viper
// Priority: --config flag > HYPERFLEET_CONFIG env > default paths
func (l *ConfigLoader) resolveAndReadConfigFile(ctx context.Context, cmd *cobra.Command) error {
	var configPath string
	var explicitPath bool

	// Priority 1: --config flag
	if cmd.Flags().Changed("config") {
		configPath, _ = cmd.Flags().GetString("config")
		explicitPath = true
		logger.With(ctx, "config_path", configPath, "source", "flag").Info("Config file specified via --config flag")
	}

	// Priority 2: HYPERFLEET_CONFIG environment variable
	if configPath == "" {
		if envPath := os.Getenv("HYPERFLEET_CONFIG"); envPath != "" {
			configPath = envPath
			explicitPath = true
			logger.With(ctx, "config_path", configPath, "source", "env").Info("Config file specified via HYPERFLEET_CONFIG")
		}
	}

	// Priority 3: Default paths
	if configPath == "" {
		// Try production path first
		prodPath := "/etc/hyperfleet/config.yaml"
		if _, err := os.Stat(prodPath); err == nil {
			configPath = prodPath
			logger.With(ctx, "config_path", configPath, "source", "default_production").
				Info("Using production default config file")
		} else {
			// Try development path
			devPath := "./configs/config.yaml"
			if _, err := os.Stat(devPath); err == nil {
				configPath = devPath
				logger.With(ctx, "config_path", configPath, "source", "default_development").
					Info("Using development default config file")
			}
		}
	}

	// If no config file found, continue with env vars and flags only
	if configPath == "" {
		logger.Info(ctx, "No config file found, using environment variables and flags only")
		return nil
	}

	// If explicitly specified but doesn't exist, this is a fatal error
	if explicitPath {
		if _, err := os.Stat(configPath); err != nil {
			return fmt.Errorf("explicitly specified config file not found: %s", configPath)
		}
	}

	// Read the config file
	l.viper.SetConfigFile(configPath)
	if err := l.viper.ReadInConfig(); err != nil {
		if explicitPath {
			// Fatal error if explicitly specified
			return fmt.Errorf("failed to read config file %s: %w", configPath, err)
		}
		// Just log warning if using default path
		logger.With(ctx, "config_path", configPath).WithError(err).Warn("Failed to read default config file, continuing")
		return nil
	}

	logger.With(ctx, "config_path", configPath).Info("Successfully loaded config file")
	return nil
}

// handleBackwardCompatibility maps old environment variables to new standard names
// Collects deprecation warnings for logging at startup
//
// KNOWN LIMITATION: Old environment variables (LOG_LEVEL, OTEL_ENABLED, etc.) have higher
// priority than CLI flags due to v.Set() being called before BindPFlags().
// This is a temporary limitation and will be resolved when old environment variables
// support is removed in the next Helm update.
//
// Workaround: Use new environment variables (HYPERFLEET_*) instead of old ones,
// or avoid mixing old environment variables with CLI flags.
func (l *ConfigLoader) handleBackwardCompatibility(ctx context.Context) {
	// Backward compatibility mappings: old env var -> new Viper key (updated to match new structure)
	backwardCompatMappings := map[string]string{
		// Logging config
		"LOG_LEVEL":  "logging.level",
		"LOG_FORMAT": "logging.format",
		"LOG_OUTPUT": "logging.output",

		// Database config
		"DB_DEBUG": "database.debug",

		// OpenTelemetry config - updated to match new structure
		"OTEL_ENABLED":       "logging.otel.enabled",
		"OTEL_SAMPLING_RATE": "logging.otel.sampling_rate",

		// Masking config - updated to match new structure (now arrays, not strings)
		"MASKING_ENABLED": "logging.masking.enabled",
		"MASKING_HEADERS": "logging.masking.headers",
		"MASKING_FIELDS":  "logging.masking.fields",
	}

	for oldEnvVar, viperKey := range backwardCompatMappings {
		oldValue := os.Getenv(oldEnvVar)
		if oldValue == "" {
			continue
		}

		// Check if new environment variable is also set
		newEnvVar := "HYPERFLEET_" + strings.ReplaceAll(strings.ToUpper(viperKey), ".", "_")
		newValue := os.Getenv(newEnvVar)

		if newValue != "" {
			// Both old and new are set - warn and use new value
			warning := fmt.Sprintf(
				"Both old (%s=%s) and new (%s=%s) environment variables are set. Using new value. "+
					"Please remove the old variable.",
				oldEnvVar, oldValue, newEnvVar, newValue,
			)
			l.deprecationWarnings = append(l.deprecationWarnings, warning)
			logger.With(ctx, "old_var", oldEnvVar, "new_var", newEnvVar).Warn(warning)
		} else {
			// Only old env var is set - use it but warn about deprecation
			if !l.viper.IsSet(viperKey) {
				l.viper.Set(viperKey, oldValue)
			}
			warning := fmt.Sprintf(
				"Deprecated environment variable %s is in use. Please migrate to %s.",
				oldEnvVar, newEnvVar,
			)
			l.deprecationWarnings = append(l.deprecationWarnings, warning)
		}
	}
}

// handleFileSecrets processes environment variables ending with _FILE suffix
// Reads the file content and sets the corresponding Viper key
// Note: Fields that already store file paths (e.g., cert_file, key_file, acl.file) are excluded
// because they should be set directly via environment variables, not loaded from files.
func (l *ConfigLoader) handleFileSecrets(ctx context.Context) error {
	// Define file secret mappings: env var suffix -> viper key (updated to match new structure)
	fileSecretMappings := map[string]string{
		"HYPERFLEET_DATABASE_HOST_FILE":      "database.host",
		"HYPERFLEET_DATABASE_PORT_FILE":      "database.port",
		"HYPERFLEET_DATABASE_USERNAME_FILE":  "database.username",
		"HYPERFLEET_DATABASE_PASSWORD_FILE":  "database.password",
		"HYPERFLEET_DATABASE_NAME_FILE":      "database.name",
		"HYPERFLEET_OCM_CLIENT_ID_FILE":      "ocm.client_id",
		"HYPERFLEET_OCM_CLIENT_SECRET_FILE":  "ocm.client_secret",
		"HYPERFLEET_OCM_SELF_TOKEN_FILE":     "ocm.self_token",
	}

	for envVar, viperKey := range fileSecretMappings {
		filePath := os.Getenv(envVar)
		if filePath == "" {
			continue
		}

		// Read file content
		content, err := os.ReadFile(filePath) //nolint:gosec // File path from env var is expected pattern for secrets
		if err != nil {
			return fmt.Errorf("failed to read file secret %s (from %s): %w", filePath, envVar, err)
		}

		// Trim whitespace and newlines
		value := strings.TrimSpace(string(content))

		// Set in Viper (only if not already set by higher priority source)
		if !l.viper.IsSet(viperKey) {
			l.viper.Set(viperKey, value)
			logger.With(ctx, "env_var", envVar, "viper_key", viperKey).Debug("Loaded secret from file")
		}
	}

	return nil
}

// validateConfig validates the configuration using struct tags
// Returns user-friendly error messages with field paths and hints
func (l *ConfigLoader) validateConfig(config *ApplicationConfig) error {
	// First, run struct tag validation
	err := l.validator.Struct(config)
	if err == nil {
		// Struct tag validation passed, now run custom validations
		// Note: validator treats time.Duration as int64, so min/max tags don't work correctly
		// Also, omitempty doesn't enforce required_if logic for conditional fields
		if err := config.Server.Timeouts.Validate(); err != nil {
			return fmt.Errorf("server timeouts validation failed: %w", err)
		}
		if err := config.Server.TLS.Validate(); err != nil {
			return fmt.Errorf("server TLS validation failed: %w", err)
		}
		if err := config.Health.Validate(); err != nil {
			return fmt.Errorf("health config validation failed: %w", err)
		}
		if err := config.Health.TLS.Validate(); err != nil {
			return fmt.Errorf("health TLS validation failed: %w", err)
		}
		if err := config.Metrics.TLS.Validate(); err != nil {
			return fmt.Errorf("metrics TLS validation failed: %w", err)
		}
		return nil
	}

	// Format validation errors for user-friendly display
	validationErrors, ok := err.(validator.ValidationErrors)
	if !ok {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	var errMessages []string
	errMessages = append(errMessages, "Configuration validation failed:")

	for _, fieldErr := range validationErrors {
		// Build full field path (e.g., "Config.Server.Port")
		fieldPath := fieldErr.Namespace()

		// Get the struct field name in Viper format (e.g., "server.port")
		// Preserve dot-separated segments for config file path
		viperPath := strings.ToLower(strings.TrimPrefix(fieldPath, "ApplicationConfig."))

		// Build error message with helpful hints
		msg := fmt.Sprintf("  - Field '%s' failed validation: %s", fieldPath, fieldErr.Tag())
		if fieldErr.Param() != "" {
			msg += fmt.Sprintf(" (parameter: %s)", fieldErr.Param())
		}
		msg += fmt.Sprintf("\n    Value: %v", fieldErr.Value())
		msg += "\n    Please provide valid value via:"
		msg += fmt.Sprintf("\n      • Config file: %s", viperPath)
		msg += fmt.Sprintf("\n      • Environment variable: HYPERFLEET_%s",
			strings.ToUpper(strings.ReplaceAll(viperPath, ".", "_")))
		msg += fmt.Sprintf("\n      • CLI flag: --%s", strings.ReplaceAll(viperPath, ".", "-"))

		errMessages = append(errMessages, msg)
	}

	return fmt.Errorf("%s", strings.Join(errMessages, "\n"))
}

// handleJSONArrayEnvVars processes environment variables containing JSON arrays
// Viper doesn't automatically parse JSON from env vars, so we handle this explicitly
// Used for: HYPERFLEET_ADAPTERS_CLUSTER_ADAPTERS and HYPERFLEET_ADAPTERS_NODEPOOL_ADAPTERS
func (l *ConfigLoader) handleJSONArrayEnvVars(ctx context.Context) error {
	// Map of env var name -> viper key
	jsonArrayMappings := map[string]string{
		"HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER":  "adapters.required.cluster",
		"HYPERFLEET_ADAPTERS_REQUIRED_NODEPOOL": "adapters.required.nodepool",
	}

	for envVar, viperKey := range jsonArrayMappings {
		jsonValue := os.Getenv(envVar)
		if jsonValue == "" {
			continue
		}

		// Parse JSON array
		var arrayValue []string
		if err := json.Unmarshal([]byte(jsonValue), &arrayValue); err != nil {
			return fmt.Errorf("failed to parse %s as JSON array: %w (value: %s)", envVar, err, jsonValue)
		}

		// Always set the parsed JSON array value to override Viper's auto-env CSV parsing.
		// Viper's AutomaticEnv treats comma-separated strings as arrays, incorrectly parsing
		// JSON arrays like '["a","b"]' as ["[\"a\"", "\"b\"]"] instead of ["a", "b"].
		//
		// We use Set() to ensure proper JSON parsing overrides Viper's CSV parsing.
		// This maintains ENV > Config > Default precedence for adapters.
		//
		// NOTE: Adapters currently have no CLI flags (see bindFlags line 494).
		// If CLI flags are added in the future, this code needs updating to check
		// if the value came from a flag before calling Set().
		l.viper.Set(viperKey, arrayValue)
		logger.With(ctx, "env_var", envVar, "count", len(arrayValue)).Debug("Parsed JSON array from environment")
	}

	return nil
}

// bindAllEnvVars explicitly binds all configuration keys to environment variables
// This is required for Viper's Unmarshal() to work with env vars (AutomaticEnv only works with Get* methods)
//
//nolint:gosec,errcheck // BindEnv errors are rare and indicate programming errors, not runtime errors
func (l *ConfigLoader) bindAllEnvVars() {
	// Server config - updated to match new structure
	l.viper.BindEnv("server.hostname")
	l.viper.BindEnv("server.host")
	l.viper.BindEnv("server.port")
	l.viper.BindEnv("server.timeouts.read")
	l.viper.BindEnv("server.timeouts.write")
	l.viper.BindEnv("server.tls.enabled")
	l.viper.BindEnv("server.tls.cert_file")
	l.viper.BindEnv("server.tls.key_file")
	l.viper.BindEnv("server.jwt.enabled")
	l.viper.BindEnv("server.jwk.cert_file")
	l.viper.BindEnv("server.jwk.cert_url")
	l.viper.BindEnv("server.authz.enabled")
	l.viper.BindEnv("server.acl.file")

	// Database config - updated to match new structure
	l.viper.BindEnv("database.dialect")
	l.viper.BindEnv("database.host")
	l.viper.BindEnv("database.port")
	l.viper.BindEnv("database.name")
	l.viper.BindEnv("database.username")
	l.viper.BindEnv("database.password")
	l.viper.BindEnv("database.debug")
	l.viper.BindEnv("database.ssl.mode")
	l.viper.BindEnv("database.ssl.root_cert_file")
	l.viper.BindEnv("database.pool.max_connections")

	// Logging config - updated to match new structure
	l.viper.BindEnv("logging.level")
	l.viper.BindEnv("logging.format")
	l.viper.BindEnv("logging.output")
	l.viper.BindEnv("logging.otel.enabled")
	l.viper.BindEnv("logging.otel.sampling_rate")
	l.viper.BindEnv("logging.masking.enabled")
	l.viper.BindEnv("logging.masking.headers")
	l.viper.BindEnv("logging.masking.fields")

	// OCM config - updated to match new structure (snake_case)
	l.viper.BindEnv("ocm.base_url")
	l.viper.BindEnv("ocm.client_id")
	l.viper.BindEnv("ocm.client_secret")
	l.viper.BindEnv("ocm.self_token")
	l.viper.BindEnv("ocm.token_url")
	l.viper.BindEnv("ocm.debug")
	l.viper.BindEnv("ocm.mock.enabled")

	// Metrics config - updated to match new structure
	l.viper.BindEnv("metrics.host")
	l.viper.BindEnv("metrics.port")
	l.viper.BindEnv("metrics.tls.enabled")
	l.viper.BindEnv("metrics.label_metrics_inclusion_duration")

	// Health config - updated to match new structure
	l.viper.BindEnv("health.host")
	l.viper.BindEnv("health.port")
	l.viper.BindEnv("health.tls.enabled")
	l.viper.BindEnv("health.shutdown_timeout")

	// Adapters config
	l.viper.BindEnv("adapters.required.cluster")
	l.viper.BindEnv("adapters.required.nodepool")
}

// bindFlags binds command-line flags to their corresponding Viper config keys
// Maps user-friendly flag names (--db-host) to nested config keys (database.host)
// This is required for UnmarshalExact to work correctly with nested config structures
//
//nolint:gosec,errcheck // BindPFlag errors are rare and indicate programming errors, not runtime errors
func (l *ConfigLoader) bindFlags(cmd *cobra.Command) {
	// --config flag (special case - not part of ApplicationConfig)
	// This is handled separately in resolveAndReadConfigFile()

	// Server flags: --server-* -> server.*
	l.viper.BindPFlag("server.hostname", cmd.Flags().Lookup("server-hostname"))
	l.viper.BindPFlag("server.host", cmd.Flags().Lookup("server-host"))
	l.viper.BindPFlag("server.port", cmd.Flags().Lookup("server-port"))
	l.viper.BindPFlag("server.timeouts.read", cmd.Flags().Lookup("server-read-timeout"))
	l.viper.BindPFlag("server.timeouts.write", cmd.Flags().Lookup("server-write-timeout"))
	l.viper.BindPFlag("server.tls.cert_file", cmd.Flags().Lookup("server-https-cert-file"))
	l.viper.BindPFlag("server.tls.key_file", cmd.Flags().Lookup("server-https-key-file"))
	l.viper.BindPFlag("server.tls.enabled", cmd.Flags().Lookup("server-https-enabled"))
	l.viper.BindPFlag("server.jwt.enabled", cmd.Flags().Lookup("server-jwt-enabled"))
	l.viper.BindPFlag("server.jwk.cert_file", cmd.Flags().Lookup("server-jwk-cert-file"))
	l.viper.BindPFlag("server.jwk.cert_url", cmd.Flags().Lookup("server-jwk-cert-url"))
	l.viper.BindPFlag("server.authz.enabled", cmd.Flags().Lookup("server-authz-enabled"))
	l.viper.BindPFlag("server.acl.file", cmd.Flags().Lookup("server-acl-file"))

	// Database flags: --db-* -> database.*
	l.viper.BindPFlag("database.host", cmd.Flags().Lookup("db-host"))
	l.viper.BindPFlag("database.port", cmd.Flags().Lookup("db-port"))
	l.viper.BindPFlag("database.username", cmd.Flags().Lookup("db-username"))
	l.viper.BindPFlag("database.password", cmd.Flags().Lookup("db-password"))
	l.viper.BindPFlag("database.name", cmd.Flags().Lookup("db-name"))
	l.viper.BindPFlag("database.dialect", cmd.Flags().Lookup("db-dialect"))
	l.viper.BindPFlag("database.ssl.mode", cmd.Flags().Lookup("db-ssl-mode"))
	l.viper.BindPFlag("database.debug", cmd.Flags().Lookup("db-debug"))
	l.viper.BindPFlag("database.pool.max_connections", cmd.Flags().Lookup("db-max-open-connections"))
	l.viper.BindPFlag("database.ssl.root_cert_file", cmd.Flags().Lookup("db-root-cert-file"))

	// Logging flags: --log-* -> logging.*
	l.viper.BindPFlag("logging.level", cmd.Flags().Lookup("log-level"))
	l.viper.BindPFlag("logging.format", cmd.Flags().Lookup("log-format"))
	l.viper.BindPFlag("logging.output", cmd.Flags().Lookup("log-output"))
	l.viper.BindPFlag("logging.otel.enabled", cmd.Flags().Lookup("log-otel-enabled"))
	l.viper.BindPFlag("logging.otel.sampling_rate", cmd.Flags().Lookup("log-otel-sampling-rate"))
	l.viper.BindPFlag("logging.masking.enabled", cmd.Flags().Lookup("log-masking-enabled"))
	l.viper.BindPFlag("logging.masking.headers", cmd.Flags().Lookup("log-masking-sensitive-headers"))
	l.viper.BindPFlag("logging.masking.fields", cmd.Flags().Lookup("log-masking-sensitive-fields"))

	// Metrics flags: --metrics-* -> metrics.*
	l.viper.BindPFlag("metrics.host", cmd.Flags().Lookup("metrics-host"))
	l.viper.BindPFlag("metrics.port", cmd.Flags().Lookup("metrics-port"))
	l.viper.BindPFlag("metrics.tls.enabled", cmd.Flags().Lookup("metrics-https-enabled"))
	l.viper.BindPFlag("metrics.label_metrics_inclusion_duration",
		cmd.Flags().Lookup("metrics-label-metrics-inclusion-duration"))

	// Health flags: --health-* -> health.*
	l.viper.BindPFlag("health.host", cmd.Flags().Lookup("health-host"))
	l.viper.BindPFlag("health.port", cmd.Flags().Lookup("health-port"))
	l.viper.BindPFlag("health.tls.enabled", cmd.Flags().Lookup("health-https-enabled"))
	l.viper.BindPFlag("health.shutdown_timeout", cmd.Flags().Lookup("health-shutdown-timeout"))

	// OCM flags: --ocm-* -> ocm.*
	l.viper.BindPFlag("ocm.base_url", cmd.Flags().Lookup("ocm-base-url"))
	l.viper.BindPFlag("ocm.token_url", cmd.Flags().Lookup("ocm-token-url"))
	l.viper.BindPFlag("ocm.client_id", cmd.Flags().Lookup("ocm-client-id"))
	l.viper.BindPFlag("ocm.client_secret", cmd.Flags().Lookup("ocm-client-secret"))
	l.viper.BindPFlag("ocm.self_token", cmd.Flags().Lookup("ocm-self-token"))
	l.viper.BindPFlag("ocm.debug", cmd.Flags().Lookup("ocm-debug"))
	l.viper.BindPFlag("ocm.mock.enabled", cmd.Flags().Lookup("ocm-mock-enabled"))

	// Adapters: No flags (configured via env vars only)
}

// logDeprecationWarnings logs all collected deprecation warnings once at startup
func (l *ConfigLoader) logDeprecationWarnings(ctx context.Context) {
	if len(l.deprecationWarnings) == 0 {
		return
	}

	logger.Warn(ctx, "=== DEPRECATION WARNINGS ===")
	for _, warning := range l.deprecationWarnings {
		logger.Warn(ctx, warning)
	}
	logger.With(ctx, "count", len(l.deprecationWarnings)).Warn("=== End of deprecation warnings ===")
}
