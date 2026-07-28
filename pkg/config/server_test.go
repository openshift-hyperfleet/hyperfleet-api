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

	t.Run("enabled JWT with no configs fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: true}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("at least one issuer"))
	})

	t.Run("enabled JWT without issuer URL fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: true, Configs: []JWTIssuerConfig{{JWKCertURL: "https://keys.example.com"}}}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("issuer_url"))
	})

	t.Run("enabled JWT without JWK source fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: true, Configs: []JWTIssuerConfig{{IssuerURL: "https://issuer.example.com"}}}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("jwk_cert_url or jwk_cert_file"))
	})

	t.Run("valid single issuer config with cert URL passes", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: true, Configs: []JWTIssuerConfig{{
			IssuerURL:  "https://issuer.example.com",
			JWKCertURL: "https://keys.example.com",
		}}}
		Expect(cfg.Validate()).To(Succeed())
	})

	caFileTests := []struct {
		name      string
		config    JWTIssuerConfig
		expectErr string
	}{
		{
			name: "jwk_cert_ca_file without jwk_cert_url fails",
			config: JWTIssuerConfig{
				IssuerURL:     "https://issuer.example.com",
				JWKCertFile:   "/etc/hyperfleet/jwks.json",
				JWKCertCAFile: "/var/run/secrets/ca.crt",
			},
			expectErr: "jwk_cert_ca_file requires jwk_cert_url",
		},
		{
			name: "jwk_cert_ca_file with jwk_cert_url passes",
			config: JWTIssuerConfig{
				IssuerURL:     "https://issuer.example.com",
				JWKCertURL:    "https://keys.example.com",
				JWKCertCAFile: "/var/run/secrets/ca.crt",
			},
		},
	}
	for _, tc := range caFileTests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			cfg := JWTConfig{Enabled: true, Configs: []JWTIssuerConfig{tc.config}}
			err := cfg.Validate()
			if tc.expectErr != "" {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(tc.expectErr))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		})
	}

	httpsSchemeTests := []struct {
		name      string
		config    JWTIssuerConfig
		expectErr string
	}{
		{
			name: "HTTPS issuer and JWK URLs pass",
			config: JWTIssuerConfig{
				IssuerURL:  "https://issuer.example.com",
				JWKCertURL: "https://keys.example.com",
			},
		},
		{
			name: "HTTP issuer URL rejected",
			config: JWTIssuerConfig{
				IssuerURL:  "http://issuer.example.com",
				JWKCertURL: "https://keys.example.com",
			},
			expectErr: "issuer_url",
		},
		{
			name: "HTTP JWK cert URL rejected",
			config: JWTIssuerConfig{
				IssuerURL:  "https://issuer.example.com",
				JWKCertURL: "http://keys.example.com",
			},
			expectErr: "jwk_cert_url",
		},
		{
			name: "HTTP localhost issuer allowed",
			config: JWTIssuerConfig{
				IssuerURL:  "http://localhost:8080",
				JWKCertURL: "https://keys.example.com",
			},
		},
		{
			name: "HTTP 127.0.0.1 issuer allowed",
			config: JWTIssuerConfig{
				IssuerURL:  "http://127.0.0.1:8080",
				JWKCertURL: "https://keys.example.com",
			},
		},
		{
			name: "HTTP IPv6 loopback allowed",
			config: JWTIssuerConfig{
				IssuerURL:  "http://[::1]:8080",
				JWKCertURL: "https://keys.example.com",
			},
		},
		{
			name: "HTTP localhost JWK cert URL allowed",
			config: JWTIssuerConfig{
				IssuerURL:  "https://issuer.example.com",
				JWKCertURL: "http://localhost:8080/.well-known/jwks.json",
			},
		},
		{
			name: "JWK cert file only skips URL scheme check",
			config: JWTIssuerConfig{
				IssuerURL:   "https://issuer.example.com",
				JWKCertFile: "/etc/hyperfleet/jwks.json",
			},
		},
		{
			name: "URL without scheme rejected",
			config: JWTIssuerConfig{
				IssuerURL:  "issuer.example.com",
				JWKCertURL: "https://keys.example.com",
			},
			expectErr: "issuer_url",
		},
		{
			name: "HTTPS opaque URI without host rejected",
			config: JWTIssuerConfig{
				IssuerURL:  "https:issuer",
				JWKCertURL: "https://keys.example.com",
			},
			expectErr: "issuer_url",
		},
	}
	for _, tc := range httpsSchemeTests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			cfg := JWTConfig{Enabled: true, Configs: []JWTIssuerConfig{tc.config}}
			err := cfg.Validate()
			if tc.expectErr != "" {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(tc.expectErr))
				Expect(err.Error()).To(ContainSubstring("https"))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		})
	}

	t.Run("valid single issuer config with cert file passes", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: true, Configs: []JWTIssuerConfig{{
			IssuerURL:   "https://issuer.example.com",
			JWKCertFile: "/etc/hyperfleet/jwks.json",
		}}}
		Expect(cfg.Validate()).To(Succeed())
	})

	t.Run("ApplyDefaults sets header and identity_claim when empty", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: true, Configs: []JWTIssuerConfig{{
			IssuerURL:  "https://issuer.example.com",
			JWKCertURL: "https://keys.example.com",
		}}}
		cfg.ApplyDefaults()
		Expect(cfg.Validate()).To(Succeed())
		Expect(cfg.Configs[0].Header).To(Equal(DefaultJWTHeader))
		Expect(cfg.Configs[0].IdentityClaim).To(Equal(DefaultJWTIdentityClaim))
	})

	t.Run("invalid identity_claim_pattern fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: true, Configs: []JWTIssuerConfig{{
			IssuerURL:            "https://issuer.example.com",
			JWKCertURL:           "https://keys.example.com",
			IdentityClaimPattern: "[invalid",
		}}}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("identity_claim_pattern"))
	})

	t.Run("valid identity_claim_pattern passes", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: true, Configs: []JWTIssuerConfig{{
			IssuerURL:            "https://issuer.example.com",
			JWKCertURL:           "https://keys.example.com",
			IdentityClaimPattern: `^[^@]+@[^@]+$`,
		}}}
		Expect(cfg.Validate()).To(Succeed())
	})

	t.Run("multiple issuers all validated", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: true, Configs: []JWTIssuerConfig{
			{IssuerURL: "https://issuer1.example.com", JWKCertURL: "https://keys1.example.com"},
			{IssuerURL: "", JWKCertURL: "https://keys2.example.com"},
		}}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("configs[1]"))
	})

	t.Run("multiple issuers second has HTTP scheme rejected", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := JWTConfig{Enabled: true, Configs: []JWTIssuerConfig{
			{IssuerURL: "https://issuer1.example.com", JWKCertURL: "https://keys1.example.com"},
			{IssuerURL: "http://issuer2.example.com", JWKCertURL: "https://keys2.example.com"},
		}}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("configs[1]"))
		Expect(err.Error()).To(ContainSubstring("https"))
	})
}

