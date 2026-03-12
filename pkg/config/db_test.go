package config

import (
	"os"
	"testing"

	"github.com/spf13/pflag"
)

// TestNewDatabaseConfig_Defaults tests default configuration values
func TestNewDatabaseConfig_Defaults(t *testing.T) {
	cfg := NewDatabaseConfig()

	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"Dialect", cfg.Dialect, "postgres"},
		{"SSLMode", cfg.SSLMode, "disable"},
		{"Debug", cfg.Debug, false},
		{"MaxOpenConnections", cfg.MaxOpenConnections, 50},
		{"AdvisoryLockTimeoutSeconds", cfg.AdvisoryLockTimeoutSeconds, 300},
		{"MaxIdleConnections", cfg.MaxIdleConnections, 10},
		{"ConnRetryAttempts", cfg.ConnRetryAttempts, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, tt.got)
			}
		})
	}
}

// TestDatabaseConfig_AddFlags tests CLI flag registration
func TestDatabaseConfig_AddFlags(t *testing.T) {
	cfg := NewDatabaseConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)

	cfg.AddFlags(fs)

	// Verify flags are registered
	flags := []string{
		"db-host-file",
		"db-port-file",
		"db-user-file",
		"db-password-file",
		"db-name-file",
		"db-sslmode",
		"enable-db-debug",
		"db-max-open-connections",
		"db-advisory-lock-timeout",
	}
	for _, flagName := range flags {
		t.Run("flag_"+flagName, func(t *testing.T) {
			if fs.Lookup(flagName) == nil {
				t.Errorf("expected %s flag to be registered", flagName)
			}
		})
	}

	// Test flag parsing for advisory lock timeout
	tests := []struct {
		name     string
		args     []string
		expected int
	}{
		{
			name:     "default advisory lock timeout",
			args:     []string{},
			expected: 300,
		},
		{
			name:     "custom advisory lock timeout",
			args:     []string{"--db-advisory-lock-timeout=600"},
			expected: 600,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := NewDatabaseConfig()
			fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
			cfg.AddFlags(fs)

			if err := fs.Parse(tt.args); err != nil {
				t.Fatalf("failed to parse flags: %v", err)
			}

			if cfg.AdvisoryLockTimeoutSeconds != tt.expected {
				t.Errorf("expected AdvisoryLockTimeoutSeconds %d, got %d", tt.expected, cfg.AdvisoryLockTimeoutSeconds)
			}
		})
	}
}

// TestDatabaseConfig_BindEnv tests environment variable binding
func TestDatabaseConfig_BindEnv(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		validate func(*testing.T, *DatabaseConfig)
	}{
		{
			name: "valid advisory lock timeout",
			envVars: map[string]string{
				"DB_ADVISORY_LOCK_TIMEOUT": "600",
			},
			validate: func(t *testing.T, cfg *DatabaseConfig) {
				if cfg.AdvisoryLockTimeoutSeconds != 600 {
					t.Errorf("expected AdvisoryLockTimeoutSeconds 600, got %d", cfg.AdvisoryLockTimeoutSeconds)
				}
			},
		},
		{
			name: "valid db debug true",
			envVars: map[string]string{
				"DB_DEBUG": "true",
			},
			validate: func(t *testing.T, cfg *DatabaseConfig) {
				if cfg.Debug != true {
					t.Errorf("expected Debug true, got %t", cfg.Debug)
				}
			},
		},
		{
			name: "valid db debug false",
			envVars: map[string]string{
				"DB_DEBUG": "false",
			},
			validate: func(t *testing.T, cfg *DatabaseConfig) {
				if cfg.Debug != false {
					t.Errorf("expected Debug false, got %t", cfg.Debug)
				}
			},
		},
		{
			name: "zero timeout keeps default",
			envVars: map[string]string{
				"DB_ADVISORY_LOCK_TIMEOUT": "0",
			},
			validate: func(t *testing.T, cfg *DatabaseConfig) {
				if cfg.AdvisoryLockTimeoutSeconds != 300 {
					t.Errorf("expected AdvisoryLockTimeoutSeconds to keep default (300), got %d", cfg.AdvisoryLockTimeoutSeconds)
				}
			},
		},
		{
			name: "negative timeout keeps default",
			envVars: map[string]string{
				"DB_ADVISORY_LOCK_TIMEOUT": "-1",
			},
			validate: func(t *testing.T, cfg *DatabaseConfig) {
				if cfg.AdvisoryLockTimeoutSeconds != 300 {
					t.Errorf("expected AdvisoryLockTimeoutSeconds to keep default (300), got %d", cfg.AdvisoryLockTimeoutSeconds)
				}
			},
		},
		{
			name: "invalid timeout string keeps default",
			envVars: map[string]string{
				"DB_ADVISORY_LOCK_TIMEOUT": "abc",
			},
			validate: func(t *testing.T, cfg *DatabaseConfig) {
				if cfg.AdvisoryLockTimeoutSeconds != 300 {
					t.Errorf("expected AdvisoryLockTimeoutSeconds to keep default (300), got %d", cfg.AdvisoryLockTimeoutSeconds)
				}
			},
		},
		{
			name: "invalid bool value keeps default",
			envVars: map[string]string{
				"DB_DEBUG": "not-a-bool",
			},
			validate: func(t *testing.T, cfg *DatabaseConfig) {
				if cfg.Debug != false {
					t.Errorf("expected Debug to keep default (false), got %t", cfg.Debug)
				}
			},
		},
		{
			name: "empty timeout keeps default",
			envVars: map[string]string{
				"DB_ADVISORY_LOCK_TIMEOUT": "",
			},
			validate: func(t *testing.T, cfg *DatabaseConfig) {
				if cfg.AdvisoryLockTimeoutSeconds != 300 {
					t.Errorf("expected AdvisoryLockTimeoutSeconds to keep default (300), got %d", cfg.AdvisoryLockTimeoutSeconds)
				}
			},
		},
		{
			name: "empty debug keeps default",
			envVars: map[string]string{
				"DB_DEBUG": "",
			},
			validate: func(t *testing.T, cfg *DatabaseConfig) {
				if cfg.Debug != false {
					t.Errorf("expected Debug to keep default (false), got %t", cfg.Debug)
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
				if val != "" {
					if err := os.Setenv(key, val); err != nil {
						t.Fatalf("failed to set env var %s: %v", key, err)
					}
				} else {
					_ = os.Unsetenv(key)
				}
			}

			cfg := NewDatabaseConfig()
			cfg.BindEnv(nil)

			tt.validate(t, cfg)
		})
	}
}

