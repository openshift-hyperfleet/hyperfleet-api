package environments

import (
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/pflag"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
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
	// Initialize structured logging first, before any other operations
	logger.Initialize(logger.Config{
		Level:   e.Config.Logging.Level,
		Format:  e.Config.Logging.Format,
		Output:  e.Config.Logging.Output,
		Version: api.Version,
	})

	slog.Info("Initializing environment", "environment", e.Name)

	envImpl, found := environments[e.Name]
	if !found {
		slog.Error("Unknown runtime environment", "environment", e.Name)
		os.Exit(1)
	}

	if err := envImpl.OverrideConfig(e.Config); err != nil {
		slog.Error("Failed to configure ApplicationConfig", "error", err)
		os.Exit(1)
	}

	messages := environment.Config.ReadFiles()
	if len(messages) != 0 {
		slog.Error("Unable to read configuration files", "errors", strings.Join(messages, "\n"))
		os.Exit(1)
	}

	// each env will set db explicitly because the DB impl has a `once` init section
	if err := envImpl.OverrideDatabase(&e.Database); err != nil {
		slog.Error("Failed to configure Database", "error", err)
		os.Exit(1)
	}

	err := e.LoadClients()
	if err != nil {
		return err
	}
	if err := envImpl.OverrideClients(&e.Clients); err != nil {
		slog.Error("Failed to configure Clients", "error", err)
		os.Exit(1)
	}

	e.LoadServices()
	if err := envImpl.OverrideServices(&e.Services); err != nil {
		slog.Error("Failed to configure Services", "error", err)
		os.Exit(1)
	}

	seedErr := e.Seed()
	if seedErr != nil {
		return seedErr
	}

	if err := envImpl.OverrideHandlers(&e.Handlers); err != nil {
		slog.Error("Failed to configure Handlers", "error", err)
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
		slog.Info("Using Mock OCM Authz Client")
		e.Clients.OCM, err = ocm.NewClientMock(ocmConfig)
	} else {
		e.Clients.OCM, err = ocm.NewClient(ocmConfig)
	}
	if err != nil {
		slog.Error("Unable to create OCM Authz client", "error", err)
		return err
	}

	return nil
}

func (e *Env) Teardown() {
	if e.Database.SessionFactory != nil {
		if err := e.Database.SessionFactory.Close(); err != nil {
			slog.Error("Error closing database session factory", "error", err)
		}
	}
	if e.Clients.OCM != nil {
		if err := e.Clients.OCM.Close(); err != nil {
			slog.Error("Error closing OCM client", "error", err)
		}
	}
}

func setConfigDefaults(flags *pflag.FlagSet, defaults map[string]string) error {
	for name, value := range defaults {
		if err := flags.Set(name, value); err != nil {
			slog.Error("Error setting flag", "flag", name, "error", err)
			return err
		}
	}
	return nil
}
