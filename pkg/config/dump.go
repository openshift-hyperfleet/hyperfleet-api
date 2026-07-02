package config

import (
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
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
    IssuerURL: %s
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
  Metrics:
    BindAddress: %s
  Health:
    BindAddress: %s
  Adapters:
    ClusterAdapters: %v
    NodePoolAdapters: %v
  Entities: %d registered (kinds: %v)
`,
		config.Server.BindAddress(),
		config.Server.TLS.Enabled,
		config.Server.JWT.Enabled,
		config.Server.JWT.IssuerURL,
		config.Database.Host,
		config.Database.Port,
		config.Database.Name,
		redactIfSet(config.Database.Username),
		redactIfSet(config.Database.Password),
		config.Database.Debug,
		config.Logging.Level,
		config.Logging.Format,
		config.Logging.OTel.Enabled,
		config.Metrics.BindAddress(),
		config.Health.BindAddress(),
		safeAdapterList(config.Adapters, true),
		safeAdapterList(config.Adapters, false),
		len(config.Entities),
		entityKindNames(config.Entities),
	)
}

// entityKindNames extracts Kind strings from entity descriptors for logging.
func entityKindNames(entities []registry.EntityDescriptor) []string {
	kinds := make([]string, len(entities))
	for i, e := range entities {
		kinds[i] = e.Kind
	}
	return kinds
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
