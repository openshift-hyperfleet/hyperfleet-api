package entities

import (
	"net/http"
	"sort"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/resources"
)

func init() {
	server.RegisterRoutes("entities", func(apiV1Router *mux.Router, svc server.ServicesInterface) {
		envServices := svc.(*environments.Services)
		resourceService := resources.Service(envServices)

		RegisterEntityRoutes(apiV1Router, resourceService)
	})
}

// RegisterEntityRoutes creates handlers and registers routes for every entity
// descriptor in the registry. Called at startup after config-driven descriptors
// have been loaded via registry.LoadDescriptors.
//
// Top-level entities get routes at /{plural}. Child entities (ParentKind != "")
// get nested routes only, under /{parent_plural}/{parent_id}/{plural}.
func RegisterEntityRoutes(apiV1Router *mux.Router, resourceService services.ResourceService) {
	descriptors := registry.All()
	sort.Slice(descriptors, func(i, j int) bool {
		return descriptors[i].Kind < descriptors[j].Kind
	})

	for _, descriptor := range descriptors {
		h := handlers.NewResourceHandler(descriptor, resourceService)

		if descriptor.ParentKind == "" {
			r := apiV1Router.PathPrefix("/" + descriptor.Plural).Subrouter()
			r.HandleFunc("", h.List).Methods(http.MethodGet)
			r.HandleFunc("", h.Create).Methods(http.MethodPost)
			r.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
			r.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
			r.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
		} else {
			parent := registry.MustGet(descriptor.ParentKind)
			pr := apiV1Router.PathPrefix("/" + parent.Plural + "/{parent_id}/" + descriptor.Plural).Subrouter()
			pr.HandleFunc("", h.ListByOwner).Methods(http.MethodGet)
			pr.HandleFunc("", h.CreateWithOwner).Methods(http.MethodPost)
			pr.HandleFunc("/{id}", h.GetByOwner).Methods(http.MethodGet)
			pr.HandleFunc("/{id}", h.PatchByOwner).Methods(http.MethodPatch)
			pr.HandleFunc("/{id}", h.DeleteByOwner).Methods(http.MethodDelete)
		}
	}
}
