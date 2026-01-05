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

// ServeConfig holds configuration for the serve command
// Requires all configuration sections (App, Server, Metrics, HealthCheck, Database, OCM)
type ServeConfig struct {
	App         *AppConfig         `mapstructure:"app" json:"app" validate:"required"`
	Server      *ServerConfig      `mapstructure:"server" json:"server" validate:"required"`
	Metrics     *MetricsConfig     `mapstructure:"metrics" json:"metrics" validate:"required"`
	HealthCheck *HealthCheckConfig `mapstructure:"health_check" json:"health_check" validate:"required"`
	Database    *DatabaseConfig    `mapstructure:"database" json:"database" validate:"required"`
	OCM         *OCMConfig         `mapstructure:"ocm" json:"ocm" validate:"required"`
}

// NewServeConfig creates a new ServeConfig with default values
func NewServeConfig() *ServeConfig {
	return &ServeConfig{
		App:         NewAppConfig(),
		Server:      NewServerConfig(),
		Metrics:     NewMetricsConfig(),
		HealthCheck: NewHealthCheckConfig(),
		Database:    NewDatabaseConfig(),
		OCM:         NewOCMConfig(),
	}
}

// defineAndBindFlags defines serve command flags and binds them to viper keys in a single pass
func (c *ServeConfig) defineAndBindFlags(v *viper.Viper, flagset *pflag.FlagSet) {
	// Global flags
	// Note: config flag is defined but NOT bound to viper (special case)
	flagset.String("config", "", "Config file path")

	// Define and bind sub-config flags (all required for serve)
	c.App.defineAndBindFlags(v, flagset)
	c.Server.defineAndBindFlags(v, flagset)
	c.Metrics.defineAndBindFlags(v, flagset)
	c.HealthCheck.defineAndBindFlags(v, flagset)
	c.Database.defineAndBindFlags(v, flagset)
	c.OCM.defineAndBindFlags(v, flagset)
}

// ConfigureFlags defines configuration flags and binds them to viper for precedence handling
func (c *ServeConfig) ConfigureFlags(v *viper.Viper, flagset *pflag.FlagSet) {
	flagset.AddGoFlagSet(flag.CommandLine)
	c.defineAndBindFlags(v, flagset)
}

// LoadServeConfig loads configuration for the serve command from multiple sources with proper precedence:
// 1. Command-line flags (highest priority)
// 2. Environment variables (HYPERFLEET_ prefix)
// 3. Configuration files
// 4. Defaults (lowest priority)
//
// The viper instance should already be configured and have flags bound via ConfigureFlags()
func LoadServeConfig(v *viper.Viper, flags *pflag.FlagSet) (*ServeConfig, error) {
	// Create config instance
	// Note: Viper is already configured with env support and flags are already bound
	config := NewServeConfig()

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

// Validate validates the serve configuration using struct tags
// All sections (App, Server, Metrics, HealthCheck, Database, OCM) are required for serve command
func (c *ServeConfig) Validate() error {
	validate := validator.New()

	if err := validate.Struct(c); err != nil {
		return formatValidationError(err)
	}

	return nil
}

// DisplayConfig logs the merged configuration at startup
// Sensitive values are redacted
func (c *ServeConfig) DisplayConfig() {
	glog.Info("=== Merged Configuration (Serve Command) ===")

	// Create a copy for display with sensitive values redacted
	displayCopy := c.redactSensitiveValues()

	// Convert to JSON for pretty display
	jsonBytes, err := json.MarshalIndent(displayCopy, "", "  ")
	if err != nil {
		glog.Errorf("Error marshaling config for display: %v", err)
		return
	}

	glog.Infof("\n%s", string(jsonBytes))
	glog.Info("============================================")
}

// GetJSONConfig returns the configuration as a JSON string
// Sensitive values are redacted
func (c *ServeConfig) GetJSONConfig() (string, error) {
	displayCopy := c.redactSensitiveValues()

	jsonBytes, err := json.MarshalIndent(displayCopy, "", "  ")
	if err != nil {
		return "", fmt.Errorf("error marshaling config to JSON: %w", err)
	}

	return string(jsonBytes), nil
}

// redactSensitiveValues creates a copy of the config with sensitive values redacted
func (c *ServeConfig) redactSensitiveValues() *ServeConfig {
	// Marshal to JSON and back to create a deep copy
	jsonBytes, err := json.Marshal(c)
	if err != nil {
		glog.Errorf("Error marshaling config for redaction: %v", err)
		return c
	}

	var copy ServeConfig
	if err := json.Unmarshal(jsonBytes, &copy); err != nil {
		glog.Errorf("Error unmarshaling config for redaction: %v", err)
		return c
	}

	// Recursively redact sensitive fields using the shared function from config.go
	redactSensitiveFields(reflect.ValueOf(&copy).Elem())

	return &copy
}

// ToApplicationConfig converts ServeConfig to ApplicationConfig for compatibility
// with existing code (e.g., environments.Initialize())
func (c *ServeConfig) ToApplicationConfig() *ApplicationConfig {
	return &ApplicationConfig{
		App:         c.App,
		Server:      c.Server,
		Metrics:     c.Metrics,
		HealthCheck: c.HealthCheck,
		Database:    c.Database,
		OCM:         c.OCM,
	}
}
