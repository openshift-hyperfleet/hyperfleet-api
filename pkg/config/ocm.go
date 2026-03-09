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
		ClientID:     redactOCMIfSet(c.ClientID),
		ClientSecret: redactOCMIfSet(c.ClientSecret),
		SelfToken:    redactOCMIfSet(c.SelfToken),
		Alias:        (*Alias)(&c),
	})
}

// redactOCMIfSet returns RedactedValue if value is non-empty, otherwise empty string
func redactOCMIfSet(value string) string {
	if value == "" {
		return ""
	}
	return RedactedValue
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
// BACKWARD COMPATIBILITY HELPERS
// ============================================================

// GetBaseURL returns base URL (legacy accessor)
func (c *OCMConfig) GetBaseURL() string {
	return c.BaseURL
}

// GetClientID returns client ID (legacy accessor)
func (c *OCMConfig) GetClientID() string {
	return c.ClientID
}

// GetClientSecret returns client secret (legacy accessor)
func (c *OCMConfig) GetClientSecret() string {
	return c.ClientSecret
}

// GetSelfToken returns self token (legacy accessor)
func (c *OCMConfig) GetSelfToken() string {
	return c.SelfToken
}

// GetTokenURL returns token URL (legacy accessor)
func (c *OCMConfig) GetTokenURL() string {
	return c.TokenURL
}

// EnableMock returns mock enabled flag (legacy accessor)
func (c *OCMConfig) EnableMock() bool {
	return c.Mock.Enabled
}
