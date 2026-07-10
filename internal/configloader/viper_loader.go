package configloader

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// EnvPrefix is the prefix for all environment variables that override deployment config
const EnvPrefix = "HYPERFLEET"

// viperKeyMappings defines mappings from config paths to env variable suffixes
// The full env var name is EnvPrefix + "_" + suffix
// Note: Uses "::" as key delimiter to avoid conflicts with dots in YAML keys
var viperKeyMappings = map[string]string{
	"debug_config":                                     "DEBUG_CONFIG",
	"clients::maestro::grpc_server_address":            "MAESTRO_GRPC_SERVER_ADDRESS",
	"clients::maestro::http_server_address":            "MAESTRO_HTTP_SERVER_ADDRESS",
	"clients::maestro::source_id":                      "MAESTRO_SOURCE_ID",
	"clients::maestro::client_id":                      "MAESTRO_CLIENT_ID",
	"clients::maestro::auth::type":                     "MAESTRO_AUTH_TYPE",
	"clients::maestro::auth::tls_config::ca_file":      "MAESTRO_CA_FILE",
	"clients::maestro::auth::tls_config::cert_file":    "MAESTRO_CERT_FILE",
	"clients::maestro::auth::tls_config::key_file":     "MAESTRO_KEY_FILE",
	"clients::maestro::auth::tls_config::http_ca_file": "MAESTRO_HTTP_CA_FILE",
	"clients::maestro::timeout":                        "MAESTRO_TIMEOUT",
	"clients::maestro::server_healthiness_timeout":     "MAESTRO_SERVER_HEALTHINESS_TIMEOUT",
	"clients::maestro::retry_attempts":                 "MAESTRO_RETRY_ATTEMPTS",
	"clients::maestro::keepalive::time":                "MAESTRO_KEEPALIVE_TIME",
	"clients::maestro::keepalive::timeout":             "MAESTRO_KEEPALIVE_TIMEOUT",
	"clients::maestro::insecure":                       "MAESTRO_INSECURE",
	"clients::hyperfleet_api::base_url":                "API_BASE_URL",
	"clients::hyperfleet_api::version":                 "API_VERSION",
	"clients::hyperfleet_api::timeout":                 "API_TIMEOUT",
	"clients::hyperfleet_api::retry_attempts":          "API_RETRY_ATTEMPTS",
	"clients::hyperfleet_api::retry_backoff":           "API_RETRY_BACKOFF",
	"clients::hyperfleet_api::base_delay":              "API_BASE_DELAY",
	"clients::hyperfleet_api::max_delay":               "API_MAX_DELAY",
	"clients::hyperfleet_api::auth::token_path":        "API_AUTH_TOKEN_PATH",
	"clients::hyperfleet_api::auth::token_cache_ttl":   "API_AUTH_TOKEN_CACHE_TTL",
	"clients::broker::subscription_id":                 "BROKER_SUBSCRIPTION_ID",
	"clients::broker::topic":                           "BROKER_TOPIC",
	"clients::kubernetes::kube_config_path":            "KUBERNETES_KUBE_CONFIG_PATH",
	"clients::kubernetes::api_version":                 "KUBERNETES_API_VERSION",
	"clients::kubernetes::qps":                         "KUBERNETES_QPS",
	"clients::kubernetes::burst":                       "KUBERNETES_BURST",
}

