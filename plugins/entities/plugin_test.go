package entities

import (
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
)

func TestRegisterEntityRoutes_TopLevelEntity(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:           "Channel",
		Plural:         "channels",
		SpecSchemaName: "ChannelSpec",
	})

	apiV1 := server.NewRouter()
	RegisterEntityRoutes(apiV1, nil, nil, nil)

	id := uuid.NewString()
	assertRouteMatches(t, apiV1, "GET", "/api/hyperfleet/v1/channels")
	assertRouteMatches(t, apiV1, "POST", "/api/hyperfleet/v1/channels")
	assertRouteMatches(t, apiV1, "GET", "/api/hyperfleet/v1/channels/"+id)
	assertRouteMatches(t, apiV1, "PATCH", "/api/hyperfleet/v1/channels/"+id)
	assertRouteMatches(t, apiV1, "DELETE", "/api/hyperfleet/v1/channels/"+id)
	assertRouteMatches(t, apiV1, "GET", "/api/hyperfleet/v1/channels/"+id+"/statuses")
	assertRouteMatches(t, apiV1, "PUT", "/api/hyperfleet/v1/channels/"+id+"/statuses")

	// Root /resources routes should also have statuses
	assertRouteMatches(t, apiV1, "GET", "/api/hyperfleet/v1/resources/"+id+"/statuses")
	assertRouteMatches(t, apiV1, "PUT", "/api/hyperfleet/v1/resources/"+id+"/statuses")
}

func TestRegisterEntityRoutes_ChildEntity(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{Kind: "Channel", Plural: "channels"})
	registry.Register(registry.EntityDescriptor{
		Kind:       "Version",
		Plural:     "versions",
		ParentKind: "Channel",
	})

	apiV1 := server.NewRouter()
	RegisterEntityRoutes(apiV1, nil, nil, nil)

	parentID := uuid.NewString()
	childID := uuid.NewString()
	nested := "/api/hyperfleet/v1/channels/" + parentID + "/versions"

	assertRouteMatches(t, apiV1, "GET", nested)
	assertRouteMatches(t, apiV1, "POST", nested)
	assertRouteMatches(t, apiV1, "GET", nested+"/"+childID)
	assertRouteMatches(t, apiV1, "PATCH", nested+"/"+childID)
	assertRouteMatches(t, apiV1, "DELETE", nested+"/"+childID)
	assertRouteMatches(t, apiV1, "GET", nested+"/"+childID+"/statuses")
	assertRouteMatches(t, apiV1, "PUT", nested+"/"+childID+"/statuses")

	flat := "/api/hyperfleet/v1/versions"
	assertRouteMatches(t, apiV1, "GET", flat)
	assertRouteMatches(t, apiV1, "POST", flat)
	assertRouteMatches(t, apiV1, "GET", flat+"/"+childID)
	assertRouteMatches(t, apiV1, "PATCH", flat+"/"+childID)
	assertRouteMatches(t, apiV1, "DELETE", flat+"/"+childID)
	assertRouteMatches(t, apiV1, "GET", flat+"/"+childID+"/statuses")
	assertRouteMatches(t, apiV1, "PUT", flat+"/"+childID+"/statuses")
}

func TestRegisterEntityRoutes_UnresolvableParentKind_Panics(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:       "Version",
		Plural:     "versions",
		ParentKind: "NonExistent",
	})

	apiV1 := server.NewRouter()

	Expect(func() {
		RegisterEntityRoutes(apiV1, nil, nil, nil)
	}).To(PanicWith(ContainSubstring("not registered")))
}

func TestRegisterEntityRoutes_EmptyRegistry(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()

	apiV1 := server.NewRouter()

	Expect(func() {
		RegisterEntityRoutes(apiV1, nil, nil, nil)
	}).ToNot(Panic())
}

func assertRouteMatches(t *testing.T, router *server.Router, method, path string) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	_, pattern := router.Handler(req)
	Expect(pattern).NotTo(BeEmpty(), "expected route to match: %s %s", method, path)
}
