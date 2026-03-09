package config

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// IsNewConfigEnabled checks if the new configuration system should be used
// Controlled by HYPERFLEET_USE_NEW_CONFIG environment variable
// Default: false (use old system for safety)
func IsNewConfigEnabled() bool {
	value := os.Getenv("HYPERFLEET_USE_NEW_CONFIG")
	return value == "true" || value == "1"
}

// LoadConfigWithMigration loads configuration using the appropriate system
// based on HYPERFLEET_USE_NEW_CONFIG feature flag.
// When enabled, it loads using the new Viper-based system.
// When disabled, it falls back to the old system.
// In both cases, it validates configuration equivalence for safety.
// The applyEnvOverrides callback is called after loading but before verification to apply
// environment-specific overrides (e.g., development mode disabling JWT/TLS).
func LoadConfigWithMigration(
	ctx context.Context,
	cmd *cobra.Command,
	oldConfig *ApplicationConfig,
	applyEnvOverrides func(*ApplicationConfig) error,
) (*ApplicationConfig, error) {
	useNewConfig := IsNewConfigEnabled()

	logger.With(ctx, "use_new_config", useNewConfig).Info("Configuration system selection")

	if !useNewConfig {
		// Use old configuration system (current production behavior)
		logger.Info(ctx, "Using legacy configuration system (HYPERFLEET_USE_NEW_CONFIG not set)")
		return oldConfig, nil
	}

	// Load configuration using new system
	logger.Info(ctx, "Using new Viper-based configuration system")
	loader := NewConfigLoader()
	newConfig, err := loader.Load(ctx, cmd)
	if err != nil {
		return nil, fmt.Errorf("new config system failed to load: %w", err)
	}

	// Apply environment-specific overrides (e.g., development mode settings)
	// This must happen before verification to match legacy system behavior
	if applyEnvOverrides != nil {
		if err := applyEnvOverrides(newConfig); err != nil {
			logger.WithError(ctx, err).Error("Failed to apply environment overrides")
			return nil, fmt.Errorf("failed to apply environment overrides: %w", err)
		}
	}

	// Verify configuration equivalence for critical fields
	if err := VerifyConfigEquivalence(ctx, oldConfig, newConfig); err != nil {
		logger.WithError(ctx, err).Warn("Configuration equivalence check failed - differences detected")
		// Log but don't fail - allow new config to be used
	} else {
		logger.Info(ctx, "Configuration equivalence verified - old and new systems produce same config")
	}

	return newConfig, nil
}

// VerifyConfigEquivalence compares old and new configuration for critical fields
// This helps ensure the migration doesn't introduce subtle bugs
func VerifyConfigEquivalence(ctx context.Context, oldConfig, newConfig *ApplicationConfig) error {
	if oldConfig == nil || newConfig == nil {
		return fmt.Errorf("cannot compare nil configs")
	}

	var diffs []string

	// Compare Server config
	if oldConfig.Server.BindAddress() != newConfig.Server.BindAddress() {
		diffs = append(diffs, fmt.Sprintf("Server.BindAddress: old=%s new=%s",
			oldConfig.Server.BindAddress(), newConfig.Server.BindAddress()))
	}
	if oldConfig.Server.EnableJWT() != newConfig.Server.EnableJWT() {
		diffs = append(diffs, fmt.Sprintf("Server.EnableJWT: old=%t new=%t",
			oldConfig.Server.EnableJWT(), newConfig.Server.EnableJWT()))
	}

	// Compare Database config
	if oldConfig.Database.Host != newConfig.Database.Host {
		diffs = append(diffs, fmt.Sprintf("Database.Host: old=%s new=%s",
			oldConfig.Database.Host, newConfig.Database.Host))
	}
	if oldConfig.Database.Port != newConfig.Database.Port {
		diffs = append(diffs, fmt.Sprintf("Database.Port: old=%d new=%d",
			oldConfig.Database.Port, newConfig.Database.Port))
	}
	if oldConfig.Database.Name != newConfig.Database.Name {
		diffs = append(diffs, fmt.Sprintf("Database.Name: old=%s new=%s",
			oldConfig.Database.Name, newConfig.Database.Name))
	}

	// Compare Logging config
	if oldConfig.Logging.Level != newConfig.Logging.Level {
		diffs = append(diffs, fmt.Sprintf("Logging.Level: old=%s new=%s",
			oldConfig.Logging.Level, newConfig.Logging.Level))
	}
	if oldConfig.Logging.Format != newConfig.Logging.Format {
		diffs = append(diffs, fmt.Sprintf("Logging.Format: old=%s new=%s",
			oldConfig.Logging.Format, newConfig.Logging.Format))
	}

	// Compare OCM config
	if oldConfig.OCM.BaseURL != newConfig.OCM.BaseURL {
		diffs = append(diffs, fmt.Sprintf("OCM.BaseURL: old=%s new=%s",
			oldConfig.OCM.BaseURL, newConfig.OCM.BaseURL))
	}

	// Compare Adapters config (if both are loaded)
	if oldConfig.Adapters != nil && newConfig.Adapters != nil {
		if !stringSlicesEqual(oldConfig.Adapters.RequiredClusterAdapters(), newConfig.Adapters.RequiredClusterAdapters()) {
			diffs = append(diffs, fmt.Sprintf("Adapters.RequiredClusterAdapters: old=%v new=%v",
				oldConfig.Adapters.RequiredClusterAdapters(), newConfig.Adapters.RequiredClusterAdapters()))
		}
		if !stringSlicesEqual(oldConfig.Adapters.RequiredNodePoolAdapters(), newConfig.Adapters.RequiredNodePoolAdapters()) {
			diffs = append(diffs, fmt.Sprintf("Adapters.RequiredNodePoolAdapters: old=%v new=%v",
				oldConfig.Adapters.RequiredNodePoolAdapters(), newConfig.Adapters.RequiredNodePoolAdapters()))
		}
	}

	if len(diffs) > 0 {
		logger.Warn(ctx, "Configuration differences detected:")
		for _, diff := range diffs {
			logger.Warn(ctx, "  "+diff)
		}
		return fmt.Errorf("found %d configuration differences", len(diffs))
	}

	return nil
}

