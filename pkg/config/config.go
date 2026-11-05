package config

import (
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/chains"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
)

// ApplicationConfig holds all application configuration
// Follows HyperFleet Configuration Standard with validation and structured marshaling
type ApplicationConfig struct {
	Server       *ServerConfig               `mapstructure:"server" json:"server" validate:"required"`
	Metrics      *MetricsConfig              `mapstructure:"metrics" json:"metrics" validate:"required"`
	Health       *HealthConfig               `mapstructure:"health" json:"health" validate:"required"`
	Database     *DatabaseConfig             `mapstructure:"database" json:"database" validate:"required"`
	Logging      *LoggingConfig              `mapstructure:"logging" json:"logging" validate:"required"`
	Tracing      *TracingConfig              `mapstructure:"tracing" json:"tracing" validate:"required"`
	Orchestrator *OrchestratorConfig         `mapstructure:"orchestrator" json:"orchestrator" validate:"required"`
	Entities     []registry.EntityDescriptor `mapstructure:"entities" json:"entities"`
	Chains       []chains.Chain              `mapstructure:"chains" json:"chains"`
}

// knownAdapters returns every adapter name declared by an entity descriptor.
func knownAdapters(entities []registry.EntityDescriptor) map[string]bool {
	names := make(map[string]bool)
	for _, entity := range entities {
		for _, adapter := range entity.RequiredAdapters {
			names[adapter] = true
		}
	}
	return names
}

// NewApplicationConfig returns default ApplicationConfig with all sub-configs initialized
// These defaults can be overridden by config file, env vars, or CLI flags
func NewApplicationConfig() *ApplicationConfig {
	return &ApplicationConfig{
		Server:       NewServerConfig(),
		Metrics:      NewMetricsConfig(),
		Health:       NewHealthConfig(),
		Database:     NewDatabaseConfig(),
		Logging:      NewLoggingConfig(),
		Tracing:      NewTracingConfig(),
		Orchestrator: NewOrchestratorConfig(),
	}
}
