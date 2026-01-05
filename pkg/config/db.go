package config

import (
	"fmt"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type DatabaseConfig struct {
	Dialect            string `mapstructure:"dialect" json:"dialect" validate:"required"`
	Host               string `mapstructure:"host" json:"host" validate:""`
	Port               int    `mapstructure:"port" json:"port" validate:"min=0,max=65535"`
	Name               string `mapstructure:"name" json:"name" validate:""`
	Username           string `mapstructure:"username" json:"username" validate:""`
	Password           string `mapstructure:"password" json:"password" validate:""`
	SSLMode            string `mapstructure:"sslmode" json:"sslmode" validate:"oneof=disable require verify-ca verify-full"`
	RootCertFile       string `mapstructure:"rootcert_file" json:"rootcert_file" validate:""`
	Debug              bool   `mapstructure:"debug" json:"debug"`
	MaxOpenConnections int    `mapstructure:"max_open_connections" json:"max_open_connections" validate:"min=1"`
}

func NewDatabaseConfig() *DatabaseConfig {
	return &DatabaseConfig{
		Dialect:            "postgres",
		SSLMode:            "disable",
		Debug:              false,
		MaxOpenConnections: 50,
	}
}

// defineAndBindFlags defines database flags and binds them to viper keys in a single pass
func (c *DatabaseConfig) defineAndBindFlags(v *viper.Viper, fs *pflag.FlagSet) {
	// Direct database connection parameters
	defineAndBindStringFlag(v, fs, "database.host", "db-host", "", c.Host, "Database host")
	defineAndBindIntFlag(v, fs, "database.port", "db-port", "", c.Port, "Database port")
	defineAndBindStringFlag(v, fs, "database.username", "db-username", "u", c.Username, "Database username")
	defineAndBindStringFlag(v, fs, "database.password", "db-password", "", c.Password, "Database password (prefer using env var)")
	defineAndBindStringFlag(v, fs, "database.name", "db-name", "d", c.Name, "Database name")

	// Connection options
	defineAndBindStringFlag(v, fs, "database.rootcert_file", "db-rootcert", "", c.RootCertFile, "Database root certificate file")
	defineAndBindStringFlag(v, fs, "database.sslmode", "db-sslmode", "", c.SSLMode, "Database SSL mode (disable | require | verify-ca | verify-full)")
	defineAndBindBoolFlag(v, fs, "database.debug", "db-debug", "", c.Debug, "Enable database debug mode")
	defineAndBindIntFlag(v, fs, "database.max_open_connections", "db-max-open-connections", "", c.MaxOpenConnections, "Maximum open DB connections")
}

// ConnectionString returns the database connection string
func (c *DatabaseConfig) ConnectionString(withSSL bool) string {
	return c.ConnectionStringWithName(c.Name, withSSL)
}

// ConnectionStringWithName returns the database connection string with a specific database name
func (c *DatabaseConfig) ConnectionStringWithName(name string, withSSL bool) string {
	var cmd string
	if withSSL {
		cmd = fmt.Sprintf(
			"host=%s port=%d user=%s password='%s' dbname=%s sslmode=%s sslrootcert=%s",
			c.Host, c.Port, c.Username, c.Password, name, c.SSLMode, c.RootCertFile,
		)
	} else {
		cmd = fmt.Sprintf(
			"host=%s port=%d user=%s password='%s' dbname=%s sslmode=disable",
			c.Host, c.Port, c.Username, c.Password, name,
		)
	}

	return cmd
}

// LogSafeConnectionString returns a connection string with sensitive data redacted
func (c *DatabaseConfig) LogSafeConnectionString(withSSL bool) string {
	return c.LogSafeConnectionStringWithName(c.Name, withSSL)
}

// LogSafeConnectionStringWithName returns a connection string with sensitive data redacted
func (c *DatabaseConfig) LogSafeConnectionStringWithName(name string, withSSL bool) string {
	if withSSL {
		return fmt.Sprintf(
			"host=%s port=%d user=%s password='<REDACTED>' dbname=%s sslmode=%s sslrootcert='<REDACTED>'",
			c.Host, c.Port, c.Username, name, c.SSLMode,
		)
	}
	return fmt.Sprintf(
		"host=%s port=%d user=%s password='<REDACTED>' dbname=%s",
		c.Host, c.Port, c.Username, name,
	)
}
