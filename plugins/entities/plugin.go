package entities

import (
	"fmt"
	"sort"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validators"
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/adapterStatus"
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/resources"
)

func init() {
	server.RegisterRoutes("entities", func(apiV1Router *server.Router, svc server.ServicesInterface) {
		envServices := svc.(*environments.Services)
		resourceService := resources.Service(envServices)
		adapterStatusService := adapterStatus.Service(envServices)

		schemaPath := environments.Environment().Config.Server.OpenAPISchemaPath
		var schemaValidator *validators.SchemaValidator
		if schemaPath != "" {
			var err error
			schemaValidator, err = validators.NewSchemaValidator(schemaPath)
			if err != nil {
				panic(fmt.Sprintf("failed to load schema validator from %s: %v", schemaPath, err))
			}
		}

		RegisterEntityRoutes(apiV1Router, resourceService, adapterStatusService, schemaValidator)
	})
}

// RegisterEntityRoutes creates handlers and registers routes for every entity
// descriptor in the registry. Called at startup after config-driven descriptors
// have been loaded via registry.LoadDescriptors.
//
// Top-level entities get routes at /{plural}. Child entities (ParentKind != "")
// get nested routes under /{parent_plural}/{parent_id}/{plural} plus flat
// read/update/delete access at /{plural} (POST rejected — needs parent context).
// All entities get /{id}/statuses sub-routes for adapter status reporting.
//
// The kind-agnostic /resources root endpoint is registered separately.
func RegisterEntityRoutes(
	apiV1Router *server.Router,
	resourceService services.ResourceService,
	adapterStatusService services.AdapterStatusService,
	schemaValidator *validators.SchemaValidator,
) {
	registerPerEntityRoutes(apiV1Router, resourceService, adapterStatusService)
	registerRootResourceRoutes(apiV1Router, resourceService, adapterStatusService, schemaValidator)
}

func registerPerEntityRoutes(
	apiV1Router *server.Router,
	resourceService services.ResourceService,
	adapterStatusService services.AdapterStatusService,
) {
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
		sh := handlers.NewResourceStatusHandler(descriptor, resourceService, adapterStatusService)

		if descriptor.ParentKind != "" {
			parent := registry.MustGet(descriptor.ParentKind)
			registerResourceRoutes(apiV1Router, "/"+parent.Plural+"/{parent_id}/"+descriptor.Plural, h, sh)
		}
		registerResourceRoutes(apiV1Router, "/"+descriptor.Plural, h, sh)
	}
}

func registerRootResourceRoutes(
	apiV1Router *server.Router,
	resourceService services.ResourceService,
	adapterStatusService services.AdapterStatusService,
	schemaValidator *validators.SchemaValidator,
) {
	rootHandler := handlers.NewRootResourceHandler(resourceService, adapterStatusService, schemaValidator)
	prefix := server.APIV1BasePath + "/resources"
	apiV1Router.HandleFunc("GET "+prefix, rootHandler.List)
	apiV1Router.HandleFunc("POST "+prefix, rootHandler.Create)
	apiV1Router.HandleFunc("GET "+prefix+"/{id}", rootHandler.Get)
	apiV1Router.HandleFunc("PATCH "+prefix+"/{id}", rootHandler.Patch)
	apiV1Router.HandleFunc("DELETE "+prefix+"/{id}", rootHandler.Delete)
	apiV1Router.HandleFunc("POST "+prefix+"/{id}/force-delete", rootHandler.ForceDelete)
	apiV1Router.HandleFunc("GET "+prefix+"/{id}/statuses", rootHandler.ListStatuses)
	apiV1Router.HandleFunc("PUT "+prefix+"/{id}/statuses", rootHandler.CreateStatus)
}

func registerResourceRoutes(
	apiV1Router *server.Router, pathSuffix string,
	h *handlers.ResourceHandler, sh *handlers.ResourceStatusHandler,
) {
	prefix := server.APIV1BasePath + pathSuffix
	apiV1Router.HandleFunc("GET "+prefix, h.List)
	apiV1Router.HandleFunc("GET "+prefix+"/statuses", h.ListStatuses)
	apiV1Router.HandleFunc("POST "+prefix, h.Create)
	apiV1Router.HandleFunc("GET "+prefix+"/{id}", h.Get)
	apiV1Router.HandleFunc("PATCH "+prefix+"/{id}", h.Patch)
	apiV1Router.HandleFunc("DELETE "+prefix+"/{id}", h.Delete)
	apiV1Router.HandleFunc("POST "+prefix+"/{id}/force-delete", h.ForceDelete)
	apiV1Router.HandleFunc("GET "+prefix+"/{id}/statuses", sh.List)
	apiV1Router.HandleFunc("PUT "+prefix+"/{id}/statuses", sh.Create)
}
