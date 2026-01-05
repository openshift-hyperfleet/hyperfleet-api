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
	if os.Getenv("DB_DEBUG") == "true" {
		c.Database.Debug = true
	}
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
