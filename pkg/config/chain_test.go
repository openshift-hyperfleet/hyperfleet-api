package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

func TestConfigLoader_Chains(t *testing.T) {
	tests := []struct {
		name      string
		adapters  string
		errSubstr string
	}{
		{
			name:     "declared chain loads",
			adapters: "[coreshift]",
		},
		{
			name:      "unknown adapter fails startup",
			adapters:  "[missing]",
			errSubstr: `chain hosted/coreshift references unknown adapter "missing"`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			SetMinimalTestEnv(t)

			configPath := filepath.Join(t.TempDir(), "config.yaml")
			configContent := `
entities:
  - kind: Cluster
    plural: clusters
    required_adapters: [coreshift]
chains:
  - topology: hosted
    platform: coreshift
    adapters: ` + tc.adapters + `
`
			Expect(os.WriteFile(configPath, []byte(configContent), 0600)).To(Succeed())
			t.Setenv("HYPERFLEET_CONFIG", configPath)

			cfg, err := NewConfigLoader().Load(context.Background(), &cobra.Command{})
			if tc.errSubstr == "" {
				Expect(err).NotTo(HaveOccurred())
				Expect(cfg.Chains).To(HaveLen(1))
				Expect(cfg.Chains[0].Adapters).To(Equal([]string{"coreshift"}))
				return
			}
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("chain validation failed"))
			Expect(err.Error()).To(ContainSubstring(tc.errSubstr))
		})
	}
}
