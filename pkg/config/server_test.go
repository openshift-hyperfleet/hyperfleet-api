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

func TestIdentityHeaderConfig_Validate(t *testing.T) {
	RegisterTestingT(t)

	t.Run("disabled identity header requires nothing", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := IdentityHeaderConfig{Enabled: false}
		Expect(cfg.Validate()).To(Succeed())
	})

	t.Run("enabled identity header without name fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := IdentityHeaderConfig{Enabled: true, Name: ""}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("identity_header.name"))
	})

	t.Run("forbidden header name fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := IdentityHeaderConfig{Enabled: true, Name: "Authorization"}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not allowed"))
	})

	t.Run("enabled identity header with valid name passes", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := IdentityHeaderConfig{Enabled: true, Name: "X-HyperFleet-Identity"}
		Expect(cfg.Validate()).To(Succeed())
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
