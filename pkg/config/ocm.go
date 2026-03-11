package config

import (
	"encoding/json"
)

// OCMConfig holds OpenShift Cluster Manager configuration
// Follows HyperFleet Configuration Standard
type OCMConfig struct {
	BaseURL      string     `mapstructure:"base_url" json:"base_url" validate:"required,url"`
	ClientID     string     `mapstructure:"client_id" json:"-"` // Excluded from JSON marshaling (sensitive)
	ClientSecret string     `mapstructure:"client_secret" json:"-"` // Excluded from JSON marshaling (sensitive)
	SelfToken    string     `mapstructure:"self_token" json:"-"` // Excluded from JSON marshaling (sensitive)
	TokenURL     string     `mapstructure:"token_url" json:"token_url" validate:"required,url"`
	Debug        bool       `mapstructure:"debug" json:"debug"`
	Mock         MockConfig `mapstructure:"mock" json:"mock" validate:"required"`
}

// MockConfig holds mock configuration for testing
type MockConfig struct {
	Enabled bool `mapstructure:"enabled" json:"enabled"`
}

// MarshalJSON implements custom JSON marshaling to redact sensitive fields
func (c OCMConfig) MarshalJSON() ([]byte, error) {
	type Alias OCMConfig
	return json.Marshal(&struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		SelfToken    string `json:"self_token"`
		*Alias
	}{
		ClientID:     redactIfSet(c.ClientID),
		ClientSecret: redactIfSet(c.ClientSecret),
		SelfToken:    redactIfSet(c.SelfToken),
		Alias:        (*Alias)(&c),
	})
}

// NewOCMConfig returns default OCMConfig values
// These defaults can be overridden by config file, env vars, or CLI flags
func NewOCMConfig() *OCMConfig {
	return &OCMConfig{
		BaseURL:      "https://api.integration.openshift.com",
		ClientID:     "",
		ClientSecret: "",
		SelfToken:    "",
		TokenURL:     "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token",
		Debug:        false,
		Mock: MockConfig{
			Enabled: false, // Default to real OCM clients; tests can opt in to mocks
		},
	}
}

// ============================================================
// Convenience Accessor Methods
// ============================================================

// EnableMock returns mock enabled flag
func (c *OCMConfig) EnableMock() bool {
	return c.Mock.Enabled
}