// cliFlags defines mappings from CLI flag names to config paths
// Note: Uses "::" as key delimiter to avoid conflicts with dots in YAML keys
var cliFlags = map[string]string{
	"debug-config":                       "debug_config",
	"maestro-grpc-server-address":        "clients::maestro::grpc_server_address",
	"maestro-http-server-address":        "clients::maestro::http_server_address",
	"maestro-source-id":                  "clients::maestro::source_id",
	"maestro-client-id":                  "clients::maestro::client_id",
	"maestro-auth-type":                  "clients::maestro::auth::type",
	"maestro-ca-file":                    "clients::maestro::auth::tls_config::ca_file",
	"maestro-cert-file":                  "clients::maestro::auth::tls_config::cert_file",
	"maestro-key-file":                   "clients::maestro::auth::tls_config::key_file",
	"maestro-http-ca-file":               "clients::maestro::auth::tls_config::http_ca_file",
	"maestro-timeout":                    "clients::maestro::timeout",
	"maestro-server-healthiness-timeout": "clients::maestro::server_healthiness_timeout",
	"maestro-retry-attempts":             "clients::maestro::retry_attempts",
	"maestro-keepalive-time":             "clients::maestro::keepalive::time",
	"maestro-keepalive-timeout":          "clients::maestro::keepalive::timeout",
	"maestro-insecure":                   "clients::maestro::insecure",
	"hyperfleet-api-base-url":            "clients::hyperfleet_api::base_url",
	"hyperfleet-api-version":             "clients::hyperfleet_api::version",
	"hyperfleet-api-timeout":             "clients::hyperfleet_api::timeout",
	"hyperfleet-api-retry":               "clients::hyperfleet_api::retry_attempts",
	"hyperfleet-api-retry-backoff":       "clients::hyperfleet_api::retry_backoff",
	"hyperfleet-api-base-delay":          "clients::hyperfleet_api::base_delay",
	"hyperfleet-api-max-delay":           "clients::hyperfleet_api::max_delay",
	"broker-subscription-id":             "clients::broker::subscription_id",
	"broker-topic":                       "clients::broker::topic",
	"kubernetes-kube-config-path":        "clients::kubernetes::kube_config_path",
	"kubernetes-api-version":             "clients::kubernetes::api_version",
	"kubernetes-qps":                     "clients::kubernetes::qps",
	"kubernetes-burst":                   "clients::kubernetes::burst",
	"log-level":                          "log::level",
	"log-format":                         "log::format",
	"log-output":                         "log::output",
}

// standardConfigPaths are tried when no explicit config path is provided
var standardConfigPaths = []string{
	"/etc/hyperfleet/config.yaml", // production
	"./configs/config.yaml",       // development
}

