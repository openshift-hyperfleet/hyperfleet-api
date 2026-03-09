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
	OTel    OTelConfig    `mapstructure:"otel" json:"otel" validate:"required"`
	Masking MaskingConfig `mapstructure:"masking" json:"masking" validate:"required"`
}

// OTelConfig holds OpenTelemetry configuration
type OTelConfig struct {
	Enabled      bool    `mapstructure:"enabled" json:"enabled"`
	SamplingRate float64 `mapstructure:"sampling_rate" json:"sampling_rate" validate:"gte=0,lte=1"`
}

// MaskingConfig holds log masking configuration
type MaskingConfig struct {
	Enabled bool     `mapstructure:"enabled" json:"enabled"`
	Headers []string `mapstructure:"headers" json:"headers"`
	Fields  []string `mapstructure:"fields" json:"fields"`
}

// NewLoggingConfig returns default LoggingConfig values
// These defaults can be overridden by config file, env vars, or CLI flags
func NewLoggingConfig() *LoggingConfig {
	return &LoggingConfig{
		Level:  "info",
		Format: "json",
		Output: "stdout",
		OTel: OTelConfig{
			Enabled:      false,
			SamplingRate: 1.0,
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
// BACKWARD COMPATIBILITY HELPERS
// For old configuration system that uses comma-separated strings
// ============================================================

// GetSensitiveHeadersString returns headers as comma-separated string (legacy)
func (l *LoggingConfig) GetSensitiveHeadersString() string {
	return strings.Join(l.Masking.Headers, ",")
}

// GetSensitiveFieldsString returns fields as comma-separated string (legacy)
func (l *LoggingConfig) GetSensitiveFieldsString() string {
	return strings.Join(l.Masking.Fields, ",")
}

// SetSensitiveHeadersFromString sets headers from comma-separated string (legacy)
func (l *LoggingConfig) SetSensitiveHeadersFromString(headers string) {
	if headers == "" {
		l.Masking.Headers = []string{}
		return
	}

	parts := strings.Split(headers, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	l.Masking.Headers = result
}

// SetSensitiveFieldsFromString sets fields from comma-separated string (legacy)
func (l *LoggingConfig) SetSensitiveFieldsFromString(fields string) {
	if fields == "" {
		l.Masking.Fields = []string{}
		return
	}

	parts := strings.Split(fields, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	l.Masking.Fields = result
}
