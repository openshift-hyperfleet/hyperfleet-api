package handlers

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/util"
)

func TestValidateName_Valid(t *testing.T) {
	RegisterTestingT(t)

	validNames := []string{
		"test",
		"test-cluster",
		"my-cluster-123",
		"123",
		"test-123-cluster",
		"a1b2c3",
		"abc",
	}

	for _, name := range validNames {
		req := openapi.ClusterCreateRequest{
			Name: name,
		}
		validator := validateName(&req, "Name", "name", 3, 53)
		err := validator()
		Expect(err).To(BeNil(), "Expected name '%s' to be valid", name)
	}
}

func TestValidateName_TooShort(t *testing.T) {
	RegisterTestingT(t)

	shortNames := []string{
		"",   // empty
		"a",  // 1 char
		"ab", // 2 chars
	}

	for _, name := range shortNames {
		req := openapi.ClusterCreateRequest{
			Name: name,
		}
		validator := validateName(&req, "Name", "name", 3, 53)
		err := validator()
		Expect(err).ToNot(BeNil(), "Expected name '%s' to be invalid (too short)", name)
		if name == "" {
			Expect(err.Reason).To(ContainSubstring("required"))
		} else {
			Expect(err.Reason).To(ContainSubstring("at least 3 characters"))
		}
	}
}

func TestValidateName_TooLong(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{
		Name: "this-is-a-very-long-name-that-exceeds-the-maximum-allowed-length-for-cluster-names",
	}
	validator := validateName(&req, "Name", "name", 3, 53)
	err := validator()
	Expect(err).ToNot(BeNil())
	Expect(err.Reason).To(ContainSubstring("at most 53 characters"))
}

func TestValidateName_InvalidCharacters(t *testing.T) {
	RegisterTestingT(t)

	invalidNames := []string{
		"TEST",          // uppercase
		"Test",          // mixed case
		"test_cluster",  // underscore
		"test.cluster",  // dot
		"test cluster",  // space
		"test@cluster",  // special char
		"test/cluster",  // slash
		"test\\cluster", // backslash
		"-test",         // starts with hyphen
		"test-",         // ends with hyphen
		"-test-",        // starts and ends with hyphen
	}

	for _, name := range invalidNames {
		req := openapi.ClusterCreateRequest{
			Name: name,
		}
		validator := validateName(&req, "Name", "name", 3, 53)
		err := validator()
		Expect(err).ToNot(BeNil(), "Expected name '%s' to be invalid", name)
		Expect(err.Reason).To(ContainSubstring("lowercase letters, numbers, and hyphens"))
	}
}

func TestValidateKind_Valid(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{
		Kind: util.PtrString("Cluster"),
	}
	validator := validateKind(&req, "Kind", "kind", "Cluster")
	err := validator()
	Expect(err).To(BeNil())
}

func TestValidateKind_Invalid(t *testing.T) {
	RegisterTestingT(t)

	invalidKinds := []string{
		"cluster",  // lowercase
		"CLUSTER",  // uppercase
		"NodePool", // wrong kind
		"",         // empty
	}

	for _, kind := range invalidKinds {
		req := openapi.ClusterCreateRequest{
			Kind: &kind,
		}
		validator := validateKind(&req, "Kind", "kind", "Cluster")
		err := validator()
		Expect(err).ToNot(BeNil(), "Expected kind '%s' to be invalid", kind)
	}
}

func TestValidateKind_Empty(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{
		Kind: util.PtrString(""),
	}
	validator := validateKind(&req, "Kind", "kind", "Cluster")
	err := validator()
	Expect(err).ToNot(BeNil())
	Expect(err.Reason).To(ContainSubstring("required"))
}

func TestValidateKind_WrongKind(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{
		Kind: util.PtrString("WrongKind"),
	}
	validator := validateKind(&req, "Kind", "kind", "Cluster")
	err := validator()
	Expect(err).ToNot(BeNil())
	Expect(err.Reason).To(ContainSubstring("must be 'Cluster'"))
}

func TestValidateNodePoolName_Valid(t *testing.T) {
	RegisterTestingT(t)

	validNames := []string{
		"abc",
		"worker-pool-1",
		"np1",
		"a1b",
		"my-pool",
		"pool-123456789", // 15 chars
	}

	for _, name := range validNames {
		req := openapi.NodePoolCreateRequest{
			Name: name,
		}
		validator := validateName(&req, "Name", "name", 3, 15)
		err := validator()
		Expect(err).To(BeNil(), "Expected nodepool name '%s' to be valid", name)
	}
}

func TestValidateNodePoolName_TooShort(t *testing.T) {
	RegisterTestingT(t)

	shortNames := []string{
		"",   // empty
		"a",  // 1 char
		"ab", // 2 chars
	}

	for _, name := range shortNames {
		req := openapi.NodePoolCreateRequest{
			Name: name,
		}
		validator := validateName(&req, "Name", "name", 3, 15)
		err := validator()
		Expect(err).ToNot(BeNil(), "Expected nodepool name '%s' to be invalid (too short)", name)
		if name == "" {
			Expect(err.Reason).To(ContainSubstring("required"))
		} else {
			Expect(err.Reason).To(ContainSubstring("at least 3 characters"))
		}
	}
}

func TestValidateNodePoolName_TooLong(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.NodePoolCreateRequest{
		Name: "this-is-too-long", // 16 chars
	}
	validator := validateName(&req, "Name", "name", 3, 15)
	err := validator()
	Expect(err).ToNot(BeNil())
	Expect(err.Reason).To(ContainSubstring("at most 15 characters"))
}

func TestValidateNodePoolName_InvalidCharacters(t *testing.T) {
	RegisterTestingT(t)

	invalidNames := []string{
		"TEST",      // uppercase
		"Test",      // mixed case
		"test_pool", // underscore
		"test.pool", // dot
		"test pool", // space
		"-test",     // starts with hyphen
		"test-",     // ends with hyphen
	}

	for _, name := range invalidNames {
		req := openapi.NodePoolCreateRequest{
			Name: name,
		}
		validator := validateName(&req, "Name", "name", 3, 15)
		err := validator()
		Expect(err).ToNot(BeNil(), "Expected nodepool name '%s' to be invalid", name)
		Expect(err.Reason).To(ContainSubstring("lowercase letters, numbers, and hyphens"))
	}
}

func TestValidateSpec_Valid(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{
		Spec: map[string]interface{}{"test": "value"},
	}
	validator := validateSpec(&req, "Spec", "spec")
	err := validator()
	Expect(err).To(BeNil(), "Expected existing spec to be valid")
}

func TestValidateSpec_EmptyMap(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{
		Spec: map[string]interface{}{},
	}
	validator := validateSpec(&req, "Spec", "spec")
	err := validator()
	Expect(err).To(BeNil(), "Expected empty map spec to be valid")
}

func TestValidateSpec_Nil(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{
		Spec: nil,
	}
	validator := validateSpec(&req, "Spec", "spec")
	err := validator()
	Expect(err).ToNot(BeNil(), "Expected nil spec to be invalid")
	Expect(err.Reason).To(ContainSubstring("spec is required"))
}
