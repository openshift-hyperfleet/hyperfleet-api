package entities

import (
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	. "github.com/onsi/gomega"

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

	router := mux.NewRouter()
	apiV1 := router.PathPrefix("/api/hyperfleet/v1").Subrouter()
	RegisterEntityRoutes(apiV1, nil, nil)

	assertRouteMatches(t, router, "GET", "/api/hyperfleet/v1/channels")
	assertRouteMatches(t, router, "POST", "/api/hyperfleet/v1/channels")
	assertRouteMatches(t, router, "GET", "/api/hyperfleet/v1/channels/00000000-0000-0000-0000-000000000001")
	assertRouteMatches(t, router, "PATCH", "/api/hyperfleet/v1/channels/00000000-0000-0000-0000-000000000001")
	assertRouteMatches(t, router, "DELETE", "/api/hyperfleet/v1/channels/00000000-0000-0000-0000-000000000001")
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

	router := mux.NewRouter()
	apiV1 := router.PathPrefix("/api/hyperfleet/v1").Subrouter()
	RegisterEntityRoutes(apiV1, nil, nil)

	parentID := "00000000-0000-0000-0000-000000000001"
	childID := "00000000-0000-0000-0000-000000000002"
	nested := "/api/hyperfleet/v1/channels/" + parentID + "/versions"

	assertRouteMatches(t, router, "GET", nested)
	assertRouteMatches(t, router, "POST", nested)
	assertRouteMatches(t, router, "GET", nested+"/"+childID)
	assertRouteMatches(t, router, "PATCH", nested+"/"+childID)
	assertRouteMatches(t, router, "DELETE", nested+"/"+childID)

	flat := "/api/hyperfleet/v1/versions"
	assertRouteMatches(t, router, "GET", flat)
	assertRouteMatches(t, router, "POST", flat)
	assertRouteMatches(t, router, "GET", flat+"/"+childID)
	assertRouteMatches(t, router, "PATCH", flat+"/"+childID)
	assertRouteMatches(t, router, "DELETE", flat+"/"+childID)
}

func TestRegisterEntityRoutes_EmptyRegistry(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()

	router := mux.NewRouter()
	apiV1 := router.PathPrefix("/api/hyperfleet/v1").Subrouter()

	Expect(func() {
		RegisterEntityRoutes(apiV1, nil, nil)
	}).ToNot(Panic())
}

func assertRouteMatches(t *testing.T, router *mux.Router, method, path string) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	match := mux.RouteMatch{}
	Expect(router.Match(req, &match)).To(BeTrue(), "expected route to match: %s %s", method, path)
}
