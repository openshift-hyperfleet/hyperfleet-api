package config

import (
	"os"
	"testing"

	"github.com/spf13/pflag"
)

// TestNewLoggingConfig_Defaults tests default configuration values
func TestNewLoggingConfig_Defaults(t *testing.T) {
	cfg := NewLoggingConfig()

	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"Level", cfg.Level, "info"},
		{"Format", cfg.Format, "json"},
		{"Output", cfg.Output, "stdout"},
		{"OTel.Enabled", cfg.OTel.Enabled, false},
		{"OTel.SamplingRate", cfg.OTel.SamplingRate, 1.0},
		{"Masking.Enabled", cfg.Masking.Enabled, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, tt.got)
			}
		})
	}

	// Check non-empty string fields
	if cfg.Masking.SensitiveHeaders == "" {
		t.Error("expected default SensitiveHeaders to be non-empty")
	}
	if cfg.Masking.SensitiveFields == "" {
		t.Error("expected default SensitiveFields to be non-empty")
	}
}

// TestLoggingConfig_AddFlags tests CLI flag registration
func TestLoggingConfig_AddFlags(t *testing.T) {
	cfg := NewLoggingConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

	cfg.AddFlags(fs)

	// Verify flags are registered
	flags := []string{"log-level", "log-format", "log-output"}
	for _, flagName := range flags {
		t.Run("flag_"+flagName, func(t *testing.T) {
			if fs.Lookup(flagName) == nil {
				t.Errorf("expected %s flag to be registered", flagName)
			}
		})
	}

	// Test flag parsing
	tests := []struct {
		name     string
		args     []string
		expected map[string]string
	}{
		{
			name: "custom values",
			args: []string{"--log-level=debug", "--log-format=text", "--log-output=stderr"},
			expected: map[string]string{
				"level":  "debug",
				"format": "text",
				"output": "stderr",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewLoggingConfig()
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			cfg.AddFlags(fs)

			if err := fs.Parse(tt.args); err != nil {
				t.Fatalf("failed to parse flags: %v", err)
			}

			if cfg.Level != tt.expected["level"] {
				t.Errorf("expected Level '%s', got '%s'", tt.expected["level"], cfg.Level)
			}
			if cfg.Format != tt.expected["format"] {
				t.Errorf("expected Format '%s', got '%s'", tt.expected["format"], cfg.Format)
			}
			if cfg.Output != tt.expected["output"] {
				t.Errorf("expected Output '%s', got '%s'", tt.expected["output"], cfg.Output)
			}
		})
	}
}

// TestLoggingConfig_ReadFiles tests that ReadFiles returns nil (no file-based config)
func TestLoggingConfig_ReadFiles(t *testing.T) {
	cfg := NewLoggingConfig()
	if err := cfg.ReadFiles(); err != nil {
		t.Errorf("expected ReadFiles to return nil, got %v", err)
	}
}

// TestLoggingConfig_BindEnv tests environment variable binding
func TestLoggingConfig_BindEnv(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		validate func(*testing.T, *LoggingConfig)
	}{
		{
			name: "basic logging env vars",
			envVars: map[string]string{
				"LOG_LEVEL":  "debug",
				"LOG_FORMAT": "text",
				"LOG_OUTPUT": "stderr",
			},
			validate: func(t *testing.T, cfg *LoggingConfig) {
				if cfg.Level != "debug" {
					t.Errorf("expected Level 'debug', got '%s'", cfg.Level)
				}
				if cfg.Format != "text" {
					t.Errorf("expected Format 'text', got '%s'", cfg.Format)
				}
				if cfg.Output != "stderr" {
					t.Errorf("expected Output 'stderr', got '%s'", cfg.Output)
				}
			},
		},
		{
			name: "otel env vars",
			envVars: map[string]string{
				"OTEL_ENABLED":       "true",
				"OTEL_SAMPLING_RATE": "0.5",
			},
			validate: func(t *testing.T, cfg *LoggingConfig) {
				if cfg.OTel.Enabled != true {
					t.Errorf("expected OTel.Enabled true, got %t", cfg.OTel.Enabled)
				}
				if cfg.OTel.SamplingRate != 0.5 {
					t.Errorf("expected OTel.SamplingRate 0.5, got %f", cfg.OTel.SamplingRate)
				}
			},
		},
		{
			name: "masking env vars",
			envVars: map[string]string{
				"MASKING_ENABLED": "false",
				"MASKING_HEADERS": "Custom-Header,Another-Header",
				"MASKING_FIELDS":  "custom_field,another_field",
			},
			validate: func(t *testing.T, cfg *LoggingConfig) {
				if cfg.Masking.Enabled != false {
					t.Errorf("expected Masking.Enabled false, got %t", cfg.Masking.Enabled)
				}
				if cfg.Masking.SensitiveHeaders != "Custom-Header,Another-Header" {
					t.Errorf("expected custom SensitiveHeaders, got '%s'", cfg.Masking.SensitiveHeaders)
				}
				if cfg.Masking.SensitiveFields != "custom_field,another_field" {
					t.Errorf("expected custom SensitiveFields, got '%s'", cfg.Masking.SensitiveFields)
				}
			},
		},
		{
			name: "invalid bool value keeps default",
			envVars: map[string]string{
				"OTEL_ENABLED": "not-a-bool",
			},
			validate: func(t *testing.T, cfg *LoggingConfig) {
				if cfg.OTel.Enabled != false {
					t.Errorf("expected OTel.Enabled to keep default (false), got %t", cfg.OTel.Enabled)
				}
			},
		},
		{
			name: "invalid float value keeps default",
			envVars: map[string]string{
				"OTEL_SAMPLING_RATE": "not-a-float",
			},
			validate: func(t *testing.T, cfg *LoggingConfig) {
				if cfg.OTel.SamplingRate != 1.0 {
					t.Errorf("expected OTel.SamplingRate to keep default (1.0), got %f", cfg.OTel.SamplingRate)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore env vars
			oldEnvs := make(map[string]string)
			for key := range tt.envVars {
				oldEnvs[key] = os.Getenv(key)
			}
			defer func() {
				for key, val := range oldEnvs {
					if val == "" {
						_ = os.Unsetenv(key)
					} else {
						_ = os.Setenv(key, val)
					}
				}
			}()

			// Set env vars
			for key, val := range tt.envVars {
				if err := os.Setenv(key, val); err != nil {
					t.Fatalf("failed to set env var %s: %v", key, err)
				}
			}

			cfg := NewLoggingConfig()
			cfg.BindEnv()

			tt.validate(t, cfg)
		})
	}
}

