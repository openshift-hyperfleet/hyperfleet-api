package environments

import (
	"context"
	"fmt"
	"os"
	"strconv"
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

// ApplyEnvironmentOverrides applies environment-specific configuration overrides
// This is used by the new config system to apply environment-specific settings
// (e.g., development environment disables JWT and TLS)
func ApplyEnvironmentOverrides(cfg *config.ApplicationConfig) error {
	// Read current environment from env var instead of using cached environment.Name
	// to ensure we use the most up-to-date value
	envName := GetEnvironmentStrFromEnv()
	envImpl, found := environments[envName]
	if !found {
		return fmt.Errorf("unknown runtime environment: %s", envName)
	}
	return envImpl.OverrideConfig(cfg)
}

// AddFlags Adds environment flags, using the environment's config struct, to the flagset 'flags'
func (e *Env) AddFlags(flags *pflag.FlagSet) error {
	e.Config.AddFlags(flags) //nolint:staticcheck // Intentional for backward compatibility with old config system
	return setConfigDefaults(flags, environments[e.Name].Flags())
}

// Initialize loads the environment's resources
// This should be called after the e.Config has been set appropriately though AddFlags and pasing, done elsewhere
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

	// For backward compatibility with old configuration system:
	// Read database configuration from environment variables if not using new config system
	if !config.IsNewConfigEnabled() {
		loadDatabaseConfigFromEnv(e.Config.Database)
	}

	if err := envImpl.OverrideConfig(e.Config); err != nil {
		logger.WithError(ctx, err).Error("Failed to configure ApplicationConfig")
		os.Exit(1)
	}

	messages := environment.Config.ReadFiles() //nolint:staticcheck // Intentional for backward compatibility
	if len(messages) != 0 {
		err := fmt.Errorf("%s", strings.Join(messages, "\n"))
		logger.WithError(ctx, err).Error("Unable to read configuration files")
		os.Exit(1)
	}

	// Load adapter configuration from environment variables
	if err := e.Config.LoadAdapters(); err != nil { //nolint:staticcheck // Intentional for backward compatibility
		logger.WithError(ctx, err).Error("Failed to load adapter configuration")
		os.Exit(1)
	}

	// each env will set db explicitly because the DB impl has a `once` init section
	if err := envImpl.OverrideDatabase(&e.Database); err != nil {
		logger.WithError(ctx, err).Error("Failed to configure Database")
		os.Exit(1)
	}

	err := e.LoadClients()
	if err != nil {
		return err
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
	if e.Config.OCM.EnableMock() {
		logger.Info(ctx, "Using Mock OCM Authz Client")
		e.Clients.OCM, err = ocm.NewClientMock(ocmConfig)
	} else {
		e.Clients.OCM, err = ocm.NewClient(ocmConfig)
	}
	if err != nil {
		logger.WithError(ctx, err).Error("Unable to create OCM Authz client")
		return err
	}

	return nil
}

func (e *Env) Teardown() {
	ctx := context.Background()
	if e.Database.SessionFactory != nil {
		if err := e.Database.SessionFactory.Close(); err != nil {
			logger.WithError(ctx, err).Error("Error closing database session factory")
		}
	}
	if e.Clients.OCM != nil {
		if err := e.Clients.OCM.Close(); err != nil {
			logger.WithError(ctx, err).Error("Error closing OCM client")
		}
	}
}

func setConfigDefaults(flags *pflag.FlagSet, defaults map[string]string) error {
	ctx := context.Background()
	for name, value := range defaults {
		if err := flags.Set(name, value); err != nil {
			logger.With(ctx, logger.FieldFlag, name).WithError(err).Error("Error setting flag")
			return err
		}
	}
	return nil
}

// loadDatabaseConfigFromEnv loads database configuration from environment variables
// This provides backward compatibility with the old configuration system
func loadDatabaseConfigFromEnv(dbConfig *config.DatabaseConfig) {
	// Support both old (DB_*) and new (HYPERFLEET_DATABASE_*) environment variable names
	if host := os.Getenv("DB_HOST"); host != "" {
		dbConfig.Host = host
	}
	if host := os.Getenv("HYPERFLEET_DATABASE_HOST"); host != "" {
		dbConfig.Host = host
	}

	if port := os.Getenv("DB_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			dbConfig.Port = p
		}
	}
	if port := os.Getenv("HYPERFLEET_DATABASE_PORT"); port != "" {
		if p, err := strconv.Atoi(port); err == nil {
			dbConfig.Port = p
		}
	}

	if name := os.Getenv("DB_NAME"); name != "" {
		dbConfig.Name = name
	}
	if name := os.Getenv("HYPERFLEET_DATABASE_NAME"); name != "" {
		dbConfig.Name = name
	}

	if username := os.Getenv("DB_USERNAME"); username != "" {
		dbConfig.Username = username
	}
	if username := os.Getenv("HYPERFLEET_DATABASE_USERNAME"); username != "" {
		dbConfig.Username = username
	}

	if password := os.Getenv("DB_PASSWORD"); password != "" {
		dbConfig.Password = password
	}
	if password := os.Getenv("HYPERFLEET_DATABASE_PASSWORD"); password != "" {
		dbConfig.Password = password
	}

	if sslMode := os.Getenv("DB_SSL_MODE"); sslMode != "" {
		dbConfig.SSL.Mode = sslMode
	}
	if sslMode := os.Getenv("HYPERFLEET_DATABASE_SSL_MODE"); sslMode != "" {
		dbConfig.SSL.Mode = sslMode
	}

	if debug := os.Getenv("DB_DEBUG"); debug == "true" { //nolint:goconst // Environment variable check
		dbConfig.Debug = true
	}
	if debug := os.Getenv("HYPERFLEET_DATABASE_DEBUG"); debug == "true" { //nolint:goconst // Environment variable check
		dbConfig.Debug = true
	}
}
