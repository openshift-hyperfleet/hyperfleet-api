package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

func TestRegisterEntityRoutes_TopLevelOnly(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(channelDescriptor)

	ctrl := gomock.NewController(t)
	router := mux.NewRouter()
	RegisterEntityRoutes(router, services.NewMockResourceService(ctrl))

	cases := []struct {
		method string
		path   string
		match  bool
	}{
		{http.MethodGet, "/channels", true},
		{http.MethodPost, "/channels", true},
		{http.MethodGet, "/channels/ch-1", true},
		{http.MethodPatch, "/channels/ch-1", true},
		{http.MethodDelete, "/channels/ch-1", true},
		{http.MethodGet, "/channels/ch-1/versions", false},
	}

	for _, tc := range cases {
		Expect(routeMatches(router, tc.method, tc.path)).To(
			Equal(tc.match),
			"method=%s path=%s",
			tc.method,
			tc.path,
		)
	}
}

func TestRegisterEntityRoutes_ChildNestedOnly(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(channelDescriptor)
	registry.Register(versionDescriptor)

	ctrl := gomock.NewController(t)
	router := mux.NewRouter()
	RegisterEntityRoutes(router, services.NewMockResourceService(ctrl))

	nestedCases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/channels/parent-id/versions"},
		{http.MethodPost, "/channels/parent-id/versions"},
		{http.MethodGet, "/channels/parent-id/versions/v-1"},
		{http.MethodPatch, "/channels/parent-id/versions/v-1"},
		{http.MethodDelete, "/channels/parent-id/versions/v-1"},
	}
	for _, tc := range nestedCases {
		Expect(routeMatches(router, tc.method, tc.path)).To(
			BeTrue(),
			"expected nested route method=%s path=%s",
			tc.method,
			tc.path,
		)
	}

	topLevelCases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/versions"},
		{http.MethodPost, "/versions"},
		{http.MethodGet, "/versions/v-1"},
		{http.MethodPatch, "/versions/v-1"},
		{http.MethodDelete, "/versions/v-1"},
	}
	for _, tc := range topLevelCases {
		Expect(routeMatches(router, tc.method, tc.path)).To(
			BeFalse(),
			"child entity must not expose top-level route method=%s path=%s",
			tc.method,
			tc.path,
		)
	}
}

func TestRegisterEntityRoutes_MissingParentPanics(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	registry.Register(versionDescriptor)

	ctrl := gomock.NewController(t)
	router := mux.NewRouter()

	Expect(func() {
		RegisterEntityRoutes(router, services.NewMockResourceService(ctrl))
	}).To(PanicWith(ContainSubstring(`entity kind "Channel" not registered`)))
}

func routeMatches(router *mux.Router, method, path string) bool {
	req := httptest.NewRequest(method, path, nil)
	var match mux.RouteMatch
	return router.Match(req, &match)
}
