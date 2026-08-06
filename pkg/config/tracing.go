package config

import "fmt"

type TracingConfig struct {
	ServiceName string `mapstructure:"service_name" json:"service_name"`
	Enabled     bool   `mapstructure:"enabled" json:"enabled"`
}

func (c *TracingConfig) Validate() error {
	if c.Enabled && c.ServiceName == "" {
		return fmt.Errorf("tracing service_name is required when tracing is enabled")
	}
	return nil
}

func NewTracingConfig() *TracingConfig {
	return &TracingConfig{
		Enabled:     true,
		ServiceName: "hyperfleet-api",
	}
}
