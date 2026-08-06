package server

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validators"
)

func NewEntityRouteRegistrar(
	resourceService services.ResourceService,
	adapterStatusService services.AdapterStatusService,
	schemaValidator *validators.SchemaValidator,
) RouteRegistrar {
	return RouteRegistrar{
		Name: "entities",
		Register: func(router *Router) error {
			return RegisterEntityRoutes(router, resourceService, adapterStatusService, schemaValidator)
		},
	}
}

// RegisterEntityRoutes creates handlers and registers routes for every entity
// descriptor in the registry. Called at startup after config-driven descriptors
// have been loaded via registry.LoadDescriptors.
//
// Top-level entities get routes at /{plural}. Child entities (ParentKind != "")
// get nested routes under /{parent_plural}/{parent_id}/{plural} plus flat
// read/update/delete access at /{plural} (POST rejected - needs parent context).
// All entities get /{id}/statuses sub-routes for adapter status reporting.
//
// The kind-agnostic /resources root endpoint is registered separately.
func RegisterEntityRoutes(
	router *Router,
	resourceService services.ResourceService,
	adapterStatusService services.AdapterStatusService,
	schemaValidator *validators.SchemaValidator,
) error {
	if err := registerPerEntityRoutes(router, resourceService, adapterStatusService); err != nil {
		return fmt.Errorf("register entity routes: %w", err)
	}
	registerRootResourceRoutes(router, resourceService, adapterStatusService, schemaValidator)
	return nil
}

func registerPerEntityRoutes(
	router *Router,
	resourceService services.ResourceService,
	adapterStatusService services.AdapterStatusService,
) error {
	descriptors := registry.All()
	slices.SortFunc(descriptors, func(a, b registry.EntityDescriptor) int {
		return cmp.Compare(a.Kind, b.Kind)
	})

	for _, descriptor := range descriptors {
		if descriptor.Plural == "resources" {
			return fmt.Errorf(
				"entity kind %q uses reserved plural %q which would shadow /resources root endpoint",
				descriptor.Kind, descriptor.Plural,
			)
		}
		h := handlers.NewResourceHandler(descriptor, resourceService)
		sh := handlers.NewResourceStatusHandler(descriptor, resourceService, adapterStatusService)

		if descriptor.ParentKind != "" {
			parent := registry.MustGet(descriptor.ParentKind)
			registerEntityResourceRoutes(router, "/"+parent.Plural+"/{parent_id}/"+descriptor.Plural, h, sh)
		}
		registerEntityResourceRoutes(router, "/"+descriptor.Plural, h, sh)
	}
	return nil
}

func registerRootResourceRoutes(
	router *Router,
	resourceService services.ResourceService,
	adapterStatusService services.AdapterStatusService,
	schemaValidator *validators.SchemaValidator,
) {
	rootHandler := handlers.NewRootResourceHandler(resourceService, adapterStatusService, schemaValidator)
	prefix := "/resources"
	router.HandleFunc("GET "+prefix, rootHandler.List)
	router.HandleFunc("POST "+prefix, rootHandler.Create)
	router.HandleFunc("GET "+prefix+"/{id}", rootHandler.Get)
	router.HandleFunc("PATCH "+prefix+"/{id}", rootHandler.Patch)
	router.HandleFunc("DELETE "+prefix+"/{id}", rootHandler.Delete)
	router.HandleFunc("POST "+prefix+"/{id}/force-delete", rootHandler.ForceDelete)
	router.HandleFunc("GET "+prefix+"/{id}/statuses", rootHandler.ListStatuses)
	router.HandleFunc("PUT "+prefix+"/{id}/statuses", rootHandler.CreateStatus)
}

func registerEntityResourceRoutes(
	router *Router, pathSuffix string,
	h *handlers.ResourceHandler, sh *handlers.ResourceStatusHandler,
) {
	prefix := pathSuffix
	router.HandleFunc("GET "+prefix, h.List)
	router.HandleFunc("POST "+prefix, h.Create)
	router.HandleFunc("GET "+prefix+"/{id}", h.Get)
	router.HandleFunc("PATCH "+prefix+"/{id}", h.Patch)
	router.HandleFunc("DELETE "+prefix+"/{id}", h.Delete)
	router.HandleFunc("POST "+prefix+"/{id}/force-delete", h.ForceDelete)
	router.HandleFunc("GET "+prefix+"/{id}/statuses", sh.List)
	router.HandleFunc("PUT "+prefix+"/{id}/statuses", sh.Create)
}
