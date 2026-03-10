package config

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// ConfigLoader handles loading and validating application configuration
// following the HyperFleet Configuration Standard.
type ConfigLoader struct {
	viper               *viper.Viper
	validator           *validator.Validate
	deprecationWarnings []string
	explicitlyBoundKeys map[string]bool // Tracks keys explicitly bound via BindEnv/BindPFlag
}

// NewConfigLoader creates a new configuration loader
func NewConfigLoader() *ConfigLoader {
	return &ConfigLoader{
		viper:               viper.New(),
		validator:           validator.New(),
		deprecationWarnings: make([]string, 0),
		explicitlyBoundKeys: make(map[string]bool),
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
	l.viper.SetEnvPrefix(EnvPrefix)
	l.viper.AutomaticEnv()
	l.viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))

	// Bind all environment variables explicitly (required for Unmarshal to work)
	l.bindAllEnvVars()

	// Step 3: Handle backward compatibility for old environment variables
	l.handleBackwardCompatibility(ctx)

	// Step 4: Bind command-line flags to Viper (maps flag names to nested config keys)
	l.bindFlags(cmd)

	// Step 4.5: Validate that all bound keys match actual struct fields
	// This catches typos in bindAllEnvVars() or bindFlags() early
	if err := l.validateBoundKeys(); err != nil {
		return nil, err
	}

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
// Old environment variables are set as defaults (lowest priority) using SetDefault(),
// which means they only apply when no other source (config file, new env vars, CLI flags)
// provides the value. This gives correct backward compatibility semantics:
// old env vars are a fallback, not an override.
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
		newEnvVar := EnvPrefix + "_" + strings.ReplaceAll(strings.ToUpper(viperKey), ".", "_")
		newValue := os.Getenv(newEnvVar)

		if newValue != "" {
			// Both old and new are set - warn and use new value
			// No need to call SetDefault() since new env var (via BindEnv) has higher priority
			warning := fmt.Sprintf(
				"Both old (%s=%s) and new (%s=%s) environment variables are set. Using new value. "+
					"Please remove the old variable.",
				oldEnvVar, oldValue, newEnvVar, newValue,
			)
			l.deprecationWarnings = append(l.deprecationWarnings, warning)
			logger.With(ctx, "old_var", oldEnvVar, "new_var", newEnvVar).Warn(warning)
		} else {
			// Only old env var is set - set it as default (lowest priority)
			// This ensures CLI flags, new env vars, and config file can override it
			l.viper.SetDefault(viperKey, oldValue)
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
		EnvPrefix + "_DATABASE_HOST_FILE":     "database.host",
		EnvPrefix + "_DATABASE_PORT_FILE":     "database.port",
		EnvPrefix + "_DATABASE_USERNAME_FILE": "database.username",
		EnvPrefix + "_DATABASE_PASSWORD_FILE": "database.password",
		EnvPrefix + "_DATABASE_NAME_FILE":     "database.name",
		EnvPrefix + "_OCM_CLIENT_ID_FILE":     "ocm.client_id",     //nolint:gosec // Config key path, not credential
		EnvPrefix + "_OCM_CLIENT_SECRET_FILE": "ocm.client_secret", //nolint:gosec // Config key path, not credential
		EnvPrefix + "_OCM_SELF_TOKEN_FILE":    "ocm.self_token",    //nolint:gosec // Config key path, not credential
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
		EnvPrefix + "_ADAPTERS_REQUIRED_CLUSTER":  "adapters.required.cluster",
		EnvPrefix + "_ADAPTERS_REQUIRED_NODEPOOL": "adapters.required.nodepool",
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

// bindEnv wraps viper.BindEnv and tracks the key for validation
func (l *ConfigLoader) bindEnv(key string) {
	l.viper.BindEnv(key) //nolint:errcheck,gosec // BindEnv errors are rare and indicate programming errors
	l.explicitlyBoundKeys[key] = true
}

// bindPFlag wraps viper.BindPFlag and tracks the key for validation
func (l *ConfigLoader) bindPFlag(key string, flag *pflag.Flag) {
	l.viper.BindPFlag(key, flag) //nolint:errcheck,gosec // BindPFlag errors are rare and indicate programming errors
	l.explicitlyBoundKeys[key] = true
}

// bindAllEnvVars explicitly binds all configuration keys to environment variables
// This is required for Viper's Unmarshal() to work with env vars (AutomaticEnv only works with Get* methods)
func (l *ConfigLoader) bindAllEnvVars() {
	// Server config
	l.bindEnv("server.hostname")
	l.bindEnv("server.host")
	l.bindEnv("server.port")
	l.bindEnv("server.timeouts.read")
	l.bindEnv("server.timeouts.write")
	l.bindEnv("server.tls.enabled")
	l.bindEnv("server.tls.cert_file")
	l.bindEnv("server.tls.key_file")
	l.bindEnv("server.jwt.enabled")
	l.bindEnv("server.jwk.cert_file")
	l.bindEnv("server.jwk.cert_url")
	l.bindEnv("server.authz.enabled")
	l.bindEnv("server.acl.file")

	// Database config
	l.bindEnv("database.dialect")
	l.bindEnv("database.host")
	l.bindEnv("database.port")
	l.bindEnv("database.name")
	l.bindEnv("database.username")
	l.bindEnv("database.password")
	l.bindEnv("database.debug")
	l.bindEnv("database.ssl.mode")
	l.bindEnv("database.ssl.root_cert_file")
	l.bindEnv("database.pool.max_connections")

	// Logging config
	l.bindEnv("logging.level")
	l.bindEnv("logging.format")
	l.bindEnv("logging.output")
	l.bindEnv("logging.otel.enabled")
	l.bindEnv("logging.otel.sampling_rate")
	l.bindEnv("logging.masking.enabled")
	l.bindEnv("logging.masking.headers")
	l.bindEnv("logging.masking.fields")

	// OCM config
	l.bindEnv("ocm.base_url")
	l.bindEnv("ocm.client_id")
	l.bindEnv("ocm.client_secret")
	l.bindEnv("ocm.self_token")
	l.bindEnv("ocm.token_url")
	l.bindEnv("ocm.debug")
	l.bindEnv("ocm.mock.enabled")

	// Metrics config
	l.bindEnv("metrics.host")
	l.bindEnv("metrics.port")
	l.bindEnv("metrics.tls.enabled")
	l.bindEnv("metrics.label_metrics_inclusion_duration")

	// Health config
	l.bindEnv("health.host")
	l.bindEnv("health.port")
	l.bindEnv("health.tls.enabled")
	l.bindEnv("health.shutdown_timeout")

	// Adapters config
	l.bindEnv("adapters.required.cluster")
	l.bindEnv("adapters.required.nodepool")
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
	l.bindPFlag("server.hostname", cmd.Flags().Lookup("server-hostname"))
	l.bindPFlag("server.host", cmd.Flags().Lookup("server-host"))
	l.bindPFlag("server.port", cmd.Flags().Lookup("server-port"))
	l.bindPFlag("server.timeouts.read", cmd.Flags().Lookup("server-read-timeout"))
	l.bindPFlag("server.timeouts.write", cmd.Flags().Lookup("server-write-timeout"))
	l.bindPFlag("server.tls.cert_file", cmd.Flags().Lookup("server-https-cert-file"))
	l.bindPFlag("server.tls.key_file", cmd.Flags().Lookup("server-https-key-file"))
	l.bindPFlag("server.tls.enabled", cmd.Flags().Lookup("server-https-enabled"))
	l.bindPFlag("server.jwt.enabled", cmd.Flags().Lookup("server-jwt-enabled"))
	l.bindPFlag("server.jwk.cert_file", cmd.Flags().Lookup("server-jwk-cert-file"))
	l.bindPFlag("server.jwk.cert_url", cmd.Flags().Lookup("server-jwk-cert-url"))
	l.bindPFlag("server.authz.enabled", cmd.Flags().Lookup("server-authz-enabled"))
	l.bindPFlag("server.acl.file", cmd.Flags().Lookup("server-acl-file"))

	// Database flags: --db-* -> database.*
	l.bindPFlag("database.host", cmd.Flags().Lookup("db-host"))
	l.bindPFlag("database.port", cmd.Flags().Lookup("db-port"))
	l.bindPFlag("database.username", cmd.Flags().Lookup("db-username"))
	l.bindPFlag("database.password", cmd.Flags().Lookup("db-password"))
	l.bindPFlag("database.name", cmd.Flags().Lookup("db-name"))
	l.bindPFlag("database.dialect", cmd.Flags().Lookup("db-dialect"))
	l.bindPFlag("database.ssl.mode", cmd.Flags().Lookup("db-ssl-mode"))
	l.bindPFlag("database.debug", cmd.Flags().Lookup("db-debug"))
	l.bindPFlag("database.pool.max_connections", cmd.Flags().Lookup("db-max-open-connections"))
	l.bindPFlag("database.ssl.root_cert_file", cmd.Flags().Lookup("db-root-cert-file"))

	// Logging flags: --log-* -> logging.*
	l.bindPFlag("logging.level", cmd.Flags().Lookup("log-level"))
	l.bindPFlag("logging.format", cmd.Flags().Lookup("log-format"))
	l.bindPFlag("logging.output", cmd.Flags().Lookup("log-output"))
	l.bindPFlag("logging.otel.enabled", cmd.Flags().Lookup("log-otel-enabled"))
	l.bindPFlag("logging.otel.sampling_rate", cmd.Flags().Lookup("log-otel-sampling-rate"))
	l.bindPFlag("logging.masking.enabled", cmd.Flags().Lookup("log-masking-enabled"))
	l.bindPFlag("logging.masking.headers", cmd.Flags().Lookup("log-masking-sensitive-headers"))
	l.bindPFlag("logging.masking.fields", cmd.Flags().Lookup("log-masking-sensitive-fields"))

	// Metrics flags: --metrics-* -> metrics.*
	l.bindPFlag("metrics.host", cmd.Flags().Lookup("metrics-host"))
	l.bindPFlag("metrics.port", cmd.Flags().Lookup("metrics-port"))
	l.bindPFlag("metrics.tls.enabled", cmd.Flags().Lookup("metrics-https-enabled"))
	l.bindPFlag("metrics.label_metrics_inclusion_duration",
		cmd.Flags().Lookup("metrics-label-metrics-inclusion-duration"))

	// Health flags: --health-* -> health.*
	l.bindPFlag("health.host", cmd.Flags().Lookup("health-host"))
	l.bindPFlag("health.port", cmd.Flags().Lookup("health-port"))
	l.bindPFlag("health.tls.enabled", cmd.Flags().Lookup("health-https-enabled"))
	l.bindPFlag("health.shutdown_timeout", cmd.Flags().Lookup("health-shutdown-timeout"))

	// OCM flags: --ocm-* -> ocm.*
	l.bindPFlag("ocm.base_url", cmd.Flags().Lookup("ocm-base-url"))
	l.bindPFlag("ocm.token_url", cmd.Flags().Lookup("ocm-token-url"))
	l.bindPFlag("ocm.client_id", cmd.Flags().Lookup("ocm-client-id"))
	l.bindPFlag("ocm.client_secret", cmd.Flags().Lookup("ocm-client-secret"))
	l.bindPFlag("ocm.self_token", cmd.Flags().Lookup("ocm-self-token"))
	l.bindPFlag("ocm.debug", cmd.Flags().Lookup("ocm-debug"))
	l.bindPFlag("ocm.mock.enabled", cmd.Flags().Lookup("ocm-mock-enabled"))

	// Adapters: No flags (configured via env vars only)
}

// validateBoundKeys validates that all keys bound in bindAllEnvVars() and bindFlags()
// match actual struct fields in ApplicationConfig. This catches typos and mismatches
// that would otherwise cause silent configuration failures.
//
// NOTE: This only validates keys that we explicitly bind (via BindEnv/BindPFlag),
// not keys from config files. Config file typos are caught later by UnmarshalExact.
func (l *ConfigLoader) validateBoundKeys() error {
	// Collect all valid configuration keys from ApplicationConfig struct tags
	validKeys := collectValidConfigKeys(reflect.TypeOf(ApplicationConfig{}), "")
	validKeySet := make(map[string]bool)
	for _, key := range validKeys {
		validKeySet[key] = true
	}

	// Check that all explicitly bound keys match struct fields
	var invalidKeys []string
	for key := range l.explicitlyBoundKeys {
		if !validKeySet[key] {
			invalidKeys = append(invalidKeys, key)
		}
	}

	if len(invalidKeys) > 0 {
		return fmt.Errorf(
			"configuration binding error: the following keys do not match any struct fields: %v\n"+
				"This usually indicates a typo in bindAllEnvVars() or bindFlags()",
			invalidKeys,
		)
	}

	return nil
}

// collectValidConfigKeys recursively collects all valid configuration key paths
// from a struct type by reading mapstructure tags. This is used to validate
// that all bound keys match actual struct fields.
func collectValidConfigKeys(t reflect.Type, prefix string) []string {
	var keys []string

	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	// Only process structs
	if t.Kind() != reflect.Struct {
		return keys
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get mapstructure tag (Viper uses mapstructure for field mapping)
		tag := field.Tag.Get("mapstructure")
		if tag == "" || tag == "-" {
			continue
		}

		// Build full key path
		fullKey := tag
		if prefix != "" {
			fullKey = prefix + "." + tag
		}

		// If field is a struct, recursively collect its keys
		fieldType := field.Type
		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
		}

		if fieldType.Kind() == reflect.Struct {
			// Recursively collect nested keys
			keys = append(keys, collectValidConfigKeys(fieldType, fullKey)...)
		} else {
			// Leaf field - add the key
			keys = append(keys, fullKey)
		}
	}

	return keys
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
