package environments

import (
	"os"

	"github.com/golang/glog"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/client/ocm"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

func init() {
	once.Do(func() {
		environment = &Env{}

		// DO NOT create Config here
		// Config will be provided by commands via Initialize()
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
// Each environment provides configuration functions for each component type.
type EnvironmentImpl interface {
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

// Initialize loads the environment's resources with pre-loaded configuration
// Configuration must be loaded by the caller using config.LoadConfig()
func (e *Env) Initialize(appConfig *config.ApplicationConfig) error {
	glog.Infof("Initializing %s environment", e.Name)

	envImpl, found := environments[e.Name]
	if !found {
		glog.Fatalf("Unknown runtime environment: %s", e.Name)
	}

	// Store the provided config
	e.Config = appConfig

	// Allow environment to apply runtime overrides (e.g., DB_DEBUG for tests)
	if err := envImpl.OverrideConfig(e.Config); err != nil {
		glog.Fatalf("Failed to configure ApplicationConfig: %s", err)
	}

	// Initialize database with config
	if err := envImpl.OverrideDatabase(&e.Database); err != nil {
		glog.Fatalf("Failed to configure Database: %s", err)
	}

	// Initialize clients
	err := e.LoadClients()
	if err != nil {
		return err
	}
	if err := envImpl.OverrideClients(&e.Clients); err != nil {
		glog.Fatalf("Failed to configure Clients: %s", err)
	}

	// Initialize services
	e.LoadServices()
	if err := envImpl.OverrideServices(&e.Services); err != nil {
		glog.Fatalf("Failed to configure Services: %s", err)
	}

	// Seed data
	seedErr := e.Seed()
	if seedErr != nil {
		return seedErr
	}

	// Initialize handlers
	if err := envImpl.OverrideHandlers(&e.Handlers); err != nil {
		glog.Fatalf("Failed to configure Handlers: %s", err)
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
		glog.Infof("Using Mock OCM Authz Client")
		e.Clients.OCM, err = ocm.NewClientMock(ocmConfig)
	} else {
		e.Clients.OCM, err = ocm.NewClient(ocmConfig)
	}
	if err != nil {
		glog.Errorf("Unable to create OCM Authz client: %s", err.Error())
		return err
	}

	return nil
}

func (e *Env) Teardown() {
	if e.Database.SessionFactory != nil {
		if err := e.Database.SessionFactory.Close(); err != nil {
			glog.Errorf("Error closing database session factory: %s", err.Error())
		}
	}
	if e.Clients.OCM != nil {
		if err := e.Clients.OCM.Close(); err != nil {
			glog.Errorf("Error closing OCM client: %v", err)
		}
	}
}
