package config

import (
	"context"
	"os"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

// TestNewLoggingConfig_Defaults tests default configuration values
func TestNewLoggingConfig_Defaults(t *testing.T) {
	RegisterTestingT(t)

	cfg := NewLoggingConfig()

	Expect(cfg.Level).To(Equal("info"))
	Expect(cfg.Format).To(Equal("json"))
	Expect(cfg.Output).To(Equal("stdout"))
	Expect(cfg.OTel.Enabled).To(BeFalse())
	Expect(cfg.OTel.SamplingRate).To(Equal(1.0))
	Expect(cfg.Masking.Enabled).To(BeTrue())
	Expect(cfg.Masking.Headers).NotTo(BeEmpty())
	Expect(cfg.Masking.Fields).NotTo(BeEmpty())
}

// TestConfigLoader_LoggingFromEnv tests loading logging config from environment
func TestConfigLoader_LoggingFromEnv(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	t.Setenv("HYPERFLEET_LOGGING_LEVEL", "debug")
	t.Setenv("HYPERFLEET_LOGGING_FORMAT", "text")
	t.Setenv("HYPERFLEET_LOGGING_OTEL_ENABLED", "true")
	t.Setenv("HYPERFLEET_LOGGING_OTEL_SAMPLING_RATE", "0.5")

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	appConfig, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())
	Expect(appConfig.Logging.Level).To(Equal("debug"))
	Expect(appConfig.Logging.Format).To(Equal("text"))
	Expect(appConfig.Logging.OTel.Enabled).To(BeTrue())
	Expect(appConfig.Logging.OTel.SamplingRate).To(Equal(0.5))
}

// TestConfigLoader_LoggingBackwardCompat tests backward compatibility env vars
func TestConfigLoader_LoggingBackwardCompat(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	// Unset new env vars to test backward compatibility with old vars only
	t.Setenv("HYPERFLEET_LOGGING_LEVEL", "")
	t.Setenv("HYPERFLEET_LOGGING_FORMAT", "")

	// Set old-style env vars
	t.Setenv("LOG_LEVEL", "warn")
	t.Setenv("LOG_FORMAT", "text")
	t.Setenv("OTEL_ENABLED", "true")
	t.Setenv("OTEL_SAMPLING_RATE", "0.25")

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	appConfig, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())
	Expect(appConfig.Logging.Level).To(Equal("warn"))
	Expect(appConfig.Logging.Format).To(Equal("text"))
	Expect(appConfig.Logging.OTel.Enabled).To(BeTrue())
	Expect(appConfig.Logging.OTel.SamplingRate).To(Equal(0.25))
}

// TestLoggingConfig_GetSensitiveHeadersList tests the headers array accessor
func TestLoggingConfig_GetSensitiveHeadersList(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "standard list",
			input:    []string{"Authorization", "X-API-Key", "Cookie"},
			expected: []string{"Authorization", "X-API-Key", "Cookie"},
		},
		{
			name:     "empty array",
			input:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewLoggingConfig()
			cfg.Masking.Headers = tt.input

			headers := cfg.GetSensitiveHeadersList()

			Expect(headers).To(Equal(tt.expected))
		})
	}
}

// TestLoggingConfig_GetSensitiveFieldsList tests the fields array accessor
func TestLoggingConfig_GetSensitiveFieldsList(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "standard list",
			input:    []string{"password", "secret", "token"},
			expected: []string{"password", "secret", "token"},
		},
		{
			name:     "empty array",
			input:    []string{},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewLoggingConfig()
			cfg.Masking.Fields = tt.input

			fields := cfg.GetSensitiveFieldsList()

			Expect(fields).To(Equal(tt.expected))
		})
	}
}

// ==============================================================
// Comprehensive Config Loader Tests
// ==============================================================

