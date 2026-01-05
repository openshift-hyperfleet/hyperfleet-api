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
