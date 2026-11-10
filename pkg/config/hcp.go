package config

import "fmt"

// HCPConfig configures the nested-coreshift hcp adapter.
type HCPConfig struct {
	// Image is the coreshift image already loaded on every management cluster.
	Image string `mapstructure:"image" json:"image"`
}

// NewHCPConfig returns default HCPConfig values.
func NewHCPConfig() *HCPConfig {
	return &HCPConfig{Image: "localhost/coreshift:latest"}
}

// Validate rejects an empty image reference.
func (c *HCPConfig) Validate() error {
	if c.Image == "" {
		return fmt.Errorf("image is required")
	}
	return nil
}
