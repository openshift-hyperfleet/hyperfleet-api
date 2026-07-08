package config

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestDumpConfig_NilConfig(t *testing.T) {
	RegisterTestingT(t)
	Expect(DumpConfig(nil)).To(Equal("nil config"))
}

func TestDumpConfig_NoIssuers(t *testing.T) {
	RegisterTestingT(t)

	cfg := NewApplicationConfig()
	cfg.Server.JWT.Enabled = false

	result := DumpConfig(cfg)
	Expect(result).To(ContainSubstring("EnableJWT: false"))
	Expect(result).To(ContainSubstring("Issuers: 0 configured"))
	Expect(result).NotTo(ContainSubstring("[0]"))
}

func TestDumpConfig_WithIssuers(t *testing.T) {
	RegisterTestingT(t)

	cfg := NewApplicationConfig()
	cfg.Server.JWT.Enabled = true
	cfg.Server.JWT.Configs = []JWTIssuerConfig{
		{IssuerURL: "https://accounts.google.com", Header: "Authorization", JWKCertURL: "https://jwks.example.com"},
		{IssuerURL: "https://login.example.com", Header: "X-Token", JWKCertFile: "/path/to/keys.json"},
		{IssuerURL: "https://misconfigured.com", Header: "Authorization"},
	}

	result := DumpConfig(cfg)
	Expect(result).To(ContainSubstring("Issuers: 3 configured"))
	Expect(result).To(ContainSubstring("[0] IssuerURL: https://accounts.google.com, Header: Authorization, JWK: url"))
	Expect(result).To(ContainSubstring("[1] IssuerURL: https://login.example.com, Header: X-Token, JWK: file"))
	Expect(result).To(ContainSubstring("[2] IssuerURL: https://misconfigured.com, Header: Authorization, JWK: none"))
}