// TestDatabaseConfig_FlagsOverrideEnv tests that CLI flags override environment variables
func TestDatabaseConfig_FlagsOverrideEnv(t *testing.T) {
	// Save and restore env var
	oldTimeout := os.Getenv("DB_ADVISORY_LOCK_TIMEOUT")
	defer func() {
		if oldTimeout == "" {
			_ = os.Unsetenv("DB_ADVISORY_LOCK_TIMEOUT")
		} else {
			_ = os.Setenv("DB_ADVISORY_LOCK_TIMEOUT", oldTimeout)
		}
	}()

	// Set env var to "600"
	if err := os.Setenv("DB_ADVISORY_LOCK_TIMEOUT", "600"); err != nil {
		t.Fatalf("failed to set DB_ADVISORY_LOCK_TIMEOUT: %v", err)
	}

	cfg := NewDatabaseConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.AddFlags(fs)

	// Parse flags with different value
	args := []string{"--db-advisory-lock-timeout=120"}
	if err := fs.Parse(args); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	// Before BindEnv, should have flag value
	if cfg.AdvisoryLockTimeoutSeconds != 120 {
		t.Errorf("expected AdvisoryLockTimeoutSeconds 120 from flag, got %d", cfg.AdvisoryLockTimeoutSeconds)
	}

	// After BindEnv, flag should take priority over env var
	cfg.BindEnv(fs)
	if cfg.AdvisoryLockTimeoutSeconds != 120 {
		t.Errorf("expected AdvisoryLockTimeoutSeconds 120 (flag > env), got %d", cfg.AdvisoryLockTimeoutSeconds)
	}
}

