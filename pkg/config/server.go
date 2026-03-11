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
	Hostname string         `mapstructure:"hostname" json:"hostname" validate:"omitempty,hostname|ip"`
	Host     string         `mapstructure:"host" json:"host" validate:"required"`
	Port     int            `mapstructure:"port" json:"port" validate:"required,min=1,max=65535"`
	Timeouts TimeoutsConfig `mapstructure:"timeouts" json:"timeouts" validate:"required"`
	TLS      TLSConfig      `mapstructure:"tls" json:"tls" validate:"required"`
	JWT      JWTConfig      `mapstructure:"jwt" json:"jwt" validate:"required"`
	JWK      JWKConfig      `mapstructure:"jwk" json:"jwk" validate:"required"`
	Authz    AuthzConfig    `mapstructure:"authz" json:"authz" validate:"required"`
	ACL      ACLConfig      `mapstructure:"acl" json:"acl" validate:"required"`
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
	Enabled  bool   `mapstructure:"enabled" json:"enabled"`
	CertFile string `mapstructure:"cert_file" json:"cert_file" validate:"omitempty,filepath"`
	KeyFile  string `mapstructure:"key_file" json:"key_file" validate:"omitempty,filepath"`
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
		Hostname: "",
		Host:     "localhost",
		Port:     8000,
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

// ReadTimeout returns read timeout
func (s *ServerConfig) ReadTimeout() time.Duration {
	return s.Timeouts.Read
}

// WriteTimeout returns write timeout
func (s *ServerConfig) WriteTimeout() time.Duration {
	return s.Timeouts.Write
}

// EnableHTTPS returns TLS enabled flag
func (s *ServerConfig) EnableHTTPS() bool {
	return s.TLS.Enabled
}

// HTTPSCertFile returns TLS cert file
func (s *ServerConfig) HTTPSCertFile() string {
	return s.TLS.CertFile
}

// HTTPSKeyFile returns TLS key file
func (s *ServerConfig) HTTPSKeyFile() string {
	return s.TLS.KeyFile
}

// EnableJWT returns JWT enabled flag
func (s *ServerConfig) EnableJWT() bool {
	return s.JWT.Enabled
}

// EnableAuthz returns authz enabled flag
func (s *ServerConfig) EnableAuthz() bool {
	return s.Authz.Enabled
}

// JwkCertFile returns JWK cert file
func (s *ServerConfig) JwkCertFile() string {
	return s.JWK.CertFile
}

// JwkCertURL returns JWK cert URL
func (s *ServerConfig) JwkCertURL() string {
	return s.JWK.CertURL
}

// ACLFile returns ACL file
func (s *ServerConfig) ACLFile() string {
	return s.ACL.File
}
