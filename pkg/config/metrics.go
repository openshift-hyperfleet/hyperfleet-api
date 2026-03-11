package config

import (
	"net"
	"strconv"
	"time"
)

// MetricsConfig holds Prometheus metrics server configuration
// Follows HyperFleet Configuration Standard
type MetricsConfig struct {
	Host                             string        `mapstructure:"host" json:"host" validate:"required"`
	Port                             int           `mapstructure:"port" json:"port" validate:"required,min=1,max=65535"`
	TLS                              TLSConfig     `mapstructure:"tls" json:"tls" validate:"required"`
	LabelMetricsInclusionDuration    time.Duration `mapstructure:"label_metrics_inclusion_duration" json:"label_metrics_inclusion_duration" validate:"required"` //nolint:lll
}

// NewMetricsConfig returns default MetricsConfig values
// These defaults can be overridden by config file, env vars, or CLI flags
func NewMetricsConfig() *MetricsConfig {
	return &MetricsConfig{
		Host: "localhost",
		Port: 9090,
		TLS: TLSConfig{
			Enabled: false,
		},
		LabelMetricsInclusionDuration: 168 * time.Hour, // 7 days
	}
}

// ============================================================
// Convenience Accessor Methods
// ============================================================

// BindAddress returns bind address in host:port format
// Uses net.JoinHostPort to correctly handle IPv6 addresses
func (m *MetricsConfig) BindAddress() string {
	return net.JoinHostPort(m.Host, strconv.Itoa(m.Port))
}

// EnableHTTPS returns TLS enabled flag
func (m *MetricsConfig) EnableHTTPS() bool {
	return m.TLS.Enabled
}

// GetLabelMetricsInclusionDuration returns label metrics inclusion duration
func (m *MetricsConfig) GetLabelMetricsInclusionDuration() time.Duration {
	return m.LabelMetricsInclusionDuration
}
