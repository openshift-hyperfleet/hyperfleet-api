package config

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
)

// AdapterRequirementsConfig configures which adapters must be ready for resources
type AdapterRequirementsConfig struct {
	RequiredClusterAdapters  []string
	RequiredNodePoolAdapters []string
}

// NewAdapterRequirementsConfig creates config from environment variables.
// Returns an error if required environment variables are not set.
// Required env vars: HYPERFLEET_CLUSTER_ADAPTERS, HYPERFLEET_NODEPOOL_ADAPTERS
// Format: JSON array, e.g., '["validation","dns","pullsecret","hypershift"]'
func NewAdapterRequirementsConfig() (*AdapterRequirementsConfig, error) {
	config := &AdapterRequirementsConfig{}

	if err := config.LoadFromEnv(); err != nil {
		return nil, err
	}

	return config, nil
}

// LoadFromEnv loads adapter lists from HYPERFLEET_CLUSTER_ADAPTERS and
// HYPERFLEET_NODEPOOL_ADAPTERS (JSON array format: '["adapter1","adapter2"]')
// Returns an error if the environment variables are not set or have invalid JSON.
func (c *AdapterRequirementsConfig) LoadFromEnv() error {
	clusterAdaptersStr := os.Getenv("HYPERFLEET_CLUSTER_ADAPTERS")
	if clusterAdaptersStr == "" {
		return fmt.Errorf("HYPERFLEET_CLUSTER_ADAPTERS environment variable is required but not set")
	}

	var clusterAdapters []string
	if err := json.Unmarshal([]byte(clusterAdaptersStr), &clusterAdapters); err != nil {
		return fmt.Errorf("failed to parse HYPERFLEET_CLUSTER_ADAPTERS: %w "+
			"(expected JSON array, e.g., '[\"validation\",\"dns\"]')", err)
	}
	c.RequiredClusterAdapters = clusterAdapters
	log.Printf("Loaded HYPERFLEET_CLUSTER_ADAPTERS from env: %v", clusterAdapters)

	nodepoolAdaptersStr := os.Getenv("HYPERFLEET_NODEPOOL_ADAPTERS")
	if nodepoolAdaptersStr == "" {
		return fmt.Errorf("HYPERFLEET_NODEPOOL_ADAPTERS environment variable is required but not set")
	}

	var nodepoolAdapters []string
	if err := json.Unmarshal([]byte(nodepoolAdaptersStr), &nodepoolAdapters); err != nil {
		return fmt.Errorf("failed to parse HYPERFLEET_NODEPOOL_ADAPTERS: %w "+
			"(expected JSON array, e.g., '[\"validation\",\"hypershift\"]')", err)
	}
	c.RequiredNodePoolAdapters = nodepoolAdapters
	log.Printf("Loaded HYPERFLEET_NODEPOOL_ADAPTERS from env: %v", nodepoolAdapters)

	return nil
}
