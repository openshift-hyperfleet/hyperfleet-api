package tenant

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestTenantConfigValidate(t *testing.T) {
	RegisterTestingT(t)

	dim := func(header, key string, required bool) DimensionConfig {
		return DimensionConfig{Header: header, Key: key, Required: required}
	}

	cases := []struct {
		name    string
		wantErr string
		config  TenantConfig
	}{
		{
			name:   "disabled config is always valid",
			config: TenantConfig{Enabled: false},
		},
		{
			name: "valid single required dimension",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
				Dimensions:   []DimensionConfig{dim("X-Tenant-Org", "org", true)},
			},
		},
		{
			name: "valid mixed required and optional",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
				Dimensions: []DimensionConfig{
					dim("X-Tenant-Org", "org", true),
					dim("X-Tenant-Project", "project", false),
				},
			},
		},
		{
			name: "missing system header",
			config: TenantConfig{
				Enabled:    true,
				Dimensions: []DimensionConfig{dim("X-Tenant-Org", "org", true)},
			},
			wantErr: "system_header",
		},
		{
			name: "no dimensions",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
			},
			wantErr: "at least one entry",
		},
		{
			name: "no required dimension",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
				Dimensions:   []DimensionConfig{dim("X-Tenant-Org", "org", false)},
			},
			wantErr: "at least one required dimension",
		},
		{
			name: "empty header",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
				Dimensions:   []DimensionConfig{dim("", "org", true)},
			},
			wantErr: "header is required",
		},
		{
			name: "empty key",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
				Dimensions:   []DimensionConfig{dim("X-Tenant-Org", "", true)},
			},
			wantErr: "key is required",
		},
		{
			name: "duplicate header",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
				Dimensions: []DimensionConfig{
					dim("X-Tenant-Org", "org", true),
					dim("X-Tenant-Org", "other", false),
				},
			},
			wantErr: "duplicated",
		},
		{
			name: "duplicate key",
			config: TenantConfig{
				Enabled:      true,
				SystemHeader: "X-HyperFleet-System",
				Dimensions: []DimensionConfig{
					dim("X-Tenant-Org", "org", true),
					dim("X-Tenant-Other", "org", false),
				},
			},
			wantErr: "duplicated",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			err := tc.config.Validate()
			if tc.wantErr == "" {
				Expect(err).ToNot(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring(tc.wantErr))
			}
		})
	}
}
