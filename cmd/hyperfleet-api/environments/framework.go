package environments

import (
	"context"
	"os"
	"strings"

	"github.com/spf13/pflag"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/client/ocm"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

func init() {
	once.Do(func() {
		environment = &Env{}

		// Create the configuration
		environment.Config = config.NewApplicationConfig()
		environment.Name = GetEnvironmentStrFromEnv()

		environments = map[string]EnvironmentImpl{
			DevelopmentEnv:        &devEnvImpl{environment},
			UnitTestingEnv:        &unitTestingEnvImpl{environment},
			IntegrationTestingEnv: &integrationTestingEnvImpl{environment},
			ProductionEnv:         &productionEnvImpl{environment},
		}
	})
}

// EnvironmentImpl defines a set of behaviors for an OCM environment.
// Each environment provides a set of flags for basic set/override of the environment
// and configuration functions for each component type.
type EnvironmentImpl interface {
	Flags() map[string]string
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

// AddFlags Adds environment flags, using the environment's config struct, to the flagset 'flags'
func (e *Env) AddFlags(flags *pflag.FlagSet) error {
	e.Config.AddFlags(flags)
	return setConfigDefaults(flags, environments[e.Name].Flags())
}

// Initialize loads the environment's resources
// This should be called after the e.Config has been set appropriately though AddFlags and pasing, done elsewhere
// The environment does NOT handle flag parsing
func (e *Env) Initialize() error {
	ctx := context.Background()
	logger.Infof(ctx, "Initializing %s environment", e.Name)

	envImpl, found := environments[e.Name]
	if !found {
		logger.Errorf(ctx, "Unknown runtime environment: %s", e.Name)
		os.Exit(1)
	}

	if err := envImpl.OverrideConfig(e.Config); err != nil {
		logger.Errorf(ctx, "Failed to configure ApplicationConfig: %s", err)
		os.Exit(1)
	}

	messages := environment.Config.ReadFiles()
	if len(messages) != 0 {
		logger.Errorf(ctx, "unable to read configuration files:\n%s", strings.Join(messages, "\n"))
		os.Exit(1)
	}

	// each env will set db explicitly because the DB impl has a `once` init section
	if err := envImpl.OverrideDatabase(&e.Database); err != nil {
		logger.Errorf(ctx, "Failed to configure Database: %s", err)
		os.Exit(1)
	}

	err := e.LoadClients()
	if err != nil {
		return err
	}
	if err := envImpl.OverrideClients(&e.Clients); err != nil {
		logger.Errorf(ctx, "Failed to configure Clients: %s", err)
		os.Exit(1)
	}

	e.LoadServices()
	if err := envImpl.OverrideServices(&e.Services); err != nil {
		logger.Errorf(ctx, "Failed to configure Services: %s", err)
		os.Exit(1)
	}

	seedErr := e.Seed()
	if seedErr != nil {
		return seedErr
	}

	if err := envImpl.OverrideHandlers(&e.Handlers); err != nil {
		logger.Errorf(ctx, "Failed to configure Handlers: %s", err)
		os.Exit(1)
	}

	return nil
}

func (e *Env) Seed() *errors.ServiceError {
	return nil
}

func (e *Env) LoadServices() {
	// Initialize the service registry map
	e.Services.serviceRegistry = make(map[string]interface{})

	// Auto-discovered services (no manual editing needed)
	registry.LoadDiscoveredServices(&e.Services, e)
}

func (e *Env) LoadClients() error {
	ctx := context.Background()
	var err error

	ocmConfig := ocm.Config{
		BaseURL:      e.Config.OCM.BaseURL,
		ClientID:     e.Config.OCM.ClientID,
		ClientSecret: e.Config.OCM.ClientSecret,
		SelfToken:    e.Config.OCM.SelfToken,
		TokenURL:     e.Config.OCM.TokenURL,
		Debug:        e.Config.OCM.Debug,
	}

	// Create OCM Authz client
	if e.Config.OCM.EnableMock {
		logger.Info(ctx, "Using Mock OCM Authz Client")
		e.Clients.OCM, err = ocm.NewClientMock(ocmConfig)
	} else {
		e.Clients.OCM, err = ocm.NewClient(ocmConfig)
	}
	if err != nil {
		logger.Errorf(ctx, "Unable to create OCM Authz client: %s", err)
		return err
	}

	return nil
}

func (e *Env) Teardown() {
	ctx := context.Background()
	if e.Database.SessionFactory != nil {
		if err := e.Database.SessionFactory.Close(); err != nil {
			logger.Errorf(ctx, "Error closing database session factory: %s", err)
		}
	}
	if e.Clients.OCM != nil {
		if err := e.Clients.OCM.Close(); err != nil {
			logger.Errorf(ctx, "Error closing OCM client: %v", err)
		}
	}
}

func setConfigDefaults(flags *pflag.FlagSet, defaults map[string]string) error {
	ctx := context.Background()
	for name, value := range defaults {
		if err := flags.Set(name, value); err != nil {
			logger.Error(ctx, "Error setting flag", "flag", name, "error", err)
			return err
		}
	}
	return nil
}
