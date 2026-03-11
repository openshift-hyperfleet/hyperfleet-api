package environments

import (
	"os"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_session"
)

var _ EnvironmentImpl = &integrationTestingEnvImpl{}

// integrationTestingEnvImpl is configuration for integration tests using testcontainers
type integrationTestingEnvImpl struct {
	env *Env
}

func (e *integrationTestingEnvImpl) OverrideDatabase(c *Database) error {
	c.SessionFactory = db_session.NewTestcontainerFactory(e.env.Config.Database)
	return nil
}

func (e *integrationTestingEnvImpl) OverrideConfig(c *config.ApplicationConfig) error {
	// Support a one-off env to allow enabling db debug in testing
	if os.Getenv("HYPERFLEET_DATABASE_DEBUG") == "true" { //nolint:goconst - "true" is not extracted to constant (standard env var idiom)
		c.Database.Debug = true
	}

	// Integration tests use testcontainers — set defaults directly
	c.Database.Name = "hyperfleet_test"
	c.Database.Username = "test"
	c.Database.Password = "test"
	c.Database.Host = "localhost"
	c.Database.Port = 5432

	// Ensure SSL mode is set to disable for testing
	if c.Database.SSL.Mode == "" {
		c.Database.SSL.Mode = SSLModeDisable
	}

	// Enable OCM mocks for integration testing (no real OCM connection needed)
	c.OCM.Mock.Enabled = true

	return nil
}

func (e *integrationTestingEnvImpl) OverrideServices(s *Services) error {
	return nil
}

func (e *integrationTestingEnvImpl) OverrideHandlers(h *Handlers) error {
	return nil
}

func (e *integrationTestingEnvImpl) OverrideClients(c *Clients) error {
	return nil
}

func (e *integrationTestingEnvImpl) EnvironmentDefaults() map[string]string {
	// Return empty map - new config system has appropriate defaults
	// and OverrideConfig() sets test-specific values programmatically
	return map[string]string{}
}
