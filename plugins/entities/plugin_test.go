package entities

import (
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
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
	RegisterEntityRoutes(apiV1, nil, nil, nil)

	id := uuid.NewString()
	assertRouteMatches(t, router, "GET", "/api/hyperfleet/v1/channels")
	assertRouteMatches(t, router, "POST", "/api/hyperfleet/v1/channels")
	assertRouteMatches(t, router, "GET", "/api/hyperfleet/v1/channels/"+id)
	assertRouteMatches(t, router, "PATCH", "/api/hyperfleet/v1/channels/"+id)
	assertRouteMatches(t, router, "DELETE", "/api/hyperfleet/v1/channels/"+id)
	assertRouteMatches(t, router, "GET", "/api/hyperfleet/v1/channels/"+id+"/statuses")
	assertRouteMatches(t, router, "PUT", "/api/hyperfleet/v1/channels/"+id+"/statuses")
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
	RegisterEntityRoutes(apiV1, nil, nil, nil)

	parentID := uuid.NewString()
	childID := uuid.NewString()
	nested := "/api/hyperfleet/v1/channels/" + parentID + "/versions"

	assertRouteMatches(t, router, "GET", nested)
	assertRouteMatches(t, router, "POST", nested)
	assertRouteMatches(t, router, "GET", nested+"/"+childID)
	assertRouteMatches(t, router, "PATCH", nested+"/"+childID)
	assertRouteMatches(t, router, "DELETE", nested+"/"+childID)
	assertRouteMatches(t, router, "GET", nested+"/"+childID+"/statuses")
	assertRouteMatches(t, router, "PUT", nested+"/"+childID+"/statuses")

	flat := "/api/hyperfleet/v1/versions"
	assertRouteMatches(t, router, "GET", flat)
	assertRouteMatches(t, router, "POST", flat)
	assertRouteMatches(t, router, "GET", flat+"/"+childID)
	assertRouteMatches(t, router, "PATCH", flat+"/"+childID)
	assertRouteMatches(t, router, "DELETE", flat+"/"+childID)
	assertRouteMatches(t, router, "GET", flat+"/"+childID+"/statuses")
	assertRouteMatches(t, router, "PUT", flat+"/"+childID+"/statuses")
}

func TestRegisterEntityRoutes_UnresolvableParentKind_Panics(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(registry.EntityDescriptor{
		Kind:       "Version",
		Plural:     "versions",
		ParentKind: "NonExistent",
	})

	router := mux.NewRouter()
	apiV1 := router.PathPrefix("/api/hyperfleet/v1").Subrouter()

	Expect(func() {
		RegisterEntityRoutes(apiV1, nil, nil, nil)
	}).To(PanicWith(ContainSubstring("not registered")))
}

func TestRegisterEntityRoutes_EmptyRegistry(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()

	router := mux.NewRouter()
	apiV1 := router.PathPrefix("/api/hyperfleet/v1").Subrouter()

	Expect(func() {
		RegisterEntityRoutes(apiV1, nil, nil, nil)
	}).ToNot(Panic())
}

func assertRouteMatches(t *testing.T, router *mux.Router, method, path string) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	match := mux.RouteMatch{}
	Expect(router.Match(req, &match)).To(BeTrue(), "expected route to match: %s %s", method, path)
}
