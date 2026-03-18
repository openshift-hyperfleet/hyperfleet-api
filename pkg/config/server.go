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
	JWK               JWKConfig      `mapstructure:"jwk" json:"jwk" validate:"required"`
	Hostname          string         `mapstructure:"hostname" json:"hostname" validate:"omitempty,hostname|ip"`
	Host              string         `mapstructure:"host" json:"host" validate:"required,hostname|ip"`
	OpenAPISchemaPath string         `mapstructure:"openapi_schema_path" json:"openapi_schema_path"`
	ACL               ACLConfig      `mapstructure:"acl" json:"acl" validate:"required"`
	TLS               TLSConfig      `mapstructure:"tls" json:"tls" validate:"required"`
	Timeouts          TimeoutsConfig `mapstructure:"timeouts" json:"timeouts" validate:"required"`
	Port              int            `mapstructure:"port" json:"port" validate:"required,min=1,max=65535"`
	JWT               JWTConfig      `mapstructure:"jwt" json:"jwt" validate:"required"`
	Authz             AuthzConfig    `mapstructure:"authz" json:"authz" validate:"required"`
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

// JWTConfig holds JWT authentication configuration
type JWTConfig struct {
	Enabled bool `mapstructure:"enabled" json:"enabled"`
}

// JWKConfig holds JWK certificate configuration
type JWKConfig struct {
	CertFile string `mapstructure:"cert_file" json:"cert_file" validate:"omitempty,filepath"`
	CertURL  string `mapstructure:"cert_url" json:"cert_url" validate:"omitempty,url"`
}

// AuthzConfig holds authorization configuration
type AuthzConfig struct {
	Enabled bool `mapstructure:"enabled" json:"enabled"`
}

// ACLConfig holds access control list configuration
type ACLConfig struct {
	File string `mapstructure:"file" json:"file" validate:"omitempty,filepath"`
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
		JWK: JWKConfig{
			CertFile: "",
			CertURL:  "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/certs",
		},
		Authz: AuthzConfig{
			Enabled: true,
		},
		ACL: ACLConfig{
			File: "",
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
