package config

import (
	"fmt"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type ServerConfig struct {
	Hostname      string        `mapstructure:"hostname" json:"hostname" validate:""`
	Host          string        `mapstructure:"host" json:"host" validate:"required"`
	Port          int           `mapstructure:"port" json:"port" validate:"required,min=1,max=65535"`
	Timeout       TimeoutConfig `mapstructure:"timeout" json:"timeout"`
	HTTPS         HTTPSConfig   `mapstructure:"https" json:"https"`
	Auth          AuthConfig    `mapstructure:"auth" json:"auth"`

	// Legacy field for backward compatibility (combines host:port)
	BindAddress string `mapstructure:"bind_address" json:"bind_address,omitempty" validate:""`
}

type TimeoutConfig struct {
	Read  time.Duration `mapstructure:"read" json:"read" validate:""`
	Write time.Duration `mapstructure:"write" json:"write" validate:""`
}

type HTTPSConfig struct {
	Enabled  bool   `mapstructure:"enabled" json:"enabled"`
	CertFile string `mapstructure:"cert_file" json:"cert_file"`
	KeyFile  string `mapstructure:"key_file" json:"key_file"`
}

type AuthConfig struct {
	JWT   JWTConfig   `mapstructure:"jwt" json:"jwt"`
	Authz AuthzConfig `mapstructure:"authz" json:"authz"`
}

type JWTConfig struct {
	Enabled  bool   `mapstructure:"enabled" json:"enabled"`
	CertFile string `mapstructure:"cert_file" json:"cert_file"`
	CertURL  string `mapstructure:"cert_url" json:"cert_url"`
}

type AuthzConfig struct {
	Enabled bool   `mapstructure:"enabled" json:"enabled"`
	ACLFile string `mapstructure:"acl_file" json:"acl_file"`
}

func NewServerConfig() *ServerConfig {
	return &ServerConfig{
		Hostname: "",
		Host:     "localhost",
		Port:     8000,
		Timeout: TimeoutConfig{
			Read:  5 * time.Second,
			Write: 30 * time.Second,
		},
		HTTPS: HTTPSConfig{
			Enabled:  false,
			CertFile: "",
			KeyFile:  "",
		},
		Auth: AuthConfig{
			JWT: JWTConfig{
				Enabled:  true,
				CertFile: "",
				CertURL:  "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/certs",
			},
			Authz: AuthzConfig{
				Enabled: true,
				ACLFile: "",
			},
		},
		BindAddress: "localhost:8000",
	}
}

// defineAndBindFlags defines & binds flags to viper keys in a single pass
func (s *ServerConfig) defineAndBindFlags(v *viper.Viper, fs *pflag.FlagSet) {
	// Server flags
	defineAndBindStringFlag(v, fs, "server.host", "server-host", "", s.Host, "Server bind host")
	defineAndBindIntFlag(v, fs, "server.port", "server-port", "p", s.Port, "Server bind port")
	defineAndBindStringFlag(v, fs, "server.hostname", "server-hostname", "", s.Hostname, "Server's public hostname")

	// Timeout flags
	defineAndBindDurationFlag(v, fs, "server.timeout.read", "server-timeout-read", "", s.Timeout.Read, "HTTP server read timeout")
	defineAndBindDurationFlag(v, fs, "server.timeout.write", "server-timeout-write", "", s.Timeout.Write, "HTTP server write timeout")

	// HTTPS flags
	defineAndBindBoolFlag(v, fs, "server.https.enabled", "server-https-enabled", "", s.HTTPS.Enabled, "Enable HTTPS rather than HTTP")
	defineAndBindStringFlag(v, fs, "server.https.cert_file", "server-https-cert-file", "", s.HTTPS.CertFile, "Path to the tls.crt file")
	defineAndBindStringFlag(v, fs, "server.https.key_file", "server-https-key-file", "", s.HTTPS.KeyFile, "Path to the tls.key file")

	// JWT flags
	defineAndBindBoolFlag(v, fs, "server.auth.jwt.enabled", "auth-jwt-enabled", "", s.Auth.JWT.Enabled, "Enable JWT authentication validation")
	defineAndBindStringFlag(v, fs, "server.auth.jwt.cert_file", "auth-jwt-cert-file", "", s.Auth.JWT.CertFile, "JWK Certificate file")
	defineAndBindStringFlag(v, fs, "server.auth.jwt.cert_url", "auth-jwt-cert-url", "", s.Auth.JWT.CertURL, "JWK Certificate URL")

	// Authz flags
	defineAndBindBoolFlag(v, fs, "server.auth.authz.enabled", "auth-authz-enabled", "", s.Auth.Authz.Enabled, "Enable Authorization on endpoints")
	defineAndBindStringFlag(v, fs, "server.auth.authz.acl_file", "auth-authz-acl-file", "", s.Auth.Authz.ACLFile, "Access control list file")
}

// GetBindAddress returns the bind address in host:port format
func (s *ServerConfig) GetBindAddress() string {
	if s.BindAddress != "" {
		return s.BindAddress
	}
	return fmt.Sprintf("%s:%d", s.Host, s.Port)
}
