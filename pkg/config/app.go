package config

import (
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type AppConfig struct {
	Name    string `mapstructure:"name" json:"name" validate:"required"`
	Version string `mapstructure:"version" json:"version"`
}

func NewAppConfig() *AppConfig {
	return &AppConfig{
		Name:    "",
		Version: "1.0.0",
	}
}

// defineAndBindFlags defines app flags and binds them to viper keys in a single pass
func (c *AppConfig) defineAndBindFlags(v *viper.Viper, fs *pflag.FlagSet) {
	defineAndBindStringFlag(v, fs, "app.name", "name", "n", c.Name, "Component name (REQUIRED)")
	defineAndBindStringFlag(v, fs, "app.version", "version", "", c.Version, "Component version")
}
