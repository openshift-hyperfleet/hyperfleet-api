package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// CoreshiftConfig configures the coreshift adapter.
type CoreshiftConfig struct {
	Binary         string `mapstructure:"binary" json:"binary" validate:"required"`
	PullSecretFile string `mapstructure:"pull_secret_file" json:"pull_secret_file"`
	StateDir       string `mapstructure:"state_dir" json:"state_dir"`
}

// NewCoreshiftConfig returns default CoreshiftConfig values.
func NewCoreshiftConfig() *CoreshiftConfig {
	stateDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		stateDir = filepath.Join(home, ".local", "state", "hyperfleet", "coreshift")
	}
	return &CoreshiftConfig{
		Binary:   "coreshift",
		StateDir: stateDir,
	}
}

// Validate rejects an empty binary name.
func (c *CoreshiftConfig) Validate() error {
	if c.Binary == "" {
		return fmt.Errorf("binary is required")
	}
	return nil
}
