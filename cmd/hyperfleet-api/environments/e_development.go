package environments

import (
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_session"
)

// devEnvImpl environment is intended for local use while developing features
type devEnvImpl struct {
	env *Env
}

var _ EnvironmentImpl = &devEnvImpl{}

func (e *devEnvImpl) OverrideDatabase(c *Database) error {
	c.SessionFactory = db_session.NewProdFactory(e.env.Config.Database)
	return nil
}

func (e *devEnvImpl) OverrideConfig(c *config.ApplicationConfig) error {
	c.Server.JWT.Enabled = false
	c.Server.TLS.Enabled = false

	// Ensure SSL mode is set to disable for development (required for database connection)
	if c.Database.SSL.Mode == "" {
		c.Database.SSL.Mode = SSLModeDisable
	}

	// Enable OCM mocks for development (no real OCM connection needed)
	c.OCM.Mock.Enabled = true

	return nil
}

func (e *devEnvImpl) OverrideServices(s *Services) error {
	return nil
}

func (e *devEnvImpl) OverrideHandlers(h *Handlers) error {
	return nil
}

func (e *devEnvImpl) OverrideClients(c *Clients) error {
	return nil
}

func (e *devEnvImpl) EnvironmentDefaults() map[string]string {
	// Return empty map - new config system has appropriate defaults
	// and OverrideConfig() sets development-specific values programmatically
	return map[string]string{}
}
