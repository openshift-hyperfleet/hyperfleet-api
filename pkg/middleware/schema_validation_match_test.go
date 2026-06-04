package middleware

import (
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
)

// matchers lists clusters before nodepools to ensure matching does not depend on slice order.
var matchers = buildMatchers([]registry.EntityDescriptor{
	{
		Kind:           "Cluster",
		Plural:         "clusters",
		SpecSchemaName: "ClusterSpec",
	},
	{
		Kind:           "NodePool",
		Plural:         "nodepools",
		ParentKind:     "Cluster",
		SpecSchemaName: "NodePoolSpec",
	},
	{
		Kind:           "Channel",
		Plural:         "channels",
		SpecSchemaName: "ChannelSpec",
	},
	{
		Kind:           "Version",
		Plural:         "versions",
		ParentKind:     "Channel",
		SpecSchemaName: "VersionSpec",
	},
})

func TestShouldValidateRequest_PostNestedNodePoolPrefersNodePoolOverCluster(t *testing.T) {
	RegisterTestingT(t)

	should, plural := shouldValidateRequest(
		http.MethodPost,
		"/api/hyperfleet/v1/clusters/550e8400-e29b-41d4-a716-446655440000/nodepools",
		matchers,
	)

	Expect(should).To(BeTrue())
	Expect(plural).To(Equal("nodepools"))
}

func TestShouldValidateRequest_PatchNestedNodePoolPrefersNodePoolOverCluster(t *testing.T) {
	RegisterTestingT(t)

	should, plural := shouldValidateRequest(
		http.MethodPatch,
		"/api/hyperfleet/v1/clusters/550e8400-e29b-41d4-a716-446655440000/nodepools/660e8400-e29b-41d4-a716-446655440001",
		matchers,
	)

	Expect(should).To(BeTrue())
	Expect(plural).To(Equal("nodepools"))
}

func TestShouldValidateRequest_PostClusterCollectionMatchesCluster(t *testing.T) {
	RegisterTestingT(t)

	should, plural := shouldValidateRequest(
		http.MethodPost,
		"/api/hyperfleet/v1/clusters",
		matchers,
	)

	Expect(should).To(BeTrue())
	Expect(plural).To(Equal("clusters"))
}

func TestShouldValidateRequest_PatchClusterResourceMatchesCluster(t *testing.T) {
	RegisterTestingT(t)

	should, plural := shouldValidateRequest(
		http.MethodPatch,
		"/api/hyperfleet/v1/clusters/550e8400-e29b-41d4-a716-446655440000",
		matchers,
	)

	Expect(should).To(BeTrue())
	Expect(plural).To(Equal("clusters"))
}

func TestShouldValidateRequest_PatchVersionResourceMatchesVersion(t *testing.T) {
	RegisterTestingT(t)

	should, plural := shouldValidateRequest(
		http.MethodPatch,
		"/api/hyperfleet/v1/channels/cc0e8400-e29b-41d4-a716-446655440000/versions/550e8400-e29b-41d4-a716-446655440000",
		matchers,
	)

	Expect(should).To(BeTrue())
	Expect(plural).To(Equal("versions"))
}

func TestShouldValidateRequest_PostNestedVersionPrefersVersionOverChannel(t *testing.T) {
	RegisterTestingT(t)

	should, plural := shouldValidateRequest(
		http.MethodPost,
		"/api/hyperfleet/v1/channels/cc0e8400-e29b-41d4-a716-446655440000/versions",
		matchers,
	)

	Expect(should).To(BeTrue())
	Expect(plural).To(Equal("versions"))
}

func TestShouldValidateRequest_GetSkipsValidation(t *testing.T) {
	RegisterTestingT(t)

	should, plural := shouldValidateRequest(
		http.MethodGet,
		"/api/hyperfleet/v1/clusters/550e8400-e29b-41d4-a716-446655440000/nodepools",
		matchers,
	)

	Expect(should).To(BeFalse())
	Expect(plural).To(BeEmpty())
}