func TestJWTConfig_ValidateIdentityHeader(t *testing.T) {
	RegisterTestingT(t)

	base := func(header string) *JWTConfig {
		return &JWTConfig{
			Enabled: true,
			Configs: []JWTIssuerConfig{{
				IssuerURL:      "https://issuer.example.com",
				JWKCertURL:     "https://issuer.example.com/.well-known/jwks.json",
				IdentityHeader: header,
			}},
		}
	}

	t.Run("empty identity header requires nothing", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := base("")
		Expect(cfg.Validate()).To(Succeed())
	})

	t.Run("forbidden header name fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := base("Authorization")
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not allowed"))
	})

	t.Run("valid header name passes", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := base("X-HyperFleet-Identity")
		Expect(cfg.Validate()).To(Succeed())
	})

	t.Run("forbidden JWT source header fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &JWTConfig{
			Enabled: true,
			Configs: []JWTIssuerConfig{{
				IssuerURL:  "https://issuer.example.com",
				JWKCertURL: "https://keys.example.com",
				Header:     "Cookie",
			}},
		}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not allowed as a JWT source"))
	})

	t.Run("header equal to identity_header fails", func(t *testing.T) {
		RegisterTestingT(t)
		cfg := &JWTConfig{
			Enabled: true,
			Configs: []JWTIssuerConfig{{
				IssuerURL:      "https://issuer.example.com",
				JWKCertURL:     "https://keys.example.com",
				Header:         "X-Token",
				IdentityHeader: "x-token",
			}},
		}
		err := cfg.Validate()
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("must differ from header"))
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
