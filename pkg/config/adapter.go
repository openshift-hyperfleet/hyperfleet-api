package config

import (
	"encoding/json"
	"log"
	"os"
)

// AdapterRequirementsConfig configures which adapters must be ready for resources
type AdapterRequirementsConfig struct {
	RequiredClusterAdapters  []string
	RequiredNodePoolAdapters []string
}

// NewAdapterRequirementsConfig creates config from environment variables or defaults
func NewAdapterRequirementsConfig() *AdapterRequirementsConfig {
	config := &AdapterRequirementsConfig{
		RequiredClusterAdapters:  getDefaultClusterAdapters(),
		RequiredNodePoolAdapters: getDefaultNodePoolAdapters(),
	}

	config.LoadFromEnv()
	return config
}

// LoadFromEnv loads adapter lists from HYPERFLEET_CLUSTER_ADAPTERS and
// HYPERFLEET_NODEPOOL_ADAPTERS (JSON array format: '["adapter1","adapter2"]')
func (c *AdapterRequirementsConfig) LoadFromEnv() {
	if clusterAdaptersStr := os.Getenv("HYPERFLEET_CLUSTER_ADAPTERS"); clusterAdaptersStr != "" {
		var adapters []string
		if err := json.Unmarshal([]byte(clusterAdaptersStr), &adapters); err == nil {
			c.RequiredClusterAdapters = adapters
			log.Printf("Loaded HYPERFLEET_CLUSTER_ADAPTERS from env: %v", adapters)
		} else {
			log.Printf("WARNING: Failed to parse HYPERFLEET_CLUSTER_ADAPTERS, using defaults: %v", err)
		}
	}

	if nodepoolAdaptersStr := os.Getenv("HYPERFLEET_NODEPOOL_ADAPTERS"); nodepoolAdaptersStr != "" {
		var adapters []string
		if err := json.Unmarshal([]byte(nodepoolAdaptersStr), &adapters); err == nil {
			c.RequiredNodePoolAdapters = adapters
			log.Printf("Loaded HYPERFLEET_NODEPOOL_ADAPTERS from env: %v", adapters)
		} else {
			log.Printf("WARNING: Failed to parse HYPERFLEET_NODEPOOL_ADAPTERS, using defaults: %v", err)
		}
	}
}

// Default cluster adapters: validation, dns, pullsecret, hypershift
func getDefaultClusterAdapters() []string {
	return []string{
		"validation",
		"dns",
		"pullsecret",
		"hypershift",
	}
}

// Default nodepool adapters: validation, hypershift
func getDefaultNodePoolAdapters() []string {
	return []string{
		"validation",
		"hypershift",
	}
}
