package environments

import (
	"os"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	dbmocks "github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/mocks"
)

var _ EnvironmentImpl = &unitTestingEnvImpl{}

// unitTestingEnvImpl is configuration for unit tests using mocked database
type unitTestingEnvImpl struct {
	env *Env
}

func (e *unitTestingEnvImpl) OverrideDatabase(c *Database) error {
	c.SessionFactory = dbmocks.NewMockSessionFactory()
	return nil
}

func (e *unitTestingEnvImpl) OverrideConfig(c *config.ApplicationConfig) error {
	// Support a one-off env to allow enabling db debug in testing
	//nolint:goconst // "true" is not extracted to constant (standard env var idiom)
	if os.Getenv("HYPERFLEET_DATABASE_DEBUG") == "true" {
		c.Database.Debug = true
	}

	// Ensure SSL mode is set to disable for testing
	if c.Database.SSL.Mode == "" {
		c.Database.SSL.Mode = SSLModeDisable
	}
	// Unit tests use a mock DB and don't need real credentials
	return nil
}

func (e *unitTestingEnvImpl) OverrideServices(s *Services) error {
	return nil
}

func (e *unitTestingEnvImpl) OverrideHandlers(h *Handlers) error {
	return nil
}

func (e *unitTestingEnvImpl) OverrideClients(c *Clients) error {
	return nil
}

func (e *unitTestingEnvImpl) EnvironmentDefaults() map[string]string {
	// Return empty map - new config system has appropriate defaults
	// and OverrideConfig() sets test-specific values programmatically
	return map[string]string{}
}
