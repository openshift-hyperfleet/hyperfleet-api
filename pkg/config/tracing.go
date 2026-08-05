package config

type TracingConfig struct {
	ServiceName string `mapstructure:"service_name" json:"service_name"`
	Enabled     bool   `mapstructure:"enabled" json:"enabled"`
}

func NewTracingConfig() *TracingConfig {
	return &TracingConfig{
		Enabled:     true,
		ServiceName: "hyperfleet-api",
	}
}
