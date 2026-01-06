package config

import (
	"os"

	"github.com/spf13/pflag"
)

// LoggingConfig contains configuration for structured logging
type LoggingConfig struct {
	// Level is the minimum log level (debug, info, warn, error)
	Level string `json:"level"`
	// Format is the output format (text, json)
	Format string `json:"format"`
	// Output is the destination (stdout, stderr)
	Output string `json:"output"`
}

// NewLoggingConfig creates a new LoggingConfig with defaults
// Precedence: flags → environment variables → defaults
// Env vars are read here as initial values, flags can override via AddFlags()
func NewLoggingConfig() *LoggingConfig {
	cfg := &LoggingConfig{
		Level:  "info",
		Format: "text",
		Output: "stdout",
	}

	// Apply environment variables as overrides to defaults
	// Flags (via AddFlags + Parse) will override these if explicitly set
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		cfg.Level = level
	}
	if format := os.Getenv("LOG_FORMAT"); format != "" {
		cfg.Format = format
	}
	if output := os.Getenv("LOG_OUTPUT"); output != "" {
		cfg.Output = output
	}

	return cfg
}

// AddFlags adds logging configuration flags
func (c *LoggingConfig) AddFlags(flagset *pflag.FlagSet) {
	flagset.StringVar(&c.Level, "log-level", c.Level, "Minimum log level: debug, info, warn, error")
	flagset.StringVar(&c.Format, "log-format", c.Format, "Log output format: text, json")
	flagset.StringVar(&c.Output, "log-output", c.Output, "Log output destination: stdout, stderr")
}

// ReadFiles is a no-op for LoggingConfig.
// Environment variables are read in NewLoggingConfig() to ensure proper precedence:
// flags → environment variables → defaults
func (c *LoggingConfig) ReadFiles() error {
	return nil
}
