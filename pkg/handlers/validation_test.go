package handlers

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
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
		validator := validateName(&req, "Name", "name", 3, 63)
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
		validator := validateName(&req, "Name", "name", 3, 63)
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
	validator := validateName(&req, "Name", "name", 3, 63)
	err := validator()
	Expect(err).ToNot(BeNil())
	Expect(err.Reason).To(ContainSubstring("at most 63 characters"))
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
		validator := validateName(&req, "Name", "name", 3, 63)
		err := validator()
		Expect(err).ToNot(BeNil(), "Expected name '%s' to be invalid", name)
		Expect(err.Reason).To(ContainSubstring("lowercase letters, numbers, and hyphens"))
	}
}

func TestValidateKind_Valid(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{
		Kind: openapi.PtrString("Cluster"),
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
		Kind: openapi.PtrString(""),
	}
	validator := validateKind(&req, "Kind", "kind", "Cluster")
	err := validator()
	Expect(err).ToNot(BeNil())
	Expect(err.Reason).To(ContainSubstring("required"))
}

func TestValidateKind_WrongKind(t *testing.T) {
	RegisterTestingT(t)

	req := openapi.ClusterCreateRequest{
		Kind: openapi.PtrString("WrongKind"),
	}
	validator := validateKind(&req, "Kind", "kind", "Cluster")
	err := validator()
	Expect(err).ToNot(BeNil())
	Expect(err.Reason).To(ContainSubstring("must be 'Cluster'"))
}
