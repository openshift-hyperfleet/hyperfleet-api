package config

import (
	"fmt"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validation"
)

// TenantDimension maps one trusted gateway-injected header, and the matching
// wristband claim, to a tenancy key. Required dimensions must be present for a
// non-system caller to be granted a tenant context; see TenantConfig.Validate
// for the at-least-one-required rule.
type TenantDimension struct {
	Header   string `mapstructure:"header" json:"header"`
	Claim    string `mapstructure:"claim" json:"claim"`
	Key      string `mapstructure:"key" json:"key"`
	Required bool   `mapstructure:"required" json:"required"`
}

// EffectiveClaim returns the claim name read from a wristband, defaulting to
// the key (oracle model); onprem sets claim explicitly (org_id vs key org).
func (d TenantDimension) EffectiveClaim() string {
	if d.Claim != "" {
		return d.Claim
	}
	return d.Key
}

// TenantConfig holds tenant enforcement middleware configuration. Identity
// arrives as gateway-injected headers (edge mode) or, in edge+api mode with
// ClaimsEnabled, from the signed wristband's claims, ignoring headers.
type TenantConfig struct {
	SystemHeader  string            `mapstructure:"system_header" json:"system_header"`
	Dimensions    []TenantDimension `mapstructure:"dimensions" json:"dimensions"`
	Enabled       bool              `mapstructure:"enabled" json:"enabled"`
	ClaimsEnabled bool              `mapstructure:"claims_enabled" json:"claims_enabled"`
}

// Validate enforces the tenant configuration invariants, including that
// claims mode requires JWT validation (jwtEnabled is server.jwt.enabled,
// outside this struct). A caller resolving zero dimensions must never reach
// the data layer (an empty tenancy map would contain-match every row), so at
// least one dimension must be required.
func (c *TenantConfig) Validate(jwtEnabled bool) error {
	if !c.Enabled {
		return nil
	}

	if c.SystemHeader == "" {
		return fmt.Errorf("server.tenant.system_header is required when tenant is enabled")
	}
	if c.ClaimsEnabled && !jwtEnabled {
		return fmt.Errorf(
			"server.tenant.claims_enabled requires server.jwt.enabled: without JWT validation " +
				"no token reaches the resolver and it silently falls back to forgeable headers",
		)
	}
	if !validation.IsValidHeaderName(c.SystemHeader) {
		return fmt.Errorf("server.tenant.system_header %q is not a valid HTTP header name", c.SystemHeader)
	}
	if validation.IsForbiddenIdentityHeaderName(c.SystemHeader) {
		return fmt.Errorf("server.tenant.system_header %q is not allowed", c.SystemHeader)
	}
	if len(c.Dimensions) == 0 {
		return fmt.Errorf("server.tenant.dimensions requires at least one dimension when tenant is enabled")
	}

	seenHeaders := make(map[string]bool, len(c.Dimensions))
	seenKeys := make(map[string]bool, len(c.Dimensions))
	seenClaims := make(map[string]bool, len(c.Dimensions))
	requiredCount := 0

	for i := range c.Dimensions {
		dim := &c.Dimensions[i]
		if err := c.validateDimension(i, dim, seenHeaders, seenKeys, seenClaims); err != nil {
			return err
		}
		if dim.Required {
			requiredCount++
		}
	}

	if requiredCount == 0 {
		return fmt.Errorf("server.tenant.dimensions requires at least one dimension with required: true")
	}

	return nil
}

// validateDimension checks a single dimension against the config-level rules:
// required fields, forbidden/colliding header names, and duplicate headers or keys
// across previously validated dimensions in the same config (tracked via seenHeaders/seenKeys).
func (c *TenantConfig) validateDimension(
	i int, dim *TenantDimension, seenHeaders, seenKeys, seenClaims map[string]bool,
) error {
	if dim.Header == "" {
		return fmt.Errorf("server.tenant.dimensions[%d].header is required", i)
	}
	if dim.Key == "" {
		return fmt.Errorf("server.tenant.dimensions[%d].key is required", i)
	}
	if !validation.IsValidHeaderName(dim.Header) {
		return fmt.Errorf("server.tenant.dimensions[%d].header %q is not a valid HTTP header name", i, dim.Header)
	}
	if validation.IsForbiddenIdentityHeaderName(dim.Header) {
		return fmt.Errorf("server.tenant.dimensions[%d].header %q is not allowed", i, dim.Header)
	}
	if strings.EqualFold(dim.Header, c.SystemHeader) {
		return fmt.Errorf("server.tenant.dimensions[%d].header %q must differ from system_header", i, dim.Header)
	}

	headerKey := strings.ToLower(dim.Header)
	if seenHeaders[headerKey] {
		return fmt.Errorf("server.tenant.dimensions[%d].header %q is a duplicate", i, dim.Header)
	}
	seenHeaders[headerKey] = true

	if seenKeys[dim.Key] {
		return fmt.Errorf("server.tenant.dimensions[%d].key %q is a duplicate", i, dim.Key)
	}
	seenKeys[dim.Key] = true

	claim := dim.EffectiveClaim()
	if seenClaims[claim] {
		return fmt.Errorf("server.tenant.dimensions[%d] resolves duplicate claim %q", i, claim)
	}
	seenClaims[claim] = true

	return nil
}