// stringSlicesEqual compares two string slices for equality
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// DumpConfig returns a human-readable string representation of configuration
// with sensitive fields redacted. Useful for debugging.
func DumpConfig(config *ApplicationConfig) string {
	if config == nil {
		return "nil config"
	}

	return fmt.Sprintf(`ApplicationConfig:
  Server:
    BindAddress: %s
    EnableHTTPS: %t
    EnableJWT: %t
  Database:
    Host: %s
    Port: %d
    Name: %s
    Username: %s
    Password: %s
    Debug: %t
  Logging:
    Level: %s
    Format: %s
    OTel.Enabled: %t
  OCM:
    BaseURL: %s
    EnableMock: %t
  Metrics:
    BindAddress: %s
  Health:
    BindAddress: %s
  Adapters:
    ClusterAdapters: %v
    NodePoolAdapters: %v
`,
		config.Server.BindAddress(),
		config.Server.EnableHTTPS(),
		config.Server.EnableJWT(),
		config.Database.Host,
		config.Database.Port,
		config.Database.Name,
		redactIfSet(config.Database.Username),
		redactIfSet(config.Database.Password),
		config.Database.Debug,
		config.Logging.Level,
		config.Logging.Format,
		config.Logging.OTel.Enabled,
		config.OCM.BaseURL,
		config.OCM.EnableMock(),
		config.Metrics.BindAddress(),
		config.Health.BindAddress(),
		safeAdapterList(config.Adapters, true),
		safeAdapterList(config.Adapters, false),
	)
}

// safeAdapterList safely extracts adapter list, handling nil config
func safeAdapterList(adapters *AdapterRequirementsConfig, cluster bool) []string {
	if adapters == nil {
		return []string{}
	}
	if cluster {
		return adapters.RequiredClusterAdapters()
	}
	return adapters.RequiredNodePoolAdapters()
}

// ValidateConfigTransition validates that switching from old to new config is safe
// Returns warnings if there are non-critical differences, error if critical issues found
func ValidateConfigTransition(
	ctx context.Context, oldConfig, newConfig *ApplicationConfig,
) (warnings []string, err error) {
	// Critical validations - these must match
	if oldConfig.Database.Host != newConfig.Database.Host ||
		oldConfig.Database.Port != newConfig.Database.Port ||
		oldConfig.Database.Name != newConfig.Database.Name {
		return nil, fmt.Errorf("database connection parameters differ - this could break the service")
	}

	if oldConfig.Server.BindAddress() != newConfig.Server.BindAddress() {
		return nil, fmt.Errorf("server bind address differs - this could break routing")
	}

	// Non-critical warnings
	if oldConfig.Logging.Level != newConfig.Logging.Level {
		warnings = append(warnings, fmt.Sprintf("Log level changed: %s → %s",
			oldConfig.Logging.Level, newConfig.Logging.Level))
	}

	if oldConfig.Database.Debug != newConfig.Database.Debug {
		warnings = append(warnings, fmt.Sprintf("DB debug mode changed: %t → %t",
			oldConfig.Database.Debug, newConfig.Database.Debug))
	}

	return warnings, nil
}
