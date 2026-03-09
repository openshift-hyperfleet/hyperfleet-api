package config

// AdapterRequirementsConfig configures which adapters must be ready for resources
// Follows HyperFleet Configuration Standard
type AdapterRequirementsConfig struct {
	Required RequiredAdapters `mapstructure:"required" json:"required" validate:"required"`
}

// RequiredAdapters holds required adapters for different resource types
type RequiredAdapters struct {
	Cluster  []string `mapstructure:"cluster" json:"cluster" validate:"dive,required"`
	Nodepool []string `mapstructure:"nodepool" json:"nodepool" validate:"dive,required"`
}

// NewAdapterRequirementsConfig returns default AdapterRequirementsConfig values
// Note: This config typically REQUIRES values from environment or config file
// Empty defaults are provided but should be overridden
func NewAdapterRequirementsConfig() *AdapterRequirementsConfig {
	return &AdapterRequirementsConfig{
		Required: RequiredAdapters{
			Cluster:  []string{}, // Set via config file, env vars (JSON array), or CLI flags
			Nodepool: []string{}, // Set via config file, env vars (JSON array), or CLI flags
		},
	}
}

// ============================================================
// BACKWARD COMPATIBILITY HELPERS
// ============================================================

// RequiredClusterAdapters returns cluster adapters (legacy accessor)
func (a *AdapterRequirementsConfig) RequiredClusterAdapters() []string {
	return a.Required.Cluster
}

// RequiredNodePoolAdapters returns nodepool adapters (legacy accessor)
func (a *AdapterRequirementsConfig) RequiredNodePoolAdapters() []string {
	return a.Required.Nodepool
}
