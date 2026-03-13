package config

import (
	"fmt"
)

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
		config.Server.TLS.Enabled,
		config.Server.JWT.Enabled,
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
		config.OCM.Mock.Enabled,
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