// TestConfigPrecedence tests the core config loader precedence contract:
// CLI flags > environment variables > config file > defaults
func TestConfigPrecedence(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name           string
		configFile     string
		envVars        map[string]string
		cliFlags       map[string]string
		expectedLevel  string
		expectedFormat string
	}{
		{
			name:           "defaults only",
			expectedLevel:  "info",
			expectedFormat: "json",
		},
		{
			name: "file overrides defaults",
			configFile: `
logging:
  level: "debug"
  format: "text"
`,
			expectedLevel:  "debug",
			expectedFormat: "text",
		},
		{
			name: "env overrides file",
			configFile: `
logging:
  level: "debug"
  format: "text"
`,
			envVars: map[string]string{
				"HYPERFLEET_LOGGING_LEVEL":  "warn",
				"HYPERFLEET_LOGGING_FORMAT": "json",
			},
			expectedLevel:  "warn",
			expectedFormat: "json",
		},
		{
			name: "flags override env and file",
			configFile: `
logging:
  level: "debug"
  format: "text"
`,
			envVars: map[string]string{
				"HYPERFLEET_LOGGING_LEVEL": "warn",
			},
			cliFlags: map[string]string{
				"log-level": "error",
			},
			expectedLevel:  "error",
			expectedFormat: "text", // From file, no env or flag override
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			// Setup minimal test environment
			SetMinimalTestEnv(t)

			ctx := context.Background()
			var configPath string

			// Create config file if provided
			if tt.configFile != "" {
				tmpDir := t.TempDir()
				configPath = tmpDir + "/config.yaml"
				err := os.WriteFile(configPath, []byte(tt.configFile), 0600)
				Expect(err).NotTo(HaveOccurred())
				t.Setenv("HYPERFLEET_CONFIG", configPath)

				// Unset env vars that would override config file for logging tests
				t.Setenv("HYPERFLEET_LOGGING_LEVEL", "")
				t.Setenv("HYPERFLEET_LOGGING_FORMAT", "")
			}

			// Set environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			// Create command with flags
			cmd := &cobra.Command{}
			AddLoggingFlags(cmd)
			for flag, value := range tt.cliFlags {
				_ = cmd.Flags().Set(flag, value)
			}

			// Load config
			loader := NewConfigLoader()
			appConfig, err := loader.Load(ctx, cmd)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(appConfig.Logging.Level).To(Equal(tt.expectedLevel),
				"logging level should match expected precedence")
			Expect(appConfig.Logging.Format).To(Equal(tt.expectedFormat),
				"logging format should match expected precedence")
		})
	}
}

