package environments

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/pflag"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

func init() {
	once.Do(func() {
		environment = &Env{}

		// Config must be set by caller using ConfigLoader before Initialize()
		environment.Name = GetEnvironmentStrFromEnv()

		environments = map[string]EnvironmentImpl{
			DevelopmentEnv:        &devEnvImpl{environment},
			UnitTestingEnv:        &unitTestingEnvImpl{environment},
			IntegrationTestingEnv: &integrationTestingEnvImpl{environment},
			ProductionEnv:         &productionEnvImpl{environment},
		}
	})
}

// EnvironmentImpl defines a set of behaviors for a runtime environment.
// Each environment provides a set of flags for basic set/override of the environment
// and configuration functions for each component type.
type EnvironmentImpl interface {
	EnvironmentDefaults() map[string]string
	OverrideConfig(c *config.ApplicationConfig) error
	OverrideServices(s *Services) error
	OverrideDatabase(s *Database) error
	OverrideHandlers(c *Handlers) error
	OverrideClients(c *Clients) error
}

func GetEnvironmentStrFromEnv() string {
	envStr, specified := os.LookupEnv(EnvironmentStringKey)
	if !specified || envStr == "" {
		envStr = EnvironmentDefault
	}
	return envStr
}

func Environment() *Env {
	return environment
}

// ApplyEnvironmentOverrides applies environment-specific configuration overrides
// This is used by the new config system to apply environment-specific settings
// (e.g., development environment disables JWT and TLS)
func ApplyEnvironmentOverrides(cfg *config.ApplicationConfig) error {
	envName := GetEnvironmentStrFromEnv()
	envImpl, found := environments[envName]
	if !found {
		return fmt.Errorf("unknown runtime environment: %s", envName)
	}
	return envImpl.OverrideConfig(cfg)
}

// SetEnvironmentDefaults sets environment-specific flag defaults
func (e *Env) SetEnvironmentDefaults(flags *pflag.FlagSet) error {
	return setFlagDefaults(flags, environments[e.Name].EnvironmentDefaults())
}

// Initialize loads the environment's resources
// This should be called after the e.Config has been set appropriately though AddFlags and parsing, done elsewhere
// The environment does NOT handle flag parsing
func (e *Env) Initialize() error {
	ctx := context.Background()

	// Re-read environment name from env var to support tests that set OCM_ENV after init()
	envName := GetEnvironmentStrFromEnv()
	e.Name = envName

	logger.With(ctx, logger.FieldEnvironment, e.Name).Info("Initializing environment")

	envImpl, found := environments[e.Name]
	if !found {
		logger.With(ctx, logger.FieldEnvironment, e.Name).Error("Unknown runtime environment")
		os.Exit(1)
	}

	if err := envImpl.OverrideConfig(e.Config); err != nil {
		logger.WithError(ctx, err).Error("Failed to configure ApplicationConfig")
		os.Exit(1)
	}

	// each env will set db explicitly because the DB impl has a `once` init section
	if err := envImpl.OverrideDatabase(&e.Database); err != nil {
		logger.WithError(ctx, err).Error("Failed to configure Database")
		os.Exit(1)
	}

	if err := envImpl.OverrideClients(&e.Clients); err != nil {
		logger.WithError(ctx, err).Error("Failed to configure Clients")
		os.Exit(1)
	}

	e.LoadServices()
	if err := envImpl.OverrideServices(&e.Services); err != nil {
		logger.WithError(ctx, err).Error("Failed to configure Services")
		os.Exit(1)
	}

	seedErr := e.Seed()
	if seedErr != nil {
		return seedErr
	}

	if err := envImpl.OverrideHandlers(&e.Handlers); err != nil {
		logger.WithError(ctx, err).Error("Failed to configure Handlers")
		os.Exit(1)
	}

	return nil
}

func (e *Env) Seed() *errors.ServiceError {
	return nil
}

func (e *Env) LoadServices() {
	e.Services.serviceRegistry = make(map[string]interface{})
	registry.LoadDiscoveredServices(&e.Services, e)
}

func (e *Env) Teardown() {
	ctx := context.Background()
	if e.Database.SessionFactory != nil {
		if err := e.Database.SessionFactory.Close(); err != nil {
			logger.WithError(ctx, err).Error("Error closing database session factory")
		}
	}
}

func setFlagDefaults(flags *pflag.FlagSet, defaults map[string]string) error {
	ctx := context.Background()
	for name, value := range defaults {
		if err := flags.Set(name, value); err != nil {
			logger.With(ctx, logger.FieldFlag, name).WithError(err).Error("Error setting flag")
			return err
		}
	}
	return nil
}
