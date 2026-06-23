package config

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

// ServerConfig holds HTTP/HTTPS server configuration
// Follows HyperFleet Configuration Standard
type ServerConfig struct {
	Hostname          string         `mapstructure:"hostname" json:"hostname" validate:"omitempty,hostname|ip"`
	Host              string         `mapstructure:"host" json:"host" validate:"required,hostname|ip"`
	OpenAPISchemaPath string         `mapstructure:"openapi_schema_path" json:"openapi_schema_path"`
	JWK               JWKConfig      `mapstructure:"jwk" json:"jwk" validate:"required"`
	TLS               TLSConfig      `mapstructure:"tls" json:"tls" validate:"required"`
	JWT               JWTConfig      `mapstructure:"jwt" json:"jwt" validate:"required"`
	Timeouts          TimeoutsConfig `mapstructure:"timeouts" json:"timeouts" validate:"required"`
	Port              int            `mapstructure:"port" json:"port" validate:"required,min=1,max=65535"`
}

// TimeoutsConfig holds HTTP timeout configuration
type TimeoutsConfig struct {
	Read  time.Duration `mapstructure:"read" json:"read" validate:"required"`
	Write time.Duration `mapstructure:"write" json:"write" validate:"required"`
}

// Validate validates timeout durations
func (c *TimeoutsConfig) Validate() error {
	if c.Read < 1*time.Second {
		return fmt.Errorf("read timeout must be at least 1 second, got %v", c.Read)
	}
	if c.Write < 1*time.Second {
		return fmt.Errorf("write timeout must be at least 1 second, got %v", c.Write)
	}
	return nil
}

// TLSConfig holds TLS configuration
type TLSConfig struct {
	CertFile string `mapstructure:"cert_file" json:"cert_file" validate:"omitempty,filepath"`
	KeyFile  string `mapstructure:"key_file" json:"key_file" validate:"omitempty,filepath"`
	Enabled  bool   `mapstructure:"enabled" json:"enabled"`
}

// Validate validates TLS configuration
func (c *TLSConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	// When TLS is enabled, both cert and key files must be provided
	if c.CertFile == "" {
		return fmt.Errorf("TLS cert file is required when TLS is enabled")
	}
	if c.KeyFile == "" {
		return fmt.Errorf("TLS key file is required when TLS is enabled")
	}
	return nil
}

// JWTIssuerConfig holds per-issuer JWT authentication and identity configuration.
// Each entry represents a distinct identity provider. A request is accepted if
// its token validates against any entry.
type JWTIssuerConfig struct {
	IssuerURL            string `mapstructure:"issuer_url" json:"issuer_url"`
	Audience             string `mapstructure:"audience" json:"audience"`
	JWKCertFile          string `mapstructure:"jwk_cert_file" json:"jwk_cert_file"`
	JWKCertURL           string `mapstructure:"jwk_cert_url" json:"jwk_cert_url"`
	IdentityClaim        string `mapstructure:"identity_claim" json:"identity_claim"`
	IdentityClaimPattern string `mapstructure:"identity_claim_pattern" json:"identity_claim_pattern"`
	Header               string `mapstructure:"header" json:"header"`
}

// JWTConfig holds JWT authentication configuration
type JWTConfig struct {
	Configs []JWTIssuerConfig `mapstructure:"configs" json:"configs"`
	Enabled bool              `mapstructure:"enabled" json:"enabled"`
}

func (c *JWTConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	// Allow empty configs at load time; NewJWTHandler validates before starting.
	for i, cfg := range c.Configs {
		if cfg.IssuerURL == "" {
			return fmt.Errorf("server.jwt.configs[%d].issuer_url is required", i)
		}
		if cfg.IdentityClaim == "" {
			return fmt.Errorf("server.jwt.configs[%d].identity_claim is required", i)
		}
	}
	return nil
}

// JWKConfig holds JWK certificate configuration (used for the identity-header-only dev mode)
type JWKConfig struct {
	CertFile string `mapstructure:"cert_file" json:"cert_file" validate:"omitempty,filepath"`
	CertURL  string `mapstructure:"cert_url" json:"cert_url" validate:"omitempty,url"`
}

// NewServerConfig returns default ServerConfig values
// These defaults can be overridden by config file, env vars, or CLI flags
func NewServerConfig() *ServerConfig {
	return &ServerConfig{
		Hostname:          "",
		Host:              "localhost",
		Port:              8000,
		OpenAPISchemaPath: "openapi/openapi.yaml",
		Timeouts: TimeoutsConfig{
			Read:  5 * time.Second,
			Write: 30 * time.Second,
		},
		TLS: TLSConfig{
			Enabled:  false,
			CertFile: "",
			KeyFile:  "",
		},
		JWT: JWTConfig{
			Enabled: true,
			Configs: []JWTIssuerConfig{},
		},
		JWK: JWKConfig{
			CertFile: "",
			CertURL:  "",
		},
	}
}

// ============================================================
// Convenience Accessor Methods
// ============================================================

// BindAddress returns bind address in host:port format
// Uses net.JoinHostPort to correctly handle IPv6 addresses
func (s *ServerConfig) BindAddress() string {
	return net.JoinHostPort(s.Host, strconv.Itoa(s.Port))
}
