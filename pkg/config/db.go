package config

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm/logger"
)

const (
	// RedactedValue is used to mask sensitive data in logs and JSON output
	// Must match pkg/middleware/masking.go RedactedValue for consistency
	RedactedValue = "***REDACTED***"
)

// simpleDSNValuePattern matches PostgreSQL DSN simple values that don't need quoting.
// Per libpq spec, simple values contain only: letters (a-z, A-Z), digits (0-9), hyphens (-), and dots (.)
var simpleDSNValuePattern = regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)

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
// Includes fields from HYPERFLEET-694 for connection lifecycle management
type PoolConfig struct {
	MaxConnections int `mapstructure:"max_connections" json:"max_connections" validate:"required,min=1,max=200"`
	MaxIdleConnections int `mapstructure:"max_idle_connections" json:"max_idle_connections" validate:"min=0"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime" json:"conn_max_lifetime"`
	ConnMaxIdleTime time.Duration `mapstructure:"conn_max_idle_time" json:"conn_max_idle_time"`
	RequestTimeout time.Duration `mapstructure:"request_timeout" json:"request_timeout"`
	ConnRetryAttempts int `mapstructure:"conn_retry_attempts" json:"conn_retry_attempts" validate:"min=1"`
	ConnRetryInterval time.Duration `mapstructure:"conn_retry_interval" json:"conn_retry_interval"`
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
// Includes defaults from HYPERFLEET-694 for connection pool management
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
			MaxConnections:     50,
			MaxIdleConnections: 10,
			ConnMaxLifetime:    5 * time.Minute,
			ConnMaxIdleTime:    1 * time.Minute,
			RequestTimeout:     30 * time.Second,
			ConnRetryAttempts:  10,
			ConnRetryInterval:  3 * time.Second,
		},
	}
}

// ============================================================
// Convenience Accessor Methods
// ============================================================

// SSLMode returns SSL mode
func (c *DatabaseConfig) SSLMode() string {
	return c.SSL.Mode
}

// MaxOpenConnections returns max open connections
func (c *DatabaseConfig) MaxOpenConnections() int {
	return c.Pool.MaxConnections
}

// RootCertFile returns root cert file
func (c *DatabaseConfig) RootCertFile() string {
	return c.SSL.RootCertFile
}

// ============================================================
// Connection String Generation
// ============================================================

// escapeDSNValue escapes a PostgreSQL DSN parameter value according to libpq rules.
//
// According to PostgreSQL libpq documentation:
//   - Simple values (containing only letters, digits, hyphens, dots) don't need quoting
//   - Values with special characters (spaces, =, ', \, etc.) must be quoted
//   - '\': escape character (must be escaped as \\)
//   - "'": quote character (must be escaped as \')
//
// This function follows the libpq specification: only quote when necessary.
func escapeDSNValue(value string) string {
	if value == "" {
		return value
	}

	// Escape backslashes first (must be done before escaping quotes)
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	// Escape single quotes
	escaped = strings.ReplaceAll(escaped, `'`, `\'`)

	// Quote if the value contains any non-simple characters
	// Per libpq spec, simple values match: ^[a-zA-Z0-9.-]+$
	if !simpleDSNValuePattern.MatchString(escaped) {
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

// SetLogLevel determines the GORM logger level based on database.debug flag and global logging level.
// Priority: database.debug > global logging level > default
//
// Behavior:
//   - database.debug=true → logger.Info (show all SQL queries)
//   - logging.level=debug → logger.Info (show all SQL queries)
//   - logging.level=error → logger.Silent (suppress all SQL logs)
//   - default → logger.Warn (show only slow queries and errors)
func (c *DatabaseConfig) SetLogLevel(globalLogLevel string) logger.LogLevel {
	// database.debug takes precedence for explicit database debugging
	if c.Debug {
		return logger.Info
	}

	// Fall back to global log level for production debugging
	switch globalLogLevel {
	case "debug":
		return logger.Info // Enable SQL query logging when global debug is on
	case "error":
		return logger.Silent // Suppress SQL logs in error-only mode
	default:
		return logger.Warn // Default: log slow queries and errors
	}
}
