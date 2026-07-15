package config

import (
	"fmt"
	"strings"

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
    Issuers: %d configured%s
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
  Entities: %d registered (kinds: %v)
`,
		config.Server.BindAddress(),
		config.Server.TLS.Enabled,
		config.Server.JWT.Enabled,
		len(config.Server.JWT.Configs),
		formatIssuers(config.Server.JWT.Configs),
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
		len(config.Entities),
		entityKindNames(config.Entities),
	)
}

func formatIssuers(configs []JWTIssuerConfig) string {
	if len(configs) == 0 {
		return ""
	}
	var b strings.Builder
	for i, cfg := range configs {
		jwkSource := "none"
		if cfg.JWKCertURL != "" {
			jwkSource = "url"
		} else if cfg.JWKCertFile != "" {
			jwkSource = "file"
		}
		caFlag := "no"
		if cfg.JWKCertCAFile != "" {
			caFlag = "yes"
		}
		fmt.Fprintf(&b, "\n      [%d] IssuerURL: %s, Header: %s, JWK: %s, CustomCA: %s",
			i, cfg.IssuerURL, cfg.Header, jwkSource, caFlag)
	}
	return b.String()
}

// entityKindNames extracts Kind strings from entity descriptors for logging.
func entityKindNames(entities []registry.EntityDescriptor) []string {
	kinds := make([]string, len(entities))
	for i, e := range entities {
		kinds[i] = e.Kind
	}
	return kinds
}
