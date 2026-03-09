package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
)

// ApplicationConfig holds all application configuration
// Follows HyperFleet Configuration Standard with validation and structured marshaling
type ApplicationConfig struct {
	Server   *ServerConfig              `mapstructure:"server" json:"server" validate:"required"`
	Metrics  *MetricsConfig             `mapstructure:"metrics" json:"metrics" validate:"required"`
	Health   *HealthConfig              `mapstructure:"health" json:"health" validate:"required"`
	Database *DatabaseConfig            `mapstructure:"database" json:"database" validate:"required"`
	OCM      *OCMConfig                 `mapstructure:"ocm" json:"ocm" validate:"required"`
	Logging  *LoggingConfig             `mapstructure:"logging" json:"logging" validate:"required"`
	Adapters *AdapterRequirementsConfig `mapstructure:"adapters" json:"adapters" validate:"required"`
}

// NewApplicationConfig returns default ApplicationConfig with all sub-configs initialized
// These defaults can be overridden by config file, env vars, or CLI flags
func NewApplicationConfig() *ApplicationConfig {
	return &ApplicationConfig{
		Server:   NewServerConfig(),
		Metrics:  NewMetricsConfig(),
		Health:   NewHealthConfig(),
		Database: NewDatabaseConfig(),
		OCM:      NewOCMConfig(),
		Logging:  NewLoggingConfig(),
		Adapters: NewAdapterRequirementsConfig(),
	}
}

// ============================================================
// BACKWARD COMPATIBILITY METHODS
// These methods maintain compatibility with old code
// They are deprecated and will be removed after full migration
// ============================================================

// AddFlags adds configuration flags using the old interface
//
// Deprecated: Use config.AddAllConfigFlags(cmd) instead
func (c *ApplicationConfig) AddFlags(flagset interface{}) {
	// This method is called by environments/framework.go for the old config system
	// It's a no-op since flags are added by AddAllConfigFlags in servecmd
	// Flag values are populated by LoadFromFlags() after parsing
}

// LoadFromFlags reads flag values from the FlagSet and populates the config struct
// This is used by the old config system to read flag values after cobra parses them
func (c *ApplicationConfig) LoadFromFlags(flags *pflag.FlagSet) error {
	// Database flags
	if v, err := flags.GetString("db-host"); err == nil && flags.Changed("db-host") {
		c.Database.Host = v
	}
	if v, err := flags.GetInt("db-port"); err == nil && flags.Changed("db-port") {
		c.Database.Port = v
	}
	if v, err := flags.GetString("db-username"); err == nil && flags.Changed("db-username") {
		c.Database.Username = v
	}
	if v, err := flags.GetString("db-password"); err == nil && flags.Changed("db-password") {
		c.Database.Password = v
	}
	if v, err := flags.GetString("db-name"); err == nil && flags.Changed("db-name") {
		c.Database.Name = v
	}
	if v, err := flags.GetString("db-ssl-mode"); err == nil && flags.Changed("db-ssl-mode") {
		c.Database.SSL.Mode = v
	}
	if v, err := flags.GetBool("db-debug"); err == nil && flags.Changed("db-debug") {
		c.Database.Debug = v
	}
	if v, err := flags.GetInt("db-max-open-connections"); err == nil && flags.Changed("db-max-open-connections") {
		c.Database.Pool.MaxConnections = v
	}
	// Server flags
	if v, err := flags.GetBool("server-jwt-enabled"); err == nil && flags.Changed("server-jwt-enabled") {
		c.Server.JWT.Enabled = v
	}
	if v, err := flags.GetBool("server-authz-enabled"); err == nil && flags.Changed("server-authz-enabled") {
		c.Server.Authz.Enabled = v
	}
	return nil
}

