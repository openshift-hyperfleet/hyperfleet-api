package middleware

import (
	"net/http"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
)

// clusterFirstEntities lists clusters before nodepools to ensure matching does not depend on slice order.
var clusterFirstEntities = []registry.EntityDescriptor{
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
}

func TestShouldValidateRequest_PostNestedNodePoolPrefersNodePoolOverCluster(t *testing.T) {
	RegisterTestingT(t)

	should, plural := shouldValidateRequest(
		http.MethodPost,
		"/api/hyperfleet/v1/clusters/abc-123/nodepools",
		clusterFirstEntities,
	)

	Expect(should).To(BeTrue())
	Expect(plural).To(Equal("nodepools"))
}

func TestShouldValidateRequest_PatchNestedNodePoolPrefersNodePoolOverCluster(t *testing.T) {
	RegisterTestingT(t)

	should, plural := shouldValidateRequest(
		http.MethodPatch,
		"/api/hyperfleet/v1/clusters/abc-123/nodepools/np-456",
		clusterFirstEntities,
	)

	Expect(should).To(BeTrue())
	Expect(plural).To(Equal("nodepools"))
}

func TestShouldValidateRequest_PostClusterCollectionMatchesCluster(t *testing.T) {
	RegisterTestingT(t)

	should, plural := shouldValidateRequest(
		http.MethodPost,
		"/api/hyperfleet/v1/clusters",
		clusterFirstEntities,
	)

	Expect(should).To(BeTrue())
	Expect(plural).To(Equal("clusters"))
}

func TestShouldValidateRequest_PatchClusterResourceMatchesCluster(t *testing.T) {
	RegisterTestingT(t)

	should, plural := shouldValidateRequest(
		http.MethodPatch,
		"/api/hyperfleet/v1/clusters/abc-123",
		clusterFirstEntities,
	)

	Expect(should).To(BeTrue())
	Expect(plural).To(Equal("clusters"))
}

func TestShouldValidateRequest_PostNestedVersionPrefersVersionOverChannel(t *testing.T) {
	RegisterTestingT(t)

	should, plural := shouldValidateRequest(
		http.MethodPost,
		"/api/hyperfleet/v1/channels/stable/versions",
		clusterFirstEntities,
	)

	Expect(should).To(BeTrue())
	Expect(plural).To(Equal("versions"))
}

func TestShouldValidateRequest_GetSkipsValidation(t *testing.T) {
	RegisterTestingT(t)

	should, plural := shouldValidateRequest(
		http.MethodGet,
		"/api/hyperfleet/v1/clusters/abc-123/nodepools",
		clusterFirstEntities,
	)

	Expect(should).To(BeFalse())
	Expect(plural).To(BeEmpty())
}
