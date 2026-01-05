package config

import (
	"fmt"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type MetricsConfig struct {
	Host                          string        `mapstructure:"host" json:"host" validate:""`
	Port                          int           `mapstructure:"port" json:"port" validate:"min=1,max=65535"`
	EnableHTTPS                   bool          `mapstructure:"enable_https" json:"enable_https"`
	LabelMetricsInclusionDuration time.Duration `mapstructure:"label_metrics_inclusion_duration" json:"label_metrics_inclusion_duration"`

	// Legacy field for backward compatibility
	BindAddress string `mapstructure:"bind_address" json:"bind_address,omitempty"`
}

func NewMetricsConfig() *MetricsConfig {
	return &MetricsConfig{
		Host:                          "localhost",
		Port:                          8080,
		EnableHTTPS:                   false,
		LabelMetricsInclusionDuration: 7 * 24 * time.Hour,
		BindAddress:                   "localhost:8080",
	}
}

// defineAndBindFlags defines & binds flags to viper keys in a single pass
func (s *MetricsConfig) defineAndBindFlags(v *viper.Viper, fs *pflag.FlagSet) {
	defineAndBindStringFlag(v, fs, "metrics.host", "metrics-host", "", s.Host, "Metrics server bind host")
	defineAndBindIntFlag(v, fs, "metrics.port", "metrics-port", "", s.Port, "Metrics server bind port")
	defineAndBindBoolFlag(v, fs, "metrics.enable_https", "metrics-https-enabled", "", s.EnableHTTPS, "Enable HTTPS for metrics server")
	defineAndBindDurationFlag(v, fs, "metrics.label_metrics_inclusion_duration", "metrics-label-inclusion-duration", "", s.LabelMetricsInclusionDuration,
		"A cluster's last telemetry date needs to be within this duration to have labels collected")
}

// GetBindAddress returns the bind address in host:port format
func (s *MetricsConfig) GetBindAddress() string {
	if s.BindAddress != "" {
		return s.BindAddress
	}
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}
