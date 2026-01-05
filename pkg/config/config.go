package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

const (
	EnvPrefix             = "HYPERFLEET"
	DefaultConfigFileProd = "/etc/hyperfleet/config.yaml"
	DefaultConfigFileDev  = "./configs/config.yaml"
	ConfigEnvVar          = "HYPERFLEET_CONFIG"
)

// NewCommandConfig creates and configures a new Viper instance for a command
// Each command should have its own viper instance to avoid configuration pollution
func NewCommandConfig() *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()
	return v
}

// Flag definition helpers that define flags and bind them to viper keys in a single pass

func defineAndBindStringFlag(v *viper.Viper, fs *pflag.FlagSet, viperKey, flagName, shorthand, defaultVal, usage string) {
	// Define the flag
	if shorthand != "" {
		fs.StringP(flagName, shorthand, defaultVal, usage)
	} else {
		fs.String(flagName, defaultVal, usage)
	}
	// Bind to viper key
	bindFlag(v, fs, viperKey, flagName)
}

func defineAndBindIntFlag(v *viper.Viper, fs *pflag.FlagSet, viperKey, flagName, shorthand string, defaultVal int, usage string) {
	// Define the flag
	if shorthand != "" {
		fs.IntP(flagName, shorthand, defaultVal, usage)
	} else {
		fs.Int(flagName, defaultVal, usage)
	}
	// Bind to viper key
	bindFlag(v, fs, viperKey, flagName)
}

func defineAndBindBoolFlag(v *viper.Viper, fs *pflag.FlagSet, viperKey, flagName, shorthand string, defaultVal bool, usage string) {
	// Define the flag
	if shorthand != "" {
		fs.BoolP(flagName, shorthand, defaultVal, usage)
	} else {
		fs.Bool(flagName, defaultVal, usage)
	}
	// Bind to viper key
	bindFlag(v, fs, viperKey, flagName)
}

func defineAndBindDurationFlag(v *viper.Viper, fs *pflag.FlagSet, viperKey, flagName, shorthand string, defaultVal time.Duration, usage string) {
	// Define the flag
	if shorthand != "" {
		fs.DurationP(flagName, shorthand, defaultVal, usage)
	} else {
		fs.Duration(flagName, defaultVal, usage)
	}
	// Bind to viper key
	bindFlag(v, fs, viperKey, flagName)
}

type ApplicationConfig struct {
	App         *AppConfig         `mapstructure:"app" json:"app" validate:"required"`
	Server      *ServerConfig      `mapstructure:"server" json:"server" validate:"required"`
	Metrics     *MetricsConfig     `mapstructure:"metrics" json:"metrics" validate:"required"`
	HealthCheck *HealthCheckConfig `mapstructure:"health_check" json:"health_check" validate:"required"`
	Database    *DatabaseConfig    `mapstructure:"database" json:"database" validate:"required"`
	OCM         *OCMConfig         `mapstructure:"ocm" json:"ocm" validate:"required"`
}

func NewApplicationConfig() *ApplicationConfig {
	return &ApplicationConfig{
		App:         NewAppConfig(),
		Server:      NewServerConfig(),
		Metrics:     NewMetricsConfig(),
		HealthCheck: NewHealthCheckConfig(),
		Database:    NewDatabaseConfig(),
		OCM:         NewOCMConfig(),
	}
}

// defineAndBindFlags defines application flags and binds them to viper keys in a single pass
func (c *ApplicationConfig) defineAndBindFlags(v *viper.Viper, flagset *pflag.FlagSet) {
	// Global flags
	// Note: config flag is defined but NOT bound to viper (special case)
	flagset.String("config", "", "Config file path")

	// Define and bind sub-config flags
	c.App.defineAndBindFlags(v, flagset)
	c.Server.defineAndBindFlags(v, flagset)
	c.Metrics.defineAndBindFlags(v, flagset)
	c.HealthCheck.defineAndBindFlags(v, flagset)
	c.Database.defineAndBindFlags(v, flagset)
	c.OCM.defineAndBindFlags(v, flagset)
}

// ConfigureFlags defines configuration flags and binds them to viper for precedence handling
func (c *ApplicationConfig) ConfigureFlags(v *viper.Viper, flagset *pflag.FlagSet) {
	flagset.AddGoFlagSet(flag.CommandLine)
	c.defineAndBindFlags(v, flagset)
}

