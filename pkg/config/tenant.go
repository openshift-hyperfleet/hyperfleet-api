package config

import (
	"fmt"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validation"
)

// TenantDimension maps one trusted gateway-injected header to a tenancy key.
// Required dimensions must be present for a non-system caller to be granted
// a tenant context; see TenantConfig.Validate for the at-least-one-required rule.
type TenantDimension struct {
	Header   string `mapstructure:"header" json:"header"`
	Key      string `mapstructure:"key" json:"key"`
	Required bool   `mapstructure:"required" json:"required"`
}

// TenantConfig holds tenant enforcement middleware configuration.
// Identity arrives as trusted headers injected by the gateway (Envoy + Authorino);
// the API never extracts JWT claims for tenancy.
type TenantConfig struct {
	SystemHeader string            `mapstructure:"system_header" json:"system_header"`
	Dimensions   []TenantDimension `mapstructure:"dimensions" json:"dimensions"`
	Enabled      bool              `mapstructure:"enabled" json:"enabled"`
}

// Validate enforces the tenant configuration invariants. A caller resolving zero
// dimensions must never reach the data layer (an empty tenancy map would
// contain-match every row), so at least one dimension must be required.
func (c *TenantConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	if c.SystemHeader == "" {
		return fmt.Errorf("server.tenant.system_header is required when tenant is enabled")
	}
	if validation.IsForbiddenIdentityHeaderName(c.SystemHeader) {
		return fmt.Errorf("server.tenant.system_header %q is not allowed", c.SystemHeader)
	}
	if len(c.Dimensions) == 0 {
		return fmt.Errorf("server.tenant.dimensions requires at least one dimension when tenant is enabled")
	}

	seenHeaders := make(map[string]bool, len(c.Dimensions))
	seenKeys := make(map[string]bool, len(c.Dimensions))
	requiredCount := 0

	for i := range c.Dimensions {
		dim := &c.Dimensions[i]
		if err := c.validateDimension(i, dim, seenHeaders, seenKeys); err != nil {
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
	i int, dim *TenantDimension, seenHeaders, seenKeys map[string]bool,
) error {
	if dim.Header == "" {
		return fmt.Errorf("server.tenant.dimensions[%d].header is required", i)
	}
	if dim.Key == "" {
		return fmt.Errorf("server.tenant.dimensions[%d].key is required", i)
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

	return nil
}
