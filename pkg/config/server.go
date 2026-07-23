package config

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validation"
)

// ServerConfig holds HTTP/HTTPS server configuration
// Follows HyperFleet Configuration Standard
type ServerConfig struct {
	Hostname          string         `mapstructure:"hostname" json:"hostname" validate:"omitempty,hostname|ip"`
	Host              string         `mapstructure:"host" json:"host" validate:"required,hostname|ip"`
	OpenAPISchemaPath string         `mapstructure:"openapi_schema_path" json:"openapi_schema_path"`
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

// JWTIssuerConfig holds configuration for a single JWT issuer (trust anchor).
// Each entry defines how to validate tokens from one identity provider.
type JWTIssuerConfig struct {
	CompiledPattern      *regexp.Regexp `mapstructure:"-" json:"-"`
	IssuerURL            string         `mapstructure:"issuer_url" json:"issuer_url" validate:"required,url"`
	JWKCertURL           string         `mapstructure:"jwk_cert_url" json:"jwk_cert_url" validate:"omitempty,url"`
	JWKCertFile          string         `mapstructure:"jwk_cert_file" json:"jwk_cert_file" validate:"omitempty,filepath"`
	JWKCertCAFile        string         `mapstructure:"jwk_cert_ca_file" json:"jwk_cert_ca_file" validate:"omitempty,filepath"` //nolint:lll
	Header               string         `mapstructure:"header" json:"header"`
	IdentityHeader       string         `mapstructure:"identity_header" json:"identity_header"`
	Audience             string         `mapstructure:"audience" json:"audience"`
	IdentityClaim        string         `mapstructure:"identity_claim" json:"identity_claim"`
	IdentityClaimPattern string         `mapstructure:"identity_claim_pattern" json:"identity_claim_pattern"`
}

// JWTConfig holds JWT authentication configuration with support for multiple issuers.
type JWTConfig struct {
	Configs []JWTIssuerConfig `mapstructure:"configs" json:"configs"`
	Enabled bool              `mapstructure:"enabled" json:"enabled"`
}

const (
	DefaultJWTHeader        = "Authorization"
	DefaultJWTIdentityClaim = "email"
)

// ApplyDefaults sets default values for optional fields in each issuer config.
// Call this after config loading and before Validate().
func (c *JWTConfig) ApplyDefaults() {
	for i := range c.Configs {
		if c.Configs[i].Header == "" {
			c.Configs[i].Header = DefaultJWTHeader
		}
		if c.Configs[i].IdentityClaim == "" {
			c.Configs[i].IdentityClaim = DefaultJWTIdentityClaim
		}
	}
}

func (c *JWTConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if len(c.Configs) == 0 {
		return fmt.Errorf("server.jwt.configs requires at least one issuer when jwt is enabled")
	}
	for i := range c.Configs {
		cfg := &c.Configs[i]

		if cfg.IssuerURL == "" {
			return fmt.Errorf("server.jwt.configs[%d].issuer_url is required", i)
		}
		if cfg.JWKCertURL == "" && cfg.JWKCertFile == "" {
			return fmt.Errorf("server.jwt.configs[%d] requires jwk_cert_url or jwk_cert_file", i)
		}
		if cfg.JWKCertCAFile != "" && cfg.JWKCertURL == "" {
			return fmt.Errorf("server.jwt.configs[%d].jwk_cert_ca_file requires jwk_cert_url to be set", i)
		}
		if cfg.Header != "" && validation.IsForbiddenJWTSourceHeaderName(cfg.Header) {
			return fmt.Errorf("server.jwt.configs[%d].header %q is not allowed as a JWT source", i, cfg.Header)
		}
		if cfg.IdentityHeader != "" {
			if validation.IsForbiddenIdentityHeaderName(cfg.IdentityHeader) {
				return fmt.Errorf("server.jwt.configs[%d].identity_header %q is not allowed", i, cfg.IdentityHeader)
			}
			if strings.EqualFold(cfg.Header, cfg.IdentityHeader) {
				return fmt.Errorf("server.jwt.configs[%d].identity_header must differ from header %q", i, cfg.Header)
			}
		}
		if cfg.IdentityClaimPattern != "" {
			re, err := regexp.Compile(cfg.IdentityClaimPattern)
			if err != nil {
				return fmt.Errorf("server.jwt.configs[%d].identity_claim_pattern is invalid: %w", i, err)
			}
			cfg.CompiledPattern = re
		}
	}
	return nil
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

func (s *ServerConfig) ReadTimeout() time.Duration {
	return s.Timeouts.Read
}

func (s *ServerConfig) WriteTimeout() time.Duration {
	return s.Timeouts.Write
}

func (s *ServerConfig) TLSEnabled() bool {
	return s.TLS.Enabled
}

func (s *ServerConfig) TLSCertFile() string {
	return s.TLS.CertFile
}

func (s *ServerConfig) TLSKeyFile() string {
	return s.TLS.KeyFile
}