// TestDatabaseConfig_EnvOverridesDefaults tests that env vars override defaults when no flag is set
func TestDatabaseConfig_EnvOverridesDefaults(t *testing.T) {
	// Save and restore env var
	oldTimeout := os.Getenv("DB_ADVISORY_LOCK_TIMEOUT")
	defer func() {
		if oldTimeout == "" {
			_ = os.Unsetenv("DB_ADVISORY_LOCK_TIMEOUT")
		} else {
			_ = os.Setenv("DB_ADVISORY_LOCK_TIMEOUT", oldTimeout)
		}
	}()

	// Set env var
	if err := os.Setenv("DB_ADVISORY_LOCK_TIMEOUT", "450"); err != nil {
		t.Fatalf("failed to set DB_ADVISORY_LOCK_TIMEOUT: %v", err)
	}

	cfg := NewDatabaseConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.AddFlags(fs)

	// Parse empty args (no flags set)
	if err := fs.Parse([]string{}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	// Before BindEnv, should have default value
	if cfg.AdvisoryLockTimeoutSeconds != 300 {
		t.Errorf("expected AdvisoryLockTimeoutSeconds 300 (default), got %d", cfg.AdvisoryLockTimeoutSeconds)
	}

	// After BindEnv, env var should override default
	cfg.BindEnv(fs)
	if cfg.AdvisoryLockTimeoutSeconds != 450 {
		t.Errorf("expected AdvisoryLockTimeoutSeconds 450 (env > default), got %d", cfg.AdvisoryLockTimeoutSeconds)
	}
}

// TestDatabaseConfig_PriorityMixed tests priority with multiple fields and mixed sources
func TestDatabaseConfig_PriorityMixed(t *testing.T) {
	// Save and restore env vars
	envVars := map[string]string{
		"DB_ADVISORY_LOCK_TIMEOUT": os.Getenv("DB_ADVISORY_LOCK_TIMEOUT"),
		"DB_DEBUG":                 os.Getenv("DB_DEBUG"),
	}
	defer func() {
		for key, val := range envVars {
			if val == "" {
				_ = os.Unsetenv(key)
			} else {
				_ = os.Setenv(key, val)
			}
		}
	}()

	// Set env vars for both fields
	_ = os.Setenv("DB_ADVISORY_LOCK_TIMEOUT", "600")
	_ = os.Setenv("DB_DEBUG", "true")

	cfg := NewDatabaseConfig()
	fs := pflag.NewFlagSet("test", pflag.ContinueOnError)
	cfg.AddFlags(fs)

	// Only set flag for advisory lock timeout
	if err := fs.Parse([]string{"--db-advisory-lock-timeout=240"}); err != nil {
		t.Fatalf("failed to parse flags: %v", err)
	}

	cfg.BindEnv(fs)

	// advisory lock timeout: flag wins over env
	if cfg.AdvisoryLockTimeoutSeconds != 240 {
		t.Errorf("expected AdvisoryLockTimeoutSeconds 240 (flag > env), got %d", cfg.AdvisoryLockTimeoutSeconds)
	}
	// db debug: env wins over default
	if cfg.Debug != true {
		t.Errorf("expected Debug true (env > default), got %t", cfg.Debug)
	}
}

// TestDatabaseConfig_InvalidEnvHandling documents that invalid env values are silently ignored
func TestDatabaseConfig_InvalidEnvHandling(t *testing.T) {
	tests := []struct {
		name        string
		envVar      string
		envValue    string
		description string
	}{
		{
			name:        "zero timeout",
			envVar:      "DB_ADVISORY_LOCK_TIMEOUT",
			envValue:    "0",
			description: "Zero timeout is rejected by validation (timeout > 0), keeps default",
		},
		{
			name:        "negative timeout",
			envVar:      "DB_ADVISORY_LOCK_TIMEOUT",
			envValue:    "-1",
			description: "Negative timeout is rejected by validation (timeout > 0), keeps default",
		},
		{
			name:        "non-numeric timeout",
			envVar:      "DB_ADVISORY_LOCK_TIMEOUT",
			envValue:    "abc",
			description: "Non-numeric value fails strconv.Atoi, keeps default",
		},
		{
			name:        "invalid bool",
			envVar:      "DB_DEBUG",
			envValue:    "not-a-bool",
			description: "Invalid bool fails strconv.ParseBool, keeps default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and restore env var
			oldVal := os.Getenv(tt.envVar)
			defer func() {
				if oldVal == "" {
					_ = os.Unsetenv(tt.envVar)
				} else {
					_ = os.Setenv(tt.envVar, oldVal)
				}
			}()

			if err := os.Setenv(tt.envVar, tt.envValue); err != nil {
				t.Fatalf("failed to set %s: %v", tt.envVar, err)
			}

			cfg := NewDatabaseConfig()
			cfg.BindEnv(nil)

			// Document the behavior: invalid values are silently ignored
			switch tt.envVar {
			case "DB_ADVISORY_LOCK_TIMEOUT":
				if cfg.AdvisoryLockTimeoutSeconds != 300 {
					t.Errorf(
						"expected default AdvisoryLockTimeoutSeconds (300) after invalid env, got %d",
						cfg.AdvisoryLockTimeoutSeconds)
				}
				t.Logf("INFO: %s - invalid value silently ignored, kept default 300", tt.description)
			case "DB_DEBUG":
				if cfg.Debug != false {
					t.Errorf("expected default Debug (false) after invalid env, got %t", cfg.Debug)
				}
				t.Logf("INFO: %s - invalid value silently ignored, kept default false", tt.description)
			}
		})
	}
}
