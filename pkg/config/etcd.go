package config

import (
	"os"
	"path/filepath"
)

// EtcdConfig configures the etcd adapter.
type EtcdConfig struct {
	// FleetDir is the directory of <mc>.kubeconfig files the adapter places members on.
	FleetDir string `mapstructure:"fleet_dir" json:"fleet_dir"`
	// PreferredMC is the management cluster that hosts member 0.
	PreferredMC  string `mapstructure:"preferred_mc" json:"preferred_mc"`
	Image        string `mapstructure:"image" json:"image"`
	StorageClass string `mapstructure:"storage_class" json:"storage_class"`
	StorageSize  string `mapstructure:"storage_size" json:"storage_size"`
	// StateDir persists membership and cert material between reconciles.
	StateDir string `mapstructure:"state_dir" json:"state_dir"`
}

// NewEtcdConfig returns default EtcdConfig values.
func NewEtcdConfig() *EtcdConfig {
	stateDir := ""
	if home, err := os.UserHomeDir(); err == nil {
		stateDir = filepath.Join(home, ".local", "state", "hyperfleet", "etcd")
	}
	return &EtcdConfig{
		Image:       "quay.io/openshift/origin-etcd:4.20.0",
		StorageSize: "8Gi",
		StateDir:    stateDir,
	}
}
