package config

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestJWTConfig_Validate(t *testing.T) {
	RegisterTestingT(t)

	t.Run("disabled JWT requires nothing", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: false}
		Expect(cfg.Validate()).To(Succeed())
	})

	t.Run("enabled JWT with empty configs passes", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: true, Configs: []JWTIssuerConfig{}}
		Expect(cfg.Validate()).To(Succeed())
	})

	t.Run("enabled JWT with valid config passes", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{
			Enabled: true,
			Configs: []JWTIssuerConfig{
				{
					IssuerURL:     "https://sso.example.com/auth/realms/test",
					IdentityClaim: "email",
					JWKCertURL:    "https://sso.example.com/certs",
				},
			},
		}
		Expect(cfg.Validate()).To(Succeed())
	})

	t.Run("enabled JWT config missing issuer_url fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{
			Enabled: true,
			Configs: []JWTIssuerConfig{
				{IdentityClaim: "email", JWKCertURL: "https://sso.example.com/certs"},
			},
		}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("issuer_url"))
	})

	t.Run("enabled JWT config missing identity_claim fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{
			Enabled: true,
			Configs: []JWTIssuerConfig{
				{IssuerURL: "https://sso.example.com/auth/realms/test", JWKCertURL: "https://sso.example.com/certs"},
			},
		}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("identity_claim"))
	})
}

func TestTimeoutsConfig_Validate(t *testing.T) {
	RegisterTestingT(t)

	t.Run("valid timeouts pass", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := TimeoutsConfig{Read: 5_000_000_000, Write: 30_000_000_000}
		Expect(cfg.Validate()).To(Succeed())
	})

	t.Run("read timeout too short fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := TimeoutsConfig{Read: 500_000_000, Write: 30_000_000_000}
		Expect(cfg.Validate()).To(HaveOccurred())
	})
}
