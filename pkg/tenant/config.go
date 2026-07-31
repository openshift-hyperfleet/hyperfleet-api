package tenant

import "fmt"

// TenantConfig holds tenant enforcement configuration. Dimensions are
// deployment configuration: each deployment declares its own tenant model
// (which gateway headers map to which tenancy keys), so the tenant model is
// never compiled into application code.
type TenantConfig struct {
	// SystemHeader is the HTTP header the authorization gateway sets to the
	// literal "true" for trusted system identities (Sentinel, adapters).
	// System callers bypass tenant scoping entirely.
	SystemHeader string            `mapstructure:"system_header" json:"system_header"`
	Dimensions   []DimensionConfig `mapstructure:"dimensions" json:"dimensions"`
	Enabled      bool              `mapstructure:"enabled" json:"enabled"`
}

// DimensionConfig maps a gateway-injected HTTP header to a tenancy map key.
type DimensionConfig struct {
	// Header is the HTTP header name injected by the authorization gateway
	// (e.g. "X-Tenant-Org").
	Header string `mapstructure:"header" json:"header"`
	// Key is the tenancy map key the header value is stored and filtered
	// under (e.g. "org").
	Key string `mapstructure:"key" json:"key"`
	// Required means the header must be present and non-empty; requests
	// without it are rejected.
	Required bool `mapstructure:"required" json:"required"`
}

// Validate checks that the config is internally consistent. At least one
// required dimension must exist so an enforced request can never resolve to
// an empty tenancy map, which would match every resource under containment
// filtering.
func (c *TenantConfig) Validate() error {
	if !c.Enabled {
		return nil
	}
	if c.SystemHeader == "" {
		return fmt.Errorf("tenant.system_header is required when tenant enforcement is enabled")
	}
	if len(c.Dimensions) == 0 {
		return fmt.Errorf("tenant.dimensions requires at least one entry when tenant enforcement is enabled")
	}
	seenHeaders := make(map[string]bool)
	seenKeys := make(map[string]bool)
	hasRequired := false
	for i, dim := range c.Dimensions {
		if dim.Header == "" {
			return fmt.Errorf("tenant.dimensions[%d].header is required", i)
		}
		if dim.Key == "" {
			return fmt.Errorf("tenant.dimensions[%d].key is required", i)
		}
		if seenHeaders[dim.Header] {
			return fmt.Errorf("tenant.dimensions[%d].header %q is duplicated", i, dim.Header)
		}
		if seenKeys[dim.Key] {
			return fmt.Errorf("tenant.dimensions[%d].key %q is duplicated", i, dim.Key)
		}
		seenHeaders[dim.Header] = true
		seenKeys[dim.Key] = true
		if dim.Required {
			hasRequired = true
		}
	}
	if !hasRequired {
		return fmt.Errorf("tenant.dimensions requires at least one required dimension when tenant enforcement is enabled")
	}
	return nil
}
