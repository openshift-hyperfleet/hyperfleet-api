package config

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

// MetricsConfig holds Prometheus metrics server configuration
// Follows HyperFleet Configuration Standard
type MetricsConfig struct {
	Host                          string        `mapstructure:"host" json:"host" validate:"required"`
	TLS                           TLSConfig     `mapstructure:"tls" json:"tls" validate:"required"`
	Port                          int           `mapstructure:"port" json:"port" validate:"required,min=1,max=65535"`
	LabelMetricsInclusionDuration time.Duration `mapstructure:"label_metrics_inclusion_duration" json:"label_metrics_inclusion_duration" validate:"required"` //nolint:lll
	DeletionStuckThreshold        time.Duration `mapstructure:"deletion_stuck_threshold" json:"deletion_stuck_threshold" validate:"required"`                 //nolint:lll
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
		DeletionStuckThreshold:        30 * time.Minute,
	}
}

// Validate validates MetricsConfig fields that struct tags cannot enforce
func (m *MetricsConfig) Validate() error {
	if m.DeletionStuckThreshold <= 0 {
		return fmt.Errorf("DeletionStuckThreshold must be positive, got %v", m.DeletionStuckThreshold)
	}
	return nil
}

// ============================================================
// Convenience Accessor Methods
// ============================================================

// BindAddress returns bind address in host:port format
// Uses net.JoinHostPort to correctly handle IPv6 addresses
func (m *MetricsConfig) BindAddress() string {
	return net.JoinHostPort(m.Host, strconv.Itoa(m.Port))
}

// GetLabelMetricsInclusionDuration returns label metrics inclusion duration
func (m *MetricsConfig) GetLabelMetricsInclusionDuration() time.Duration {
	return m.LabelMetricsInclusionDuration
}
