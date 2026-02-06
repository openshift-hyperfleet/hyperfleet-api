package config

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestNewAdapterRequirementsConfig_MissingClusterAdapters(t *testing.T) {
	RegisterTestingT(t)

	// Ensure env vars are not set
	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", "")
	t.Setenv("HYPERFLEET_NODEPOOL_ADAPTERS", `["validation"]`)

	_, err := NewAdapterRequirementsConfig()

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("HYPERFLEET_CLUSTER_ADAPTERS"))
	Expect(err.Error()).To(ContainSubstring("required"))
}

func TestNewAdapterRequirementsConfig_MissingNodePoolAdapters(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", `["validation"]`)
	t.Setenv("HYPERFLEET_NODEPOOL_ADAPTERS", "")

	_, err := NewAdapterRequirementsConfig()

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("HYPERFLEET_NODEPOOL_ADAPTERS"))
	Expect(err.Error()).To(ContainSubstring("required"))
}

func TestNewAdapterRequirementsConfig_MissingBothAdapters(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", "")
	t.Setenv("HYPERFLEET_NODEPOOL_ADAPTERS", "")

	_, err := NewAdapterRequirementsConfig()

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("HYPERFLEET_CLUSTER_ADAPTERS"))
}

func TestLoadFromEnv_ClusterAdapters(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", `["validation","dns"]`)
	t.Setenv("HYPERFLEET_NODEPOOL_ADAPTERS", `["validation","hypershift"]`)

	config, err := NewAdapterRequirementsConfig()

	Expect(err).NotTo(HaveOccurred())
	Expect(config.RequiredClusterAdapters).To(Equal([]string{
		"validation",
		"dns",
	}))
	Expect(config.RequiredNodePoolAdapters).To(Equal([]string{
		"validation",
		"hypershift",
	}))
}

func TestLoadFromEnv_BothAdapters(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", `["validation","dns"]`)
	t.Setenv("HYPERFLEET_NODEPOOL_ADAPTERS", `["validation"]`)

	config, err := NewAdapterRequirementsConfig()

	Expect(err).NotTo(HaveOccurred())
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
	t.Setenv("HYPERFLEET_NODEPOOL_ADAPTERS", `[]`)

	config, err := NewAdapterRequirementsConfig()

	Expect(err).NotTo(HaveOccurred())
	Expect(config.RequiredClusterAdapters).To(Equal([]string{}))
	Expect(config.RequiredNodePoolAdapters).To(Equal([]string{}))
}

func TestLoadFromEnv_InvalidJSON_ClusterAdapters(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", `not-valid-json`)
	t.Setenv("HYPERFLEET_NODEPOOL_ADAPTERS", `["validation"]`)

	_, err := NewAdapterRequirementsConfig()

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to parse HYPERFLEET_CLUSTER_ADAPTERS"))
}

func TestLoadFromEnv_InvalidJSON_NodePoolAdapters(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", `["validation"]`)
	t.Setenv("HYPERFLEET_NODEPOOL_ADAPTERS", `{invalid}`)

	_, err := NewAdapterRequirementsConfig()

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring("failed to parse HYPERFLEET_NODEPOOL_ADAPTERS"))
}

func TestLoadFromEnv_SingleAdapter(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", `["validation"]`)
	t.Setenv("HYPERFLEET_NODEPOOL_ADAPTERS", `["hypershift"]`)

	config, err := NewAdapterRequirementsConfig()

	Expect(err).NotTo(HaveOccurred())
	Expect(config.RequiredClusterAdapters).To(Equal([]string{
		"validation",
	}))
	Expect(config.RequiredNodePoolAdapters).To(Equal([]string{
		"hypershift",
	}))
}

func TestLoadFromEnv_CustomAdapterNames(t *testing.T) {
	RegisterTestingT(t)

	t.Setenv("HYPERFLEET_CLUSTER_ADAPTERS", `["custom-adapter-1","custom-adapter-2","test"]`)
	t.Setenv("HYPERFLEET_NODEPOOL_ADAPTERS", `["custom-np-adapter"]`)

	config, err := NewAdapterRequirementsConfig()

	Expect(err).NotTo(HaveOccurred())
	Expect(config.RequiredClusterAdapters).To(Equal([]string{
		"custom-adapter-1",
		"custom-adapter-2",
		"test",
	}))
	Expect(config.RequiredNodePoolAdapters).To(Equal([]string{
		"custom-np-adapter",
	}))
}
