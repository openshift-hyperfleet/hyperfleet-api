package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"gorm.io/gorm/logger"
)

const (
	// RedactedValue is used to mask sensitive data in logs and JSON output
	// Must match pkg/middleware/masking.go RedactedValue for consistency
	RedactedValue = "***REDACTED***"
)

// DatabaseConfig holds database connection configuration
// Follows HyperFleet Configuration Standard
type DatabaseConfig struct {
	Dialect  string     `mapstructure:"dialect" json:"dialect" validate:"required,oneof=postgres"`
	Host     string     `mapstructure:"host" json:"host" validate:"required,hostname|ip"`
	Port     int        `mapstructure:"port" json:"port" validate:"required,min=1,max=65535"`
	Name     string     `mapstructure:"name" json:"name" validate:"required"`
	Username string     `mapstructure:"username" json:"-"` // Excluded from JSON marshaling (sensitive)
	Password string     `mapstructure:"password" json:"-"` // Excluded from JSON marshaling (sensitive)
	Debug    bool       `mapstructure:"debug" json:"debug"`
	SSL      SSLConfig  `mapstructure:"ssl" json:"ssl" validate:"required"`
	Pool     PoolConfig `mapstructure:"pool" json:"pool" validate:"required"`
}

// SSLConfig holds SSL/TLS configuration
type SSLConfig struct {
	Mode         string `mapstructure:"mode" json:"mode" validate:"required,oneof=disable require verify-ca verify-full"`
	RootCertFile string `mapstructure:"root_cert_file" json:"root_cert_file"`
}

// PoolConfig holds connection pool configuration
type PoolConfig struct {
	MaxConnections int `mapstructure:"max_connections" json:"max_connections" validate:"required,min=1,max=200"`
}

// MarshalJSON implements custom JSON marshaling to redact sensitive fields
func (c DatabaseConfig) MarshalJSON() ([]byte, error) {
	type Alias DatabaseConfig
	return json.Marshal(&struct {
		Username string `json:"username"`
		Password string `json:"password"`
		*Alias
	}{
		Username: redactIfSet(c.Username),
		Password: redactIfSet(c.Password),
		Alias:    (*Alias)(&c),
	})
}

// redactIfSet returns "**REDACTED**" if value is non-empty, otherwise empty string
func redactIfSet(value string) string {
	if value == "" {
		return ""
	}
	return RedactedValue
}

// NewDatabaseConfig returns default DatabaseConfig values
// These defaults can be overridden by config file, env vars, or CLI flags
func NewDatabaseConfig() *DatabaseConfig {
	return &DatabaseConfig{
		Dialect:  "postgres",
		Host:     "localhost",
		Port:     5432,
		Name:     "hyperfleet",
		Username: "hyperfleet",
		Password: "",
		Debug:    false,
		SSL: SSLConfig{
			Mode:         "disable",
			RootCertFile: "",
		},
		Pool: PoolConfig{
			MaxConnections: 50,
		},
	}
}

// ============================================================
// BACKWARD COMPATIBILITY HELPERS
// ============================================================

// SSLMode returns SSL mode (legacy accessor)
func (c *DatabaseConfig) SSLMode() string {
	return c.SSL.Mode
}

// MaxOpenConnections returns max open connections (legacy accessor)
func (c *DatabaseConfig) MaxOpenConnections() int {
	return c.Pool.MaxConnections
}

// RootCertFile returns root cert file (legacy accessor)
func (c *DatabaseConfig) RootCertFile() string {
	return c.SSL.RootCertFile
}

// ============================================================
// LEGACY METHODS (for old configuration system)
// ============================================================

// escapeDSNValue escapes a PostgreSQL DSN parameter value according to libpq rules.
// It escapes backslashes and single quotes, and wraps values containing spaces in single quotes.
func escapeDSNValue(value string) string {
	if value == "" {
		return value
	}

	// Escape backslashes first (must be done before escaping quotes)
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	// Escape single quotes
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)

	// Wrap in single quotes if the value contains spaces or special characters
	if strings.ContainsAny(escaped, " \t\n\r") {
		return fmt.Sprintf("'%s'", escaped)
	}

	return escaped
}

// ConnectionString generates database connection string
// ssl parameter controls whether to include SSL settings
func (c *DatabaseConfig) ConnectionString(ssl bool) string {
	var params []string

	if ssl && c.SSL.Mode != disable {
		params = append(params, fmt.Sprintf("sslmode=%s", c.SSL.Mode))

		if c.SSL.RootCertFile != "" {
			params = append(params, fmt.Sprintf("sslrootcert=%s", escapeDSNValue(c.SSL.RootCertFile)))
		}
	} else {
		params = append(params, "sslmode=disable")
	}

	return fmt.Sprintf(
		"host=%s port=%d dbname=%s user=%s password=%s %s",
		escapeDSNValue(c.Host),
		c.Port,
		escapeDSNValue(c.Name),
		escapeDSNValue(c.Username),
		escapeDSNValue(c.Password),
		strings.Join(params, " "),
	)
}

// LogSafeConnectionString returns connection string with username and password redacted
func (c *DatabaseConfig) LogSafeConnectionString(ssl bool) string {
	tempConfig := *c
	tempConfig.Username = RedactedValue
	tempConfig.Password = RedactedValue
	return tempConfig.ConnectionString(ssl)
}

// ConnectionStringWithName generates database connection string with custom database name
// This is useful for test databases. ssl parameter controls whether to include SSL settings.
func (c *DatabaseConfig) ConnectionStringWithName(name string, ssl bool) string {
	tempConfig := *c
	tempConfig.Name = name
	return tempConfig.ConnectionString(ssl)
}

// LogSafeConnectionStringWithName returns connection string with custom name and username/password redacted
// This is useful for test databases.
func (c *DatabaseConfig) LogSafeConnectionStringWithName(name string, ssl bool) string {
	tempConfig := *c
	tempConfig.Name = name
	tempConfig.Username = RedactedValue
	tempConfig.Password = RedactedValue
	return tempConfig.ConnectionString(ssl)
}

const disable = "disable"

// ReadFiles reads database configuration from files (legacy method for old system)
// In new system, file reading is handled by ConfigLoader
func (c *DatabaseConfig) ReadFiles() error {
	// This method is kept for backward compatibility with old system
	// In new system, ConfigLoader handles file reading
	if IsNewConfigEnabled() {
		return nil
	}

	// Old system: read from environment variables with _FILE suffix
	if err := readFileValueString(os.Getenv("DB_HOST_FILE"), &c.Host); err != nil {
		return err
	}
	if err := readFileValueInt(os.Getenv("DB_PORT_FILE"), &c.Port); err != nil {
		return err
	}
	if err := readFileValueString(os.Getenv("DB_USERNAME_FILE"), &c.Username); err != nil {
		return err
	}
	if err := readFileValueString(os.Getenv("DB_PASSWORD_FILE"), &c.Password); err != nil {
		return err
	}
	if err := readFileValueString(os.Getenv("DB_NAME_FILE"), &c.Name); err != nil {
		return err
	}
	if err := readFileValueString(os.Getenv("DB_ROOTCERT_FILE"), &c.SSL.RootCertFile); err != nil {
		return err
	}

	return nil
}


// SetLogLevel sets GORM logger level based on Debug flag
// This is called during database initialization
func (c *DatabaseConfig) SetLogLevel() logger.LogLevel {
	if c.Debug {
		return logger.Info
	}
	return logger.Warn
}
