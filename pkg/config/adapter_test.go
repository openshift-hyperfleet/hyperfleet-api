package config

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/spf13/cobra"
)

// Test that NewAdapterRequirementsConfig returns default empty config
func TestNewAdapterRequirementsConfig_ReturnsDefaults(t *testing.T) {
	RegisterTestingT(t)

	config := NewAdapterRequirementsConfig()

	Expect(config).NotTo(BeNil())
	Expect(config.RequiredClusterAdapters()).To(Equal([]string{}))
	Expect(config.RequiredNodePoolAdapters()).To(Equal([]string{}))
}

// Test loading adapter config via ConfigLoader with valid JSON env vars
func TestConfigLoader_AdaptersFromEnv(t *testing.T) {
	RegisterTestingT(t)

	// Set minimal test environment
	SetMinimalTestEnv(t)

	// Set environment variables for adapters
	t.Setenv("HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER", `["validation","dns"]`)
	t.Setenv("HYPERFLEET_ADAPTERS_REQUIRED_NODEPOOL", `["validation","hypershift"]`)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	// Load config
	appConfig, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())
	Expect(appConfig.Adapters).NotTo(BeNil())
	Expect(appConfig.Adapters.RequiredClusterAdapters()).To(Equal([]string{
		"validation",
		"dns",
	}))
	Expect(appConfig.Adapters.RequiredNodePoolAdapters()).To(Equal([]string{
		"validation",
		"hypershift",
	}))
}

// Test loading adapter config with single adapter
func TestConfigLoader_AdaptersSingleValue(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	t.Setenv("HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER", `["validation"]`)
	t.Setenv("HYPERFLEET_ADAPTERS_REQUIRED_NODEPOOL", `["hypershift"]`)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	appConfig, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())
	Expect(appConfig.Adapters.RequiredClusterAdapters()).To(Equal([]string{"validation"}))
	Expect(appConfig.Adapters.RequiredNodePoolAdapters()).To(Equal([]string{"hypershift"}))
}

// Test loading adapter config with empty arrays
func TestConfigLoader_AdaptersEmptyArrays(t *testing.T) {
	RegisterTestingT(t)

	appConfig, err := LoadTestConfig(t)

	Expect(err).NotTo(HaveOccurred())
	Expect(appConfig.Adapters.RequiredClusterAdapters()).To(Equal([]string{}))
	Expect(appConfig.Adapters.RequiredNodePoolAdapters()).To(Equal([]string{}))
}

// Test loading adapter config with invalid JSON should fail
func TestConfigLoader_AdaptersInvalidJSON(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	t.Setenv("HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER", `not-valid-json`)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	_, err := loader.Load(ctx, cmd)

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to parse"))
	Expect(err.Error()).To(ContainSubstring("HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER"))
}

// Test loading adapter config with custom adapter names
func TestConfigLoader_AdaptersCustomNames(t *testing.T) {
	RegisterTestingT(t)

	SetMinimalTestEnv(t)

	t.Setenv("HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER", `["custom-adapter-1","custom-adapter-2","test"]`)
	t.Setenv("HYPERFLEET_ADAPTERS_REQUIRED_NODEPOOL", `["custom-np-adapter"]`)

	loader := NewConfigLoader()
	cmd := &cobra.Command{}
	ctx := context.Background()

	appConfig, err := loader.Load(ctx, cmd)

	Expect(err).NotTo(HaveOccurred())
	Expect(appConfig.Adapters.RequiredClusterAdapters()).To(Equal([]string{
		"custom-adapter-1",
		"custom-adapter-2",
		"test",
	}))
	Expect(appConfig.Adapters.RequiredNodePoolAdapters()).To(Equal([]string{
		"custom-np-adapter",
	}))
}
