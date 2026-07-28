package environments

import (
	"fmt"
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
	//nolint:goconst // "true" is not extracted to constant (standard env var idiom)
	if os.Getenv("HYPERFLEET_DATABASE_DEBUG") == "true" {
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

	// Integration tests always use JWT. Config load may disable JWT so validation
	// can pass before issuers exist; re-enable and bootstrap a complete issuer here.
	// JWKCertURL is a placeholder; the test harness overwrites it once the JWK
	// mock server is running.
	c.Server.JWT.Enabled = true
	if len(c.Server.JWT.Configs) == 0 {
		c.Server.JWT.Configs = []config.JWTIssuerConfig{
			{
				IssuerURL:     "https://test-issuer.example.com",
				JWKCertURL:    "https://test-issuer.example.com/.well-known/jwks.json",
				Header:        "Authorization",
				IdentityClaim: "email",
			},
		}
	}

	c.Server.JWT.ApplyDefaults()
	if err := c.Server.JWT.Validate(); err != nil {
		return fmt.Errorf("integration test JWT config validation failed: %w", err)
	}

	return nil
}

func (e *integrationTestingEnvImpl) OverrideServices(s *Services) error {
	return nil
}

func (e *integrationTestingEnvImpl) OverrideHandlers(h *Handlers) error {
	return nil
}

func (e *integrationTestingEnvImpl) EnvironmentDefaults() map[string]string {
	// Return empty map - new config system has appropriate defaults
	// and OverrideConfig() sets test-specific values programmatically
	return map[string]string{}
}
