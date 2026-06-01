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

	t.Run("enabled JWT without issuer URL fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: true, IssuerURL: ""}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("issuer_url"))
	})

	t.Run("enabled JWT with issuer URL passes", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{
			Enabled:       true,
			IssuerURL:     "https://sso.example.com/auth/realms/test",
			IdentityClaim: "email",
		}
		Expect(cfg.Validate()).To(Succeed())
	})

	t.Run("enabled JWT without identity claim fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: true, IssuerURL: "https://sso.example.com/auth/realms/test", IdentityClaim: ""}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("identity_claim"))
	})
}

func TestServerConfig_ValidateIdentityHeader(t *testing.T) {
	RegisterTestingT(t)

	t.Run("empty identity header requires nothing", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &ServerConfig{}
		Expect(cfg.ValidateIdentityHeader()).To(Succeed())
	})

	t.Run("forbidden header name fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &ServerConfig{IdentityHeader: "Authorization"}
		err := cfg.ValidateIdentityHeader()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not allowed"))
	})

	t.Run("valid header name passes", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &ServerConfig{IdentityHeader: "X-HyperFleet-Identity"}
		Expect(cfg.ValidateIdentityHeader()).To(Succeed())
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
