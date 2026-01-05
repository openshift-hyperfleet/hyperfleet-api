package config

import (
	"fmt"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type HealthCheckConfig struct {
	Host        string `mapstructure:"host" json:"host" validate:""`
	Port        int    `mapstructure:"port" json:"port" validate:"min=1,max=65535"`
	EnableHTTPS bool   `mapstructure:"enable_https" json:"enable_https"`

	// Legacy field for backward compatibility
	BindAddress string `mapstructure:"bind_address" json:"bind_address,omitempty"`
}

func NewHealthCheckConfig() *HealthCheckConfig {
	return &HealthCheckConfig{
		Host:        "localhost",
		Port:        8083,
		EnableHTTPS: false,
		BindAddress: "localhost:8083",
	}
}

// defineAndBindFlags defines & binds flags to viper keys in a single pass
func (c *HealthCheckConfig) defineAndBindFlags(v *viper.Viper, fs *pflag.FlagSet) {
	defineAndBindStringFlag(v, fs, "health_check.host", "health-check-host", "", c.Host, "Health check server bind host")
	defineAndBindIntFlag(v, fs, "health_check.port", "health-check-port", "", c.Port, "Health check server bind port")
	defineAndBindBoolFlag(v, fs, "health_check.enable_https", "health-check-https-enabled", "", c.EnableHTTPS, "Enable HTTPS for health check server")
}

// GetBindAddress returns the bind address in host:port format
func (c *HealthCheckConfig) GetBindAddress() string {
	if c.BindAddress != "" {
		return c.BindAddress
	}
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}