// bindFlag is a simple helper to bind an existing flag to a viper key
func bindFlag(v *viper.Viper, fs *pflag.FlagSet, viperKey, flagName string) {
	if err := v.BindPFlag(viperKey, fs.Lookup(flagName)); err != nil {
		panic(fmt.Sprintf("failed to bind flag %s to %s: %v", flagName, viperKey, err))
	}
}

// LoadConfig loads configuration from multiple sources with proper precedence:
// 1. Command-line flags (highest priority)
// 2. Environment variables (HYPERFLEET_ prefix)
// 3. Configuration files
// 4. Defaults (lowest priority)
//
// The viper instance should already be configured and have flags bound via ConfigureFlags()
func LoadConfig(v *viper.Viper, flags *pflag.FlagSet) (*ApplicationConfig, error) {
	// Create config instance
	// Note: Viper is already configured with env support and flags are already bound
	config := NewApplicationConfig()

	// Determine config file path
	configFile := getConfigFilePath(flags, v)

	// Load config file if it exists
	if configFile != "" {
		v.SetConfigFile(configFile)
		if err := v.ReadInConfig(); err != nil {
			if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
				return nil, fmt.Errorf("error reading config file %s: %w", configFile, err)
			}
			glog.Infof("Config file not found: %s, continuing with flags and environment variables", configFile)
		} else {
			glog.Infof("Loaded configuration from: %s", configFile)
		}
	}

	// Unmarshal into config struct
	// Viper now contains: config file values < env vars < bound flags
	// This gives us the correct precedence automatically
	if err := v.UnmarshalExact(config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// getConfigFilePath determines the config file path based on precedence:
// 1. --config flag
// 2. HYPERFLEET_CONFIG environment variable
// 3. Default paths
func getConfigFilePath(flags *pflag.FlagSet, v *viper.Viper) string {
	// Check --config flag first
	if flags != nil {
		if configFlag := flags.Lookup("config"); configFlag != nil && configFlag.Changed {
			return configFlag.Value.String()
		}
	}

	// Check environment variable
	if configEnv := os.Getenv(ConfigEnvVar); configEnv != "" {
		return configEnv
	}

	// Try default paths in order
	defaultPaths := []string{
		DefaultConfigFileDev,  // Try development path first
		DefaultConfigFileProd, // Then production path
	}

	for _, path := range defaultPaths {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}

	// No config file found
	return ""
}

// Validate validates the configuration using struct tags
func (c *ApplicationConfig) Validate() error {
	validate := validator.New()

	if err := validate.Struct(c); err != nil {
		return formatValidationError(err)
	}

	return nil
}

// formatValidationError formats validation errors following the HyperFleet standard
func formatValidationError(err error) error {
	if validationErrors, ok := err.(validator.ValidationErrors); ok {
		var messages []string
		messages = append(messages, "Configuration validation failed:")

		for _, fieldError := range validationErrors {
			fieldPath := getFieldPath(fieldError)

			msg := fmt.Sprintf("  - Field '%s' failed validation: %s", fieldPath, fieldError.Tag())

			if fieldError.Param() != "" {
				msg += fmt.Sprintf(" (param: %s)", fieldError.Param())
			}

			msg += fmt.Sprintf("\n    Value: %v", fieldError.Value())
			msg += getHelpfulHint(fieldPath, fieldError.Tag())

			messages = append(messages, msg)
		}

		return fmt.Errorf("%s", strings.Join(messages, "\n"))
	}

	return err
}

// getFieldPath extracts the full field path from a validation error
func getFieldPath(fieldError validator.FieldError) string {
	namespace := fieldError.Namespace()
	// Remove the root struct name (ApplicationConfig)
	parts := strings.Split(namespace, ".")
	if len(parts) > 1 {
		return "Config." + strings.Join(parts[1:], ".")
	}
	return namespace
}

// getHelpfulHint provides hints for how to fix validation errors
func getHelpfulHint(fieldPath, tag string) string {
	// Convert field path to flag and env var names
	// E.g., "Config.App.Name" -> "app.name"
	parts := strings.Split(fieldPath, ".")
	if len(parts) <= 1 {
		return ""
	}

	// Remove "Config" prefix
	configParts := parts[1:]

	// Convert to lowercase and join with dots
	var lowerParts []string
	for _, part := range configParts {
		lowerParts = append(lowerParts, strings.ToLower(part))
	}
	configPath := strings.Join(lowerParts, ".")

	// Create flag name (replace dots and underscores with hyphens)
	flagName := "--" + strings.ReplaceAll(strings.ReplaceAll(configPath, ".", "-"), "_", "-")

	// Create env var name (uppercase, replace dots with underscores)
	envVarName := EnvPrefix + "_" + strings.ToUpper(strings.ReplaceAll(configPath, ".", "_"))

	hint := "\n    Please provide via:\n"
	hint += fmt.Sprintf("      • Flag: %s\n", flagName)
	hint += fmt.Sprintf("      • Environment variable: %s\n", envVarName)
	hint += fmt.Sprintf("      • Config file: %s", configPath)

	return hint
}

// DisplayConfig logs the merged configuration at startup
// Sensitive values are redacted
func (c *ApplicationConfig) DisplayConfig() {
	glog.Info("=== Merged Configuration ===")

	// Create a copy for display with sensitive values redacted
	displayCopy := c.redactSensitiveValues()

	// Convert to JSON for pretty display
	jsonBytes, err := json.MarshalIndent(displayCopy, "", "  ")
	if err != nil {
		glog.Errorf("Error marshaling config for display: %v", err)
		return
	}

	glog.Infof("\n%s", string(jsonBytes))
	glog.Info("============================")
}

// redactSensitiveValues creates a copy of the config with sensitive values redacted
// It uses reflection to automatically redact any field whose name contains
// sensitive keywords (password, secret, token, key, cert)
func (c *ApplicationConfig) redactSensitiveValues() *ApplicationConfig {
	// Marshal to JSON and back to create a deep copy
	jsonBytes, err := json.Marshal(c)
	if err != nil {
		glog.Errorf("Error marshaling config for redaction: %v", err)
		return c
	}

	var copy ApplicationConfig
	if err := json.Unmarshal(jsonBytes, &copy); err != nil {
		glog.Errorf("Error unmarshaling config for redaction: %v", err)
		return c
	}

	// Recursively redact sensitive fields
	redactSensitiveFields(reflect.ValueOf(&copy).Elem())

	return &copy
}

// redactSensitiveFields recursively walks through a struct and redacts
// any string field whose name matches sensitive patterns
func redactSensitiveFields(v reflect.Value) {
	if !v.IsValid() {
		return
	}

	switch v.Kind() {
	case reflect.Ptr:
		if !v.IsNil() {
			redactSensitiveFields(v.Elem())
		}

	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			field := v.Field(i)
			fieldType := t.Field(i)

			// Skip unexported fields
			if !field.CanSet() {
				continue
			}

			// Check if this field is sensitive
			if isSensitiveField(fieldType.Name) {
				// Redact string fields
				if field.Kind() == reflect.String && field.String() != "" {
					field.SetString("***")
				}
			} else {
				// Recursively process nested structs and pointers
				redactSensitiveFields(field)
			}
		}

	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			redactSensitiveFields(v.Index(i))
		}

	case reflect.Map:
		for _, key := range v.MapKeys() {
			mapValue := v.MapIndex(key)
			if mapValue.Kind() == reflect.Ptr || mapValue.Kind() == reflect.Struct {
				redactSensitiveFields(mapValue)
			}
		}
	}
}

// GetJSONConfig returns the configuration as a JSON string
// Sensitive values are redacted
func (c *ApplicationConfig) GetJSONConfig() (string, error) {
	displayCopy := c.redactSensitiveValues()

	jsonBytes, err := json.MarshalIndent(displayCopy, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshaling config to JSON: %w", err)
	}

	return string(jsonBytes), nil
}

// isSensitiveField checks if a field name contains sensitive data keywords
func isSensitiveField(fieldName string) bool {
	sensitiveFields := []string{
		"password", "secret", "token", "key", "cert",
	}

	lowerName := strings.ToLower(fieldName)
	for _, sensitive := range sensitiveFields {
		if strings.Contains(lowerName, sensitive) {
			return true
		}
	}

	return false
}
