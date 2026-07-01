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
	RegisterEntityRoutes(apiV1, nil)

	assertRouteMatches(t, router, "GET", "/api/hyperfleet/v1/channels")
	assertRouteMatches(t, router, "POST", "/api/hyperfleet/v1/channels")
	assertRouteMatches(t, router, "GET", "/api/hyperfleet/v1/channels/00000000-0000-0000-0000-000000000001")
	assertRouteMatches(t, router, "PATCH", "/api/hyperfleet/v1/channels/00000000-0000-0000-0000-000000000001")
	assertRouteMatches(t, router, "DELETE", "/api/hyperfleet/v1/channels/00000000-0000-0000-0000-000000000001")
}

func TestRegisterEntityRoutes_ChildEntity_NestedOnly(t *testing.T) {
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
	RegisterEntityRoutes(apiV1, nil)

	parentID := "00000000-0000-0000-0000-000000000001"
	childID := "00000000-0000-0000-0000-000000000002"
	base := "/api/hyperfleet/v1/channels/" + parentID + "/versions"

	assertRouteMatches(t, router, "GET", base)
	assertRouteMatches(t, router, "POST", base)
	assertRouteMatches(t, router, "GET", base+"/"+childID)
	assertRouteMatches(t, router, "PATCH", base+"/"+childID)
	assertRouteMatches(t, router, "DELETE", base+"/"+childID)

	assertRouteNotFound(t, router, "GET", "/api/hyperfleet/v1/versions")
	assertRouteNotFound(t, router, "POST", "/api/hyperfleet/v1/versions")
}

func TestRegisterEntityRoutes_EmptyRegistry(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()

	router := mux.NewRouter()
	apiV1 := router.PathPrefix("/api/hyperfleet/v1").Subrouter()

	Expect(func() {
		RegisterEntityRoutes(apiV1, nil)
	}).ToNot(Panic())
}

func assertRouteMatches(t *testing.T, router *mux.Router, method, path string) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	match := mux.RouteMatch{}
	Expect(router.Match(req, &match)).To(BeTrue(), "expected route to match: %s %s", method, path)
}

func assertRouteNotFound(t *testing.T, router *mux.Router, method, path string) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	match := mux.RouteMatch{}
	matched := router.Match(req, &match)
	if matched && match.MatchErr == nil && match.Handler != nil {
		t.Errorf("expected no route match for %s %s but one was found", method, path)
	}
}
