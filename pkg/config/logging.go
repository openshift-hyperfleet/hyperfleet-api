package config

import (
	"strings"
)

// LoggingConfig holds logging configuration
// Follows HyperFleet Configuration Standard
type LoggingConfig struct {
	Level   string        `mapstructure:"level" json:"level" validate:"required,oneof=debug info warn error"`
	Format  string        `mapstructure:"format" json:"format" validate:"required,oneof=json text"`
	Output  string        `mapstructure:"output" json:"output" validate:"required,oneof=stdout stderr"`
	Masking MaskingConfig `mapstructure:"masking" json:"masking" validate:"required"`
	OTel    OTelConfig    `mapstructure:"otel" json:"otel" validate:"required"`
}

// OTelConfig holds OpenTelemetry configuration
// Configuration is driven entirely by standard environment variables (HyperFleet + OpenTelemetry):
//   - TRACING_ENABLED: enable/disable tracing
//   - OTEL_SERVICE_NAME: service name (default: "hyperfleet-api")
//   - OTEL_EXPORTER_OTLP_ENDPOINT: OTLP collector endpoint (if not set, uses stdout)
//   - OTEL_EXPORTER_OTLP_PROTOCOL: "grpc" (default) or "http/protobuf"
//   - OTEL_TRACES_SAMPLER: sampler type (default: "parentbased_traceidratio")
//   - OTEL_TRACES_SAMPLER_ARG: sampling rate 0.0-1.0 (default: 1.0)
//   - OTEL_RESOURCE_ATTRIBUTES: additional resource attributes (k=v,k2=v2)
//   - K8S_NAMESPACE: kubernetes namespace (added as k8s.namespace.name)
type OTelConfig struct {
	Enabled bool `mapstructure:"enabled" json:"enabled"`
}

// MaskingConfig holds log masking configuration
type MaskingConfig struct {
	Headers []string `mapstructure:"headers" json:"headers"`
	Fields  []string `mapstructure:"fields" json:"fields"`
	Enabled bool     `mapstructure:"enabled" json:"enabled"`
}

// NewLoggingConfig returns default LoggingConfig values
// These defaults can be overridden by config file, env vars, or CLI flags
func NewLoggingConfig() *LoggingConfig {
	return &LoggingConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
		OTel: OTelConfig{
			Enabled: false,
		},
		Masking: MaskingConfig{
			Enabled: true,
			Headers: []string{
				"Authorization",
				"X-API-Key",
				"Cookie",
				"X-Auth-Token",
				"X-Forwarded-Authorization",
			},
			Fields: []string{
				"password",
				"secret",
				"token",
				"api_key",
				"access_token",
				"refresh_token",
				"client_secret",
			},
		},
	}
}

// ============================================================
// HELPER METHODS
// ============================================================

// GetSensitiveHeadersList returns list of sensitive headers
// This is used by logger for masking
func (l *LoggingConfig) GetSensitiveHeadersList() []string {
	return l.Masking.Headers
}

// GetSensitiveFieldsList returns list of sensitive fields
// This is used by logger for masking
func (l *LoggingConfig) GetSensitiveFieldsList() []string {
	return l.Masking.Fields
}

// ============================================================
// Convenience Accessor Methods
// String conversion methods for CLI flags
// ============================================================

// GetSensitiveHeadersString returns headers as comma-separated string
func (l *LoggingConfig) GetSensitiveHeadersString() string {
	return strings.Join(l.Masking.Headers, ",")
}

// GetSensitiveFieldsString returns fields as comma-separated string
func (l *LoggingConfig) GetSensitiveFieldsString() string {
	return strings.Join(l.Masking.Fields, ",")
}