// TestLoggingConfig_GetSensitiveHeadersList tests parsing of comma-separated headers
func TestLoggingConfig_GetSensitiveHeadersList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "standard list",
			input:    "Authorization,X-API-Key,Cookie",
			expected: []string{"Authorization", "X-API-Key", "Cookie"},
		},
		{
			name:     "with whitespace",
			input:    "  Authorization  ,  X-API-Key  ,  Cookie  ",
			expected: []string{"Authorization", "X-API-Key", "Cookie"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewLoggingConfig()
			cfg.Masking.SensitiveHeaders = tt.input

			headers := cfg.GetSensitiveHeadersList()

			if len(headers) != len(tt.expected) {
				t.Fatalf("expected %d headers, got %d", len(tt.expected), len(headers))
			}
			for i, h := range tt.expected {
				if headers[i] != h {
					t.Errorf("expected header[%d] '%s', got '%s'", i, h, headers[i])
				}
			}
		})
	}
}

// TestLoggingConfig_GetSensitiveFieldsList tests parsing of comma-separated fields
func TestLoggingConfig_GetSensitiveFieldsList(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected []string
	}{
		{
			name:     "standard list",
			input:    "password,secret,token",
			expected: []string{"password", "secret", "token"},
		},
		{
			name:     "with whitespace",
			input:    "  password  ,  secret  ,  token  ",
			expected: []string{"password", "secret", "token"},
		},
		{
			name:     "empty string",
			input:    "",
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewLoggingConfig()
			cfg.Masking.SensitiveFields = tt.input

			fields := cfg.GetSensitiveFieldsList()

			if len(fields) != len(tt.expected) {
				t.Fatalf("expected %d fields, got %d", len(tt.expected), len(fields))
			}
			for i, f := range tt.expected {
				if fields[i] != f {
					t.Errorf("expected field[%d] '%s', got '%s'", i, f, fields[i])
				}
			}
		})
	}
}

// TestLoggingConfig_EnvOverridesFlags tests that environment variables override CLI flags
func TestLoggingConfig_EnvOverridesFlags(t *testing.T) {
	// Save and restore env var
	oldLevel := os.Getenv("LOG_LEVEL")
	defer func() {
		if oldLevel == "" {
			_ = os.Unsetenv("LOG_LEVEL")
		} else {
			_ = os.Setenv("LOG_LEVEL", oldLevel)
		}
	}()

	// Set env var
	if err := os.Setenv("LOG_LEVEL", "error"); err != nil {
		t.Fatalf("failed to set LOG_LEVEL: %v", err)
	}

	cfg := NewLoggingConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.AddFlags(fs)

	// Parse flags with different value
	args := []string{"--log-level=debug"}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	// Before BindEnv, should have flag value
	if cfg.Level != "debug" {
		t.Errorf("expected Level 'debug' from flag, got '%s'", cfg.Level)
	}

	// After BindEnv, env var should override
	cfg.BindEnv()
	if cfg.Level != "error" {
		t.Errorf("expected Level 'error' from env (after BindEnv), got '%s'", cfg.Level)
	}
}