// TestValidationFailures tests that the loader properly validates configuration
// and returns helpful error messages
func TestValidationFailures(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name          string
		envVars       map[string]string
		expectedError string
	}{
		{
			name: "invalid server port (too low)",
			envVars: map[string]string{
				"HYPERFLEET_SERVER_PORT": "0",
			},
			expectedError: "Configuration validation failed",
		},
		{
			name: "invalid server port (too high)",
			envVars: map[string]string{
				"HYPERFLEET_SERVER_PORT": "99999",
			},
			expectedError: "Configuration validation failed",
		},
		{
			name: "invalid database host",
			envVars: map[string]string{
				"HYPERFLEET_DATABASE_HOST": "invalid!@#$%",
			},
			expectedError: "Configuration validation failed",
		},
		{
			name: "invalid database dialect",
			envVars: map[string]string{
				"HYPERFLEET_DATABASE_DIALECT": "invalid",
			},
			expectedError: "Configuration validation failed",
		},
		{
			name: "invalid server read timeout (too short)",
			envVars: map[string]string{
				"HYPERFLEET_SERVER_TIMEOUTS_READ": "500ms",
			},
			expectedError: "server timeouts validation failed",
		},
		{
			name: "invalid health shutdown timeout (too long)",
			envVars: map[string]string{
				"HYPERFLEET_HEALTH_SHUTDOWN_TIMEOUT": "200s",
			},
			expectedError: "health config validation failed",
		},
		{
			name: "server TLS enabled without cert file",
			envVars: map[string]string{
				"HYPERFLEET_SERVER_TLS_ENABLED":  "true",
				"HYPERFLEET_SERVER_TLS_KEY_FILE": "/path/to/key.pem",
			},
			expectedError: "server TLS validation failed",
		},
		{
			name: "server TLS enabled without key file",
			envVars: map[string]string{
				"HYPERFLEET_SERVER_TLS_ENABLED":   "true",
				"HYPERFLEET_SERVER_TLS_CERT_FILE": "/path/to/cert.pem",
			},
			expectedError: "server TLS validation failed",
		},
		{
			name: "health TLS enabled without cert file",
			envVars: map[string]string{
				"HYPERFLEET_HEALTH_TLS_ENABLED":  "true",
				"HYPERFLEET_HEALTH_TLS_KEY_FILE": "/path/to/key.pem",
			},
			expectedError: "health TLS validation failed",
		},
		{
			name: "metrics TLS enabled without key file",
			envVars: map[string]string{
				"HYPERFLEET_METRICS_TLS_ENABLED":   "true",
				"HYPERFLEET_METRICS_TLS_CERT_FILE": "/path/to/cert.pem",
			},
			expectedError: "metrics TLS validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			SetMinimalTestEnv(t)

			// Set invalid environment variables
			for key, value := range tt.envVars {
				t.Setenv(key, value)
			}

			loader := NewConfigLoader()
			cmd := &cobra.Command{}
			ctx := context.Background()

			// Load should fail validation
			_, err := loader.Load(ctx, cmd)

			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(tt.expectedError))
		})
	}
}

// TestFileSecretResolution tests that the loader properly reads secrets from files
// referenced by *_FILE environment variables and prefers them appropriately
func TestFileSecretResolution(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name             string
		fileContent      string
		envVar           string
		plainEnvVar      string
		plainEnvValue    string
		expectedPassword string
		description      string
	}{
		{
			name:             "file secret only",
			fileContent:      "file-secret-password",
			envVar:           "HYPERFLEET_DATABASE_PASSWORD_FILE",
			expectedPassword: "file-secret-password",
			description:      "password from file secret only",
		},
		{
			name:             "plain env var only",
			plainEnvVar:      "HYPERFLEET_DATABASE_PASSWORD",
			plainEnvValue:    "plain-env-password",
			expectedPassword: "plain-env-password",
			description:      "password from plain env var only",
		},
		{
			name:             "env var takes precedence over file secret",
			fileContent:      "file-secret-password",
			envVar:           "HYPERFLEET_DATABASE_PASSWORD_FILE",
			plainEnvVar:      "HYPERFLEET_DATABASE_PASSWORD",
			plainEnvValue:    "plain-env-password",
			expectedPassword: "plain-env-password",
			description:      "env var should win over file secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			SetMinimalTestEnv(t)

			// Unset the default password set by SetMinimalTestEnv so our test values are used
			t.Setenv("HYPERFLEET_DATABASE_PASSWORD", "")

			tmpDir := t.TempDir()

			// Create file secret if needed
			if tt.fileContent != "" && tt.envVar != "" {
				secretFile := tmpDir + "/secret"
				err := os.WriteFile(secretFile, []byte(tt.fileContent), 0600)
				Expect(err).NotTo(HaveOccurred())
				t.Setenv(tt.envVar, secretFile)
			}

			// Set plain env var if needed
			if tt.plainEnvVar != "" && tt.plainEnvValue != "" {
				t.Setenv(tt.plainEnvVar, tt.plainEnvValue)
			}

			loader := NewConfigLoader()
			cmd := &cobra.Command{}
			ctx := context.Background()

			// Load config
			appConfig, err := loader.Load(ctx, cmd)

			// Assert
			Expect(err).NotTo(HaveOccurred())
			Expect(appConfig.Database.Password).To(Equal(tt.expectedPassword),
				tt.description)
		})
	}
}
