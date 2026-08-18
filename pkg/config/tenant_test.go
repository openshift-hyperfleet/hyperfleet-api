package config

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestTenantConfig_Validate(t *testing.T) {
	RegisterTestingT(t)

	validDimension := TenantDimension{Header: "X-HyperFleet-Org", Key: "org", Required: true}

	cases := []struct {
		name      string
		expectErr string
		config    TenantConfig
	}{
		{
			name:   "disabled tenant requires nothing",
			config: TenantConfig{Enabled: false},
		},
		{
			name: "valid config with single required dimension passes",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
				Dimensions:   []TenantDimension{validDimension},
			},
		},
		{
			name: "valid config with mixed required and optional dimensions passes",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
				Dimensions: []TenantDimension{
					validDimension,
					{Header: "X-HyperFleet-Project", Key: "project", Required: false},
				},
			},
		},
		{
			name: "enabled with empty system header fails",
			config: TenantConfig{
				Enabled:    true,
				Dimensions: []TenantDimension{validDimension},
			},
			expectErr: "system_header is required",
		},
		{
			name: "forbidden system header name fails",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "Authorization",
				Dimensions:   []TenantDimension{validDimension},
			},
			expectErr: "system_header \"Authorization\" is not allowed",
		},
		{
			name: "enabled with zero dimensions fails",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
			},
			expectErr: "requires at least one dimension when tenant is enabled",
		},
		{
			name: "dimension missing header fails",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
				Dimensions:   []TenantDimension{{Key: "org", Required: true}},
			},
			expectErr: "dimensions[0].header is required",
		},
		{
			name: "dimension missing key fails",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
				Dimensions:   []TenantDimension{{Header: "X-HyperFleet-Org", Required: true}},
			},
			expectErr: "dimensions[0].key is required",
		},
		{
			name: "forbidden dimension header name fails",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
				Dimensions:   []TenantDimension{{Header: "Cookie", Key: "org", Required: true}},
			},
			expectErr: "dimensions[0].header \"Cookie\" is not allowed",
		},
		{
			name: "dimension header equal to system header fails",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
				Dimensions:   []TenantDimension{{Header: "x-hyperfleet-system", Key: "org", Required: true}},
			},
			expectErr: "must differ from system_header",
		},
		{
			name: "duplicate dimension headers fail",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
				Dimensions: []TenantDimension{
					{Header: "X-HyperFleet-Org", Key: "org", Required: true},
					{Header: "x-hyperfleet-org", Key: "org2", Required: false},
				},
			},
			expectErr: "dimensions[1].header \"x-hyperfleet-org\" is a duplicate",
		},
		{
			name: "duplicate dimension keys fail",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
				Dimensions: []TenantDimension{
					{Header: "X-HyperFleet-Org", Key: "org", Required: true},
					{Header: "X-HyperFleet-Project", Key: "org", Required: false},
				},
			},
			expectErr: "dimensions[1].key \"org\" is a duplicate",
		},
		{
			name: "zero required dimensions fails",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
				Dimensions: []TenantDimension{
					{Header: "X-HyperFleet-Org", Key: "org", Required: false},
					{Header: "X-HyperFleet-Project", Key: "project", Required: false},
				},
			},
			expectErr: "at least one dimension with required: true",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tc.config.Validate()
			if tc.expectErr != "" {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(tc.expectErr))
			} else {
				Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}
