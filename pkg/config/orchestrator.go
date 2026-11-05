package config

import (
	"fmt"
	"time"
)

// OrchestratorConfig controls the in-server drift detection loop.
type OrchestratorConfig struct {
	Enabled      bool          `mapstructure:"enabled" json:"enabled"`
	PollInterval time.Duration `mapstructure:"poll_interval" json:"poll_interval" validate:"required"`
}

// NewOrchestratorConfig returns default OrchestratorConfig values.
func NewOrchestratorConfig() *OrchestratorConfig {
	return &OrchestratorConfig{
		Enabled:      false,
		PollInterval: 5 * time.Second,
	}
}

// Validate rejects a non-positive poll interval.
func (o *OrchestratorConfig) Validate() error {
	if o.PollInterval <= 0 {
		return fmt.Errorf("poll interval must be positive, got %v", o.PollInterval)
	}
	return nil
}
