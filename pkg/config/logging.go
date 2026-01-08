package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/spf13/pflag"
)

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `json:"log_level"`
	Format string `json:"log_format"`
	Output string `json:"log_output"`

	OTel    OTelConfig    `json:"otel"`
	Masking MaskingConfig `json:"masking"`
}

// OTelConfig holds OpenTelemetry configuration
type OTelConfig struct {
	Enabled      bool    `json:"enabled"`
	SamplingRate float64 `json:"sampling_rate"`
}

// MaskingConfig holds data masking configuration
type MaskingConfig struct {
	Enabled          bool   `json:"enabled"`
	SensitiveHeaders string `json:"sensitive_headers"`
	SensitiveFields  string `json:"sensitive_fields"`
}

// NewLoggingConfig creates a new LoggingConfig with default values
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
			Enabled:          true,
			SensitiveHeaders: "Authorization,X-API-Key,Cookie,X-Auth-Token",
			SensitiveFields:  "password,secret,token,api_key,access_token,refresh_token",
		},
	}
}

// AddFlags adds CLI flags for core logging configuration
func (l *LoggingConfig) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&l.Level, "log-level", l.Level, "Log level (debug, info, warn, error)")
	fs.StringVar(&l.Format, "log-format", l.Format, "Log format (text, json)")
	fs.StringVar(&l.Output, "log-output", l.Output, "Log output (stdout, stderr)")
}

// ReadFiles satisfies the config interface
func (l *LoggingConfig) ReadFiles() error {
	return nil
}

// BindEnv reads configuration from environment variables
// Priority: flags > env vars > defaults
// If fs is nil, all env vars are applied (backward compatibility)
func (l *LoggingConfig) BindEnv(fs *pflag.FlagSet) {
	// Fields with flags: only apply env if flag not set
	if val := os.Getenv("LOG_LEVEL"); val != "" {
		if fs == nil || !fs.Changed("log-level") {
			l.Level = val
		}
	}
	if val := os.Getenv("LOG_FORMAT"); val != "" {
		if fs == nil || !fs.Changed("log-format") {
			l.Format = val
		}
	}
	if val := os.Getenv("LOG_OUTPUT"); val != "" {
		if fs == nil || !fs.Changed("log-output") {
			l.Output = val
		}
	}

	// Fields without flags: always apply env vars
	if val := os.Getenv("OTEL_ENABLED"); val != "" {
		enabled, err := strconv.ParseBool(val)
		if err == nil {
			l.OTel.Enabled = enabled
		}
	}
	if val := os.Getenv("OTEL_SAMPLING_RATE"); val != "" {
		rate, err := strconv.ParseFloat(val, 64)
		if err == nil && rate >= 0.0 && rate <= 1.0 {
			l.OTel.SamplingRate = rate
		}
	}

	if val := os.Getenv("MASKING_ENABLED"); val != "" {
		enabled, err := strconv.ParseBool(val)
		if err == nil {
			l.Masking.Enabled = enabled
		}
	}
	if val := os.Getenv("MASKING_HEADERS"); val != "" {
		l.Masking.SensitiveHeaders = val
	}
	if val := os.Getenv("MASKING_FIELDS"); val != "" {
		l.Masking.SensitiveFields = val
	}
}

// GetSensitiveHeadersList parses comma-separated sensitive headers
func (l *LoggingConfig) GetSensitiveHeadersList() []string {
	if l.Masking.SensitiveHeaders == "" {
		return []string{}
	}
	headers := strings.Split(l.Masking.SensitiveHeaders, ",")
	for i := range headers {
		headers[i] = strings.TrimSpace(headers[i])
	}
	return headers
}

// GetSensitiveFieldsList parses comma-separated sensitive fields
func (l *LoggingConfig) GetSensitiveFieldsList() []string {
	if l.Masking.SensitiveFields == "" {
		return []string{}
	}
	fields := strings.Split(l.Masking.SensitiveFields, ",")
	for i := range fields {
		fields[i] = strings.TrimSpace(fields[i])
	}
	return fields
}
