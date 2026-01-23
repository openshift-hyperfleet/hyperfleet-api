package config

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestNewAdapterRequirementsConfig_Defaults(t *testing.T) {
	RegisterTestingT(t)

	// Ensure env vars are not set
	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", "")
	t.Setenv("HYPERFLEET_NODEPOOL_ADAPTERS", "")

	config := NewAdapterRequirementsConfig()

	// Verify default cluster adapters
	Expect(config.RequiredClusterAdapters).To(Equal([]string{
		"validation",
		"dns",
		"pullsecret",
		"hypershift",
	}))

	// Verify default nodepool adapters
	Expect(config.RequiredNodePoolAdapters).To(Equal([]string{
		"validation",
		"hypershift",
	}))
}

func TestLoadFromEnv_ClusterAdapters(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", `["validation","dns"]`)
	t.Setenv("HYPERFLEET_NODEPOOL_ADAPTERS", "")

	config := NewAdapterRequirementsConfig()

	// Verify cluster adapters from env
	Expect(config.RequiredClusterAdapters).To(Equal([]string{
		"validation",
		"dns",
	}))

	// Verify nodepool adapters use defaults
	Expect(config.RequiredNodePoolAdapters).To(Equal([]string{
		"validation",
		"hypershift",
	}))
}

func TestLoadFromEnv_NodePoolAdapters(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HYPERFLEET_NODEPOOL_ADAPTERS", `["validation"]`)
	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", "")

	config := NewAdapterRequirementsConfig()

	// Verify cluster adapters use defaults
	Expect(config.RequiredClusterAdapters).To(Equal([]string{
		"validation",
		"dns",
		"pullsecret",
		"hypershift",
	}))

	// Verify nodepool adapters from env
	Expect(config.RequiredNodePoolAdapters).To(Equal([]string{
		"validation",
	}))
}

func TestLoadFromEnv_BothAdapters(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", `["validation","dns"]`)
	t.Setenv("HYPERFLEET_NODEPOOL_ADAPTERS", `["validation"]`)

	config := NewAdapterRequirementsConfig()

	// Verify both are loaded from env
	Expect(config.RequiredClusterAdapters).To(Equal([]string{
		"validation",
		"dns",
	}))
	Expect(config.RequiredNodePoolAdapters).To(Equal([]string{
		"validation",
	}))
}

func TestLoadFromEnv_EmptyArray(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", `[]`)

	config := NewAdapterRequirementsConfig()

	// Verify empty array is loaded
	Expect(config.RequiredClusterAdapters).To(Equal([]string{}))
}

func TestLoadFromEnv_InvalidJSON_ClusterAdapters(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", `not-valid-json`)

	config := NewAdapterRequirementsConfig()

	// Verify defaults are used when JSON is invalid
	Expect(config.RequiredClusterAdapters).To(Equal([]string{
		"validation",
		"dns",
		"pullsecret",
		"hypershift",
	}))
}

func TestLoadFromEnv_InvalidJSON_NodePoolAdapters(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HYPERFLEET_NODEPOOL_ADAPTERS", `{invalid}`)

	config := NewAdapterRequirementsConfig()

	// Verify defaults are used when JSON is invalid
	Expect(config.RequiredNodePoolAdapters).To(Equal([]string{
		"validation",
		"hypershift",
	}))
}

func TestLoadFromEnv_SingleAdapter(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", `["validation"]`)

	config := NewAdapterRequirementsConfig()

	// Verify single adapter is loaded
	Expect(config.RequiredClusterAdapters).To(Equal([]string{
		"validation",
	}))
}

func TestLoadFromEnv_CustomAdapterNames(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", `["custom-adapter-1","custom-adapter-2","test"]`)

	config := NewAdapterRequirementsConfig()

	// Verify custom adapters are loaded
	Expect(config.RequiredClusterAdapters).To(Equal([]string{
		"custom-adapter-1",
		"custom-adapter-2",
		"test",
	}))
}
