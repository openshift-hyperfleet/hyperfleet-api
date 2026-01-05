package config

import (
	"encoding/json"
	"flag"
	"fmt"
	"reflect"

	"github.com/go-playground/validator/v10"
	"github.com/golang/glog"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

// MigrateConfig holds configuration for the migrate command
// Only requires App and Database configuration
type MigrateConfig struct {
	App      *AppConfig      `mapstructure:"app" json:"app" validate:"required"`
	Database *DatabaseConfig `mapstructure:"database" json:"database" validate:"required"`
}

// NewMigrateConfig creates a new MigrateConfig with default values
func NewMigrateConfig() *MigrateConfig {
	return &MigrateConfig{
		App:      NewAppConfig(),
		Database: NewDatabaseConfig(),
	}
}

// defineAndBindFlags defines migrate command flags and binds them to viper keys in a single pass
func (c *MigrateConfig) defineAndBindFlags(v *viper.Viper, flagset *pflag.FlagSet) {
	// Global flags
	// Note: config flag is defined but NOT bound to viper (special case)
	flagset.String("config", "", "Config file path")

	// Define and bind sub-config flags (only App and Database for migrate)
	c.App.defineAndBindFlags(v, flagset)
	c.Database.defineAndBindFlags(v, flagset)
}

// ConfigureFlags defines configuration flags and binds them to viper for precedence handling
func (c *MigrateConfig) ConfigureFlags(v *viper.Viper, flagset *pflag.FlagSet) {
	flagset.AddGoFlagSet(flag.CommandLine)
	c.defineAndBindFlags(v, flagset)
}

// LoadMigrateConfig loads configuration for the migrate command from multiple sources with proper precedence:
// 1. Command-line flags (highest priority)
// 2. Environment variables (HYPERFLEET_ prefix)
// 3. Configuration files
// 4. Defaults (lowest priority)
//
// The viper instance should already be configured and have flags bound via ConfigureFlags()
func LoadMigrateConfig(v *viper.Viper, flags *pflag.FlagSet) (*MigrateConfig, error) {
	// Create config instance
	// Note: Viper is already configured with env support and flags are already bound
	config := NewMigrateConfig()

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
	// Note: Using Unmarshal (not UnmarshalExact) to allow extra fields in config file
	// (e.g., shared config files with server, metrics, etc. will be ignored)
	if err := v.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// Validate validates the migrate configuration using struct tags
// Only App and Database are required for migrate command
func (c *MigrateConfig) Validate() error {
	validate := validator.New()

	if err := validate.Struct(c); err != nil {
		return formatValidationError(err)
	}

	return nil
}

// DisplayConfig logs the merged configuration at startup
// Sensitive values are redacted
func (c *MigrateConfig) DisplayConfig() {
	glog.Info("=== Merged Configuration (Migrate Command) ===")

	// Create a copy for display with sensitive values redacted
	displayCopy := c.redactSensitiveValues()

	// Convert to JSON for pretty display
	jsonBytes, err := json.MarshalIndent(displayCopy, "", "  ")
	if err != nil {
		glog.Errorf("Error marshaling config for display: %v", err)
		return
	}

	glog.Infof("\n%s", string(jsonBytes))
	glog.Info("===============================================")
}

// GetJSONConfig returns the configuration as a JSON string
// Sensitive values are redacted
func (c *MigrateConfig) GetJSONConfig() (string, error) {
	displayCopy := c.redactSensitiveValues()

	jsonBytes, err := json.MarshalIndent(displayCopy, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshaling config to JSON: %w", err)
	}

	return string(jsonBytes), nil
}

// redactSensitiveValues creates a copy of the config with sensitive values redacted
func (c *MigrateConfig) redactSensitiveValues() *MigrateConfig {
	// Marshal to JSON and back to create a deep copy
	jsonBytes, err := json.Marshal(c)
	if err != nil {
		glog.Errorf("Error marshaling config for redaction: %v", err)
		return c
	}

	var copy MigrateConfig
	if err := json.Unmarshal(jsonBytes, &copy); err != nil {
		glog.Errorf("Error unmarshaling config for redaction: %v", err)
		return c
	}

	// Recursively redact sensitive fields using the shared function from config.go
	redactSensitiveFields(reflect.ValueOf(&copy).Elem())

	return &copy
}
