package handlers

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

// RegisterEntityRoutes registers CRUD routes for all entity types in the registry.
// Top-level entities get /{plural} routes; child entities get nested /{parent-plural}/{parent_id}/{plural} only.
func RegisterEntityRoutes(apiV1Router *mux.Router, resourceService services.ResourceService) {
	for _, descriptor := range registry.All() {
		h := NewResourceHandler(descriptor, resourceService)

		if descriptor.ParentKind == "" {
			r := apiV1Router.PathPrefix("/" + descriptor.Plural).Subrouter()
			r.HandleFunc("", h.List).Methods(http.MethodGet)
			r.HandleFunc("", h.Create).Methods(http.MethodPost)
			r.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
			r.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
			r.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
			continue
		}

		parent := registry.MustGet(descriptor.ParentKind)
		nested := apiV1Router.PathPrefix(
			"/" + parent.Plural + "/{parent_id}/" + descriptor.Plural,
		).Subrouter()
		nested.HandleFunc("", h.ListByOwner).Methods(http.MethodGet)
		nested.HandleFunc("", h.CreateWithOwner).Methods(http.MethodPost)
		nested.HandleFunc("/{id}", h.GetByOwner).Methods(http.MethodGet)
		nested.HandleFunc("/{id}", h.PatchByOwner).Methods(http.MethodPatch)
		nested.HandleFunc("/{id}", h.DeleteByOwner).Methods(http.MethodDelete)
	}
}
