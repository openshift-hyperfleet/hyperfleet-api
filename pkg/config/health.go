package config

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

// HealthConfig holds health check server configuration
// Follows HyperFleet Configuration Standard
type HealthConfig struct {
	Host            string        `mapstructure:"host" json:"host" validate:"required"`
	Port            int           `mapstructure:"port" json:"port" validate:"required,min=1,max=65535"`
	TLS             TLSConfig     `mapstructure:"tls" json:"tls" validate:"required"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout" json:"shutdown_timeout" validate:"required"`
	DBPingTimeout   time.Duration `mapstructure:"db_ping_timeout" json:"db_ping_timeout"` // From HYPERFLEET-694
}

// Validate validates health config durations
func (h *HealthConfig) Validate() error {
	if h.ShutdownTimeout < 1*time.Second {
		return fmt.Errorf("shutdown timeout must be at least 1 second, got %v", h.ShutdownTimeout)
	}
	if h.ShutdownTimeout > 120*time.Second {
		return fmt.Errorf("shutdown timeout must be at most 120 seconds, got %v", h.ShutdownTimeout)
	}
	return nil
}

// NewHealthConfig returns default HealthConfig values
// These defaults can be overridden by config file, env vars, or CLI flags
func NewHealthConfig() *HealthConfig {
	return &HealthConfig{
		Host: "localhost",
		Port: 8080,
		TLS: TLSConfig{
			Enabled: false,
		},
		ShutdownTimeout: 20 * time.Second,
		DBPingTimeout:   2 * time.Second, // From HYPERFLEET-694
	}
}

// ============================================================
// Convenience Accessor Methods
// ============================================================

// BindAddress returns bind address in host:port format
// Uses net.JoinHostPort to correctly handle IPv6 addresses
func (h *HealthConfig) BindAddress() string {
	return net.JoinHostPort(h.Host, strconv.Itoa(h.Port))
}

// EnableHTTPS returns TLS enabled flag (legacy accessor)
func (h *HealthConfig) EnableHTTPS() bool {
	return h.TLS.Enabled
}

// GetShutdownTimeout returns shutdown timeout (legacy accessor)
func (h *HealthConfig) GetShutdownTimeout() time.Duration {
	return h.ShutdownTimeout
}
