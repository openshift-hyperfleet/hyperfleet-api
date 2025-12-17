package environments

import (
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_session"
)

// embeddedDevelopmentEnvImpl runs the API with an embedded PostgreSQL instance (testcontainers).
// Intended for local/dev usage by dependent projects that want a fully autonomous API.
type embeddedDevelopmentEnvImpl struct {
	env *Env
}

var _ EnvironmentImpl = &embeddedDevelopmentEnvImpl{}

func (e *embeddedDevelopmentEnvImpl) OverrideDatabase(c *Database) error {
	c.SessionFactory = db_session.NewTestcontainerFactory(e.env.Config.Database)
	return nil
}

func (e *embeddedDevelopmentEnvImpl) OverrideConfig(c *config.ApplicationConfig) error {
	c.Server.EnableJWT = false
	c.Server.EnableAuthz = false
	c.Server.EnableHTTPS = false
	return nil
}

func (e *embeddedDevelopmentEnvImpl) OverrideServices(_ *Services) error {
	return nil
}

func (e *embeddedDevelopmentEnvImpl) OverrideHandlers(_ *Handlers) error {
	return nil
}

func (e *embeddedDevelopmentEnvImpl) OverrideClients(_ *Clients) error {
	return nil
}

func (e *embeddedDevelopmentEnvImpl) Flags() map[string]string {
	return map[string]string{
		"v":                      "10",
		"logtostderr":            "true",
		"enable-authz":           "false",
		"enable-jwt":             "false",
		"ocm-debug":              "false",
		"enable-ocm-mock":        "true",
		"enable-https":           "false",
		"enable-metrics-https":   "false",
		"api-server-hostname":    "localhost",
		"api-server-bindaddress": "localhost:8000",
	}
}
