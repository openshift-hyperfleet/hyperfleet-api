package entities

import (
	"fmt"
	"net/http"
	"sort"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validators"
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/resources"
)

func init() {
	server.RegisterRoutes("entities", func(apiV1Router *mux.Router, svc server.ServicesInterface) {
		envServices := svc.(*environments.Services)
		resourceService := resources.Service(envServices)

		schemaPath := environments.Environment().Config.Server.OpenAPISchemaPath
		var schemaValidator *validators.SchemaValidator
		if schemaPath != "" {
			var err error
			schemaValidator, err = validators.NewSchemaValidator(schemaPath)
			if err != nil {
				panic(fmt.Sprintf("failed to load schema validator from %s: %v", schemaPath, err))
			}
		}

		RegisterEntityRoutes(apiV1Router, resourceService, schemaValidator)
	})
}

// RegisterEntityRoutes creates handlers and registers routes for every entity
// descriptor in the registry. Called at startup after config-driven descriptors
// have been loaded via registry.LoadDescriptors.
//
// Top-level entities get routes at /{plural}. Child entities (ParentKind != "")
// get nested routes under /{parent_plural}/{parent_id}/{plural} plus flat
// read/update/delete access at /{plural} (POST rejected — needs parent context).
//
// The kind-agnostic /resources root endpoint is registered separately.
func RegisterEntityRoutes(
	apiV1Router *mux.Router,
	resourceService services.ResourceService,
	schemaValidator *validators.SchemaValidator,
) {
	registerPerEntityRoutes(apiV1Router, resourceService)
	registerRootResourceRoutes(apiV1Router, resourceService, schemaValidator)
}

func registerPerEntityRoutes(apiV1Router *mux.Router, resourceService services.ResourceService) {
	descriptors := registry.All()
	sort.Slice(descriptors, func(i, j int) bool {
		return descriptors[i].Kind < descriptors[j].Kind
	})

	for _, descriptor := range descriptors {
		if descriptor.Plural == "resources" {
			panic(fmt.Sprintf(
				"entity kind %q uses reserved plural %q which would shadow /resources root endpoint",
				descriptor.Kind, descriptor.Plural,
			))
		}
		h := handlers.NewResourceHandler(descriptor, resourceService)

		if descriptor.ParentKind != "" {
			parent := registry.MustGet(descriptor.ParentKind)
			registerResourceRoutes(apiV1Router, "/"+parent.Plural+"/{parent_id}/"+descriptor.Plural, h)
		}
		registerResourceRoutes(apiV1Router, "/"+descriptor.Plural, h)
	}
}

func registerRootResourceRoutes(
	apiV1Router *mux.Router,
	resourceService services.ResourceService,
	schemaValidator *validators.SchemaValidator,
) {
	rootHandler := handlers.NewRootResourceHandler(resourceService, schemaValidator)
	r := apiV1Router.PathPrefix("/resources").Subrouter()
	r.HandleFunc("", rootHandler.List).Methods(http.MethodGet)
	r.HandleFunc("", rootHandler.Create).Methods(http.MethodPost)
	r.HandleFunc("/{id}", rootHandler.Get).Methods(http.MethodGet)
	r.HandleFunc("/{id}", rootHandler.Patch).Methods(http.MethodPatch)
	r.HandleFunc("/{id}", rootHandler.Delete).Methods(http.MethodDelete)
	r.HandleFunc("/{id}/force-delete", rootHandler.ForceDelete).Methods(http.MethodPost)
	// TODO: HYPERFLEET-1154 — wire /{id}/statuses GET and PUT once ResourceStatusHandler exists
}

func registerResourceRoutes(apiV1Router *mux.Router, prefix string, h *handlers.ResourceHandler) {
	r := apiV1Router.PathPrefix(prefix).Subrouter()
	r.HandleFunc("", h.List).Methods(http.MethodGet)
	r.HandleFunc("", h.Create).Methods(http.MethodPost)
	r.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
	r.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
	r.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
	r.HandleFunc("/{id}/force-delete", h.ForceDelete).Methods(http.MethodPost)
}