// loadAdapterConfigWithViper loads the deployment configuration from a YAML file
// with environment variable and CLI flag overrides using Viper.
// Priority: CLI flags > Environment variables > Config file > Defaults
// Returns the resolved config file path alongside the loaded config.
func loadAdapterConfigWithViper(
	filePath string,
	flags *pflag.FlagSet,
) (string, *AdapterConfig, error) {
	// Use "::" as key delimiter to avoid conflicts with dots in YAML keys
	// (e.g., "hyperfleet.io/component" in metadata.labels)
	v := viper.NewWithOptions(viper.KeyDelimiter("::"))

	// Set config file path
	if filePath == "" {
		filePath = os.Getenv(EnvAdapterConfig)
	}

	// Try standard paths if no path configured
	if filePath == "" {
		for _, p := range standardConfigPaths {
			if _, err := os.Stat(p); err == nil {
				filePath = p
				break
			}
		}
	}

	if filePath == "" {
		return "", nil, fmt.Errorf(
			"adapter config file path is required (use --config flag or %s env var)",
			EnvAdapterConfig,
		)
	}

	// Read the YAML file first to get base configuration
	data, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return "", nil, fmt.Errorf("failed to read adapter config file %q: %w", filePath, err)
	}

	// Pre-validate YAML against the AdapterConfig struct to catch unknown fields.
	// KnownFields only works when decoding into a struct, not a map.
	preValidator := yaml.NewDecoder(bytes.NewReader(data))
	preValidator.KnownFields(true)
	var validateConfig AdapterConfig
	if err := preValidator.Decode(&validateConfig); err != nil {
		return "", nil, fmt.Errorf("failed to parse adapter config YAML: %w", err)
	}

	// Parse YAML into a map for Viper (env/CLI overrides are applied next)
	var configMap map[string]interface{}
	if err := yaml.Unmarshal(data, &configMap); err != nil {
		return "", nil, fmt.Errorf("failed to parse adapter config YAML: %w", err)
	}

	// Load the map into Viper
	if err := v.MergeConfigMap(configMap); err != nil {
		return "", nil, fmt.Errorf("failed to merge config map: %w", err)
	}

	// Bind environment variables
	v.SetEnvPrefix(EnvPrefix)
	v.AutomaticEnv()
	// Replace "::" (our key delimiter) and "-" with "_" for env var lookups
	v.SetEnvKeyReplacer(strings.NewReplacer("::", "_", "-", "_"))

	// Bind specific environment variables
	for configPath, envSuffix := range viperKeyMappings {
		envVar := EnvPrefix + "_" + envSuffix
		if val := os.Getenv(envVar); val != "" {
			v.Set(configPath, val)
		}
	}

	// Legacy broker env vars without HYPERFLEET_ prefix (kept for compatibility)
	if os.Getenv(EnvPrefix+"_BROKER_SUBSCRIPTION_ID") == "" {
		if val := os.Getenv("BROKER_SUBSCRIPTION_ID"); val != "" {
			v.Set("clients::broker::subscription_id", val)
		}
	}
	if os.Getenv(EnvPrefix+"_BROKER_TOPIC") == "" {
		if val := os.Getenv("BROKER_TOPIC"); val != "" {
			v.Set("clients::broker::topic", val)
		}
	}

	// Log env vars use LOG_ prefix without HYPERFLEET_ (consistent with serve mode)
	if val := os.Getenv("LOG_LEVEL"); val != "" {
		v.Set("log::level", strings.ToLower(val))
	}
	if val := os.Getenv("LOG_FORMAT"); val != "" {
		v.Set("log::format", strings.ToLower(val))
	}
	if val := os.Getenv("LOG_OUTPUT"); val != "" {
		v.Set("log::output", val)
	}

	// Bind CLI flags if provided
	if flags != nil {
		for flagName, configPath := range cliFlags {
			if flag := flags.Lookup(flagName); flag != nil && flag.Changed {
				v.Set(configPath, flag.Value.String())
			}
		}
	}

	// Unmarshal into AdapterConfig struct
	var config AdapterConfig
	if err := v.Unmarshal(&config); err != nil {
		return "", nil, fmt.Errorf("failed to unmarshal adapter config: %w", err)
	}

	return filePath, &config, nil
}

// loadTaskConfig loads the task configuration from a YAML file without Viper overrides.
// Task config is purely static YAML configuration.
func loadTaskConfig(filePath string) (*AdapterTaskConfig, error) {
	if filePath == "" {
		filePath = os.Getenv(EnvTaskConfigPath)
	}

	if filePath == "" {
		return nil, fmt.Errorf(
			"task config file path is required (use --task-config flag or %s env var)",
			EnvTaskConfigPath,
		)
	}

	data, err := os.ReadFile(filepath.Clean(filePath))
	if err != nil {
		return nil, fmt.Errorf("failed to read task config file %q: %w", filePath, err)
	}

	var config AdapterTaskConfig
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("failed to parse task config YAML: %w", err)
	}

	return &config, nil
}

// getBaseDir returns the base directory for a config file path
func getBaseDir(filePath string) (string, error) {
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path for %q: %w", filePath, err)
	}
	return filepath.Dir(absPath), nil
}

// loadAdapterConfigWithViperGeneric wraps loadAdapterConfigWithViper,
// binding CLI flags if provided and of correct type.
// Returns the resolved config file path alongside the loaded config.
func loadAdapterConfigWithViperGeneric(
	filePath string,
	flags interface{},
) (string, *AdapterConfig, error) {
	if pflags, ok := flags.(*pflag.FlagSet); ok && pflags != nil {
		return loadAdapterConfigWithViper(filePath, pflags)
	}
	return loadAdapterConfigWithViper(filePath, nil)
}
