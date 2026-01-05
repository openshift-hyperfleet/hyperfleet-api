package config

import (
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type OCMConfig struct {
	BaseURL      string `mapstructure:"base_url" json:"base_url" validate:""`
	ClientID     string `mapstructure:"client_id" json:"client_id" validate:""`
	ClientSecret string `mapstructure:"client_secret" json:"client_secret" validate:""`
	SelfToken    string `mapstructure:"self_token" json:"self_token" validate:""`
	TokenURL     string `mapstructure:"token_url" json:"token_url" validate:""`
	Debug        bool   `mapstructure:"debug" json:"debug"`
	EnableMock   bool   `mapstructure:"enable_mock" json:"enable_mock"`
}

func NewOCMConfig() *OCMConfig {
	return &OCMConfig{
		BaseURL:    "https://api.integration.openshift.com",
		TokenURL:   "https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token",
		Debug:      false,
		EnableMock: true,
	}
}

// defineAndBindFlags defines & binds flags to viper keys in a single pass
func (c *OCMConfig) defineAndBindFlags(v *viper.Viper, fs *pflag.FlagSet) {
	// OCM connection parameters
	defineAndBindStringFlag(v, fs, "ocm.base_url", "ocm-base-url", "", c.BaseURL, "OCM API base URL")
	defineAndBindStringFlag(v, fs, "ocm.token_url", "ocm-token-url", "", c.TokenURL, "OCM token URL")
	defineAndBindStringFlag(v, fs, "ocm.client_id", "ocm-client-id", "", c.ClientID, "OCM client ID")
	defineAndBindStringFlag(v, fs, "ocm.client_secret", "ocm-client-secret", "", c.ClientSecret, "OCM client secret (prefer using env var)")
	defineAndBindStringFlag(v, fs, "ocm.self_token", "ocm-self-token", "", c.SelfToken, "OCM self token (prefer using env var)")

	// Options
	defineAndBindBoolFlag(v, fs, "ocm.debug", "ocm-debug", "", c.Debug, "Enable OCM debug mode")
	defineAndBindBoolFlag(v, fs, "ocm.enable_mock", "ocm-mock", "", c.EnableMock, "Enable mock OCM client")
}
