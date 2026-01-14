package config

import (
	"time"

	"github.com/spf13/pflag"
)

type HealthConfig struct {
	BindAddress     string        `json:"bind_address"`
	EnableHTTPS     bool          `json:"enable_https"`
	ShutdownTimeout time.Duration `json:"shutdown_timeout"`
}

func NewHealthConfig() *HealthConfig {
	return &HealthConfig{
		BindAddress:     "localhost:8080",
		EnableHTTPS:     false,
		ShutdownTimeout: 20 * time.Second,
	}
}

func (s *HealthConfig) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&s.BindAddress, "health-server-bindaddress", s.BindAddress, "Health server bind address")
	fs.BoolVar(&s.EnableHTTPS, "enable-health-https", s.EnableHTTPS, "Enable HTTPS for health server")
	fs.DurationVar(&s.ShutdownTimeout, "health-shutdown-timeout", s.ShutdownTimeout, "Health server shutdown timeout")
}

func (s *HealthConfig) ReadFiles() error {
	return nil
}