// ReadFiles loads configuration from files using the old interface
//
// Deprecated: Configuration loading is now handled by ConfigLoader
func (c *ApplicationConfig) ReadFiles() []string {
	// This method is called by environments/framework.go
	// In the new system, file loading is handled by ConfigLoader
	if IsNewConfigEnabled() {
		return []string{} // New system handles file loading
	}

	// Old system: read from files
	var messages []string
	if err := c.Database.ReadFiles(); err != nil {
		messages = append(messages, fmt.Sprintf("Database.ReadFiles: %s", err.Error()))
	}
	// Other config sections don't have ReadFiles() methods yet
	return messages
}

// LoadAdapters initializes adapter configuration using the old interface
//
// Deprecated: Adapters are now loaded via Viper in the new system
func (c *ApplicationConfig) LoadAdapters() error {
	// This method is called by environments/framework.go
	// In the old system, adapters were loaded from env vars
	// In the new system, they're loaded via Viper
	// If using old system, load from env vars; if new system, already loaded
	if IsNewConfigEnabled() {
		// Already loaded by ConfigLoader
		return nil
	}

	// Old system: load from environment variables
	clusterAdapters := os.Getenv("HYPERFLEET_CLUSTER_ADAPTERS")
	nodepoolAdapters := os.Getenv("HYPERFLEET_NODEPOOL_ADAPTERS")

	// If neither is configured, return early (this is OK for some environments)
	if clusterAdapters == "" && nodepoolAdapters == "" {
		return nil
	}

	// Initialize adapters config before any assignment
	if c.Adapters == nil {
		c.Adapters = NewAdapterRequirementsConfig()
	}

	// Parse cluster adapters if provided
	if clusterAdapters != "" {
		var cluster []string
		if err := json.Unmarshal([]byte(clusterAdapters), &cluster); err != nil {
			return fmt.Errorf("failed to parse HYPERFLEET_CLUSTER_ADAPTERS: %w", err)
		}
		c.Adapters.Required.Cluster = cluster
	}

	// Parse nodepool adapters if provided
	if nodepoolAdapters != "" {
		var nodepool []string
		if err := json.Unmarshal([]byte(nodepoolAdapters), &nodepool); err != nil {
			return fmt.Errorf("failed to parse HYPERFLEET_NODEPOOL_ADAPTERS: %w", err)
		}
		c.Adapters.Required.Nodepool = nodepool
	}

	return nil
}

// Read the contents of file into integer value
func readFileValueInt(file string, val *int) error {
	fileContents, err := ReadFile(file)
	if err != nil || fileContents == "" {
		return err
	}

	*val, err = strconv.Atoi(fileContents)
	return err
}

// Read the contents of file into string value
func readFileValueString(file string, val *string) error {
	fileContents, err := ReadFile(file)
	if err != nil || fileContents == "" {
		return err
	}

	*val = strings.TrimSuffix(fileContents, "\n")
	return err
}

// Read the contents of file into boolean value
func readFileValueBool(file string, val *bool) error {
	fileContents, err := ReadFile(file)
	if err != nil || fileContents == "" {
		return err
	}

	*val, err = strconv.ParseBool(fileContents)
	return err
}

func ReadFile(file string) (string, error) {
	// If the value is in quotes, unquote it
	unquotedFile, err := strconv.Unquote(file)
	if err != nil {
		// values without quotes will raise an error, ignore it.
		unquotedFile = file
	}

	// If no file is provided, leave val unchanged.
	if unquotedFile == "" {
		return "", nil
	}

	// Resolve relative paths from the current working directory
	absFilePath := unquotedFile
	if !filepath.IsAbs(unquotedFile) {
		wd, wdErr := os.Getwd()
		if wdErr != nil {
			return "", fmt.Errorf("failed to get working directory: %w", wdErr)
		}
		absFilePath = filepath.Join(wd, unquotedFile)
	}

	// Read the file
	buf, err := os.ReadFile(absFilePath) //nolint:gosec // File path is controlled by config file settings
	if err != nil {
		return "", err
	}
	return string(buf), nil
}
