package handlers

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/presenters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

// ResourceHandler serves both flat and owner-nested routes for a single entity
// kind. Every method branches on whether "parent_id" is present in mux.Vars(r)
// rather than dispatching statically per route. This is only correct because
// plugins/entities/plugin.go guarantees the invariant: a nested (ParentKind != "")
// descriptor is registered exclusively under a {parent_id} subrouter, and a flat
// descriptor never is. If that registration is ever bypassed — e.g. a nested kind
// wired to a flat route — these branches take the wrong path silently (Create
// would skip setting owner references instead of erroring).
type ResourceHandler struct {
	service    services.ResourceService
	descriptor registry.EntityDescriptor
}

func NewResourceHandler(
	descriptor registry.EntityDescriptor,
	service services.ResourceService,
) *ResourceHandler {
	return &ResourceHandler{
		descriptor: descriptor,
		service:    service,
	}
}

func (h *ResourceHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req openapi.ResourceCreateRequest
	cfg := &handlerConfig{
		MarshalInto: &req,
		Validate: []validate{
			validateKind(&req, "Kind", "kind", h.descriptor.Kind),
			validateName(&req, "Name", "name", h.descriptor.NameMinLen, h.descriptor.NameMaxLen),
			validateSpec(&req, "Spec", "spec"),
			validateLabels(&req, "Labels"),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			var resource *api.Resource
			var err error
			if parentID, hasParent := mux.Vars(r)["parent_id"]; hasParent {
				parent, svcErr := h.service.Get(ctx, h.descriptor.ParentKind, parentID)
				if svcErr != nil {
					return nil, svcErr
				}
				resource, err = presenters.ConvertResourceWithOwner(&req, parent.ID, parent.Kind, parent.Href)
			} else if h.descriptor.ParentKind != "" {
				return nil, childCreateRejection(h.descriptor)
			} else {
				resource, err = presenters.ConvertResource(&req)
			}
			if err != nil {
				return nil, errors.GeneralError("failed to convert resource: %v", err)
			}

			refs := extractReferences(req.References)
			resource, svcErr := h.service.Create(ctx, h.descriptor.Kind, resource, refs)
			if svcErr != nil {
				return nil, svcErr
			}
			return presenters.PresentResource(resource), nil
		},
	}
	handle(w, r, cfg, http.StatusCreated)
}

func (h *ResourceHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			vars := mux.Vars(r)
			id := vars["id"]

			var resource *api.Resource
			var err *errors.ServiceError
			if parentID, hasParent := vars["parent_id"]; hasParent {
				if _, err = h.service.Get(ctx, h.descriptor.ParentKind, parentID); err != nil {
					return nil, err
				}
				resource, err = h.service.GetByOwner(ctx, h.descriptor.Kind, id, parentID)
			} else {
				resource, err = h.service.Get(ctx, h.descriptor.Kind, id)
			}
			if err != nil {
				return nil, err
			}

			return applyFieldFilter(r, presenters.PresentResource(resource))
		},
	}
	handleGet(w, r, cfg)
}

func (h *ResourceHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			listArgs, err := parseListParams(r.URL.Query())
			if err != nil {
				return nil, err
			}

			var resources api.ResourceList
			var paging *api.PagingMeta
			if parentID, hasParent := mux.Vars(r)["parent_id"]; hasParent {
				if _, svcErr := h.service.Get(ctx, h.descriptor.ParentKind, parentID); svcErr != nil {
					return nil, svcErr
				}
				resources, paging, err = h.service.ListByOwner(ctx, h.descriptor.Kind, parentID, listArgs)
			} else {
				resources, paging, err = h.service.List(ctx, h.descriptor.Kind, listArgs)
			}
			if err != nil {
				return nil, err
			}

			result := presenters.PresentResourceList(resources, paging)
			if listArgs.Fields != nil {
				return presenters.SliceFilter(listArgs.Fields, result)
			}
			return result, nil
		},
	}
	handleList(w, r, cfg)
}

func (h *ResourceHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var req openapi.ResourcePatchRequest
	cfg := &handlerConfig{
		MarshalInto:     &req,
		StrictUnmarshal: true,
		Validate: []validate{
			validatePatchRequest(&req),
			validateLabels(&req, "Labels"),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]

			if err := h.verifyOwnership(r, id); err != nil {
				return nil, err
			}

			patch := convertResourcePatch(&req)
			resource, err := h.service.Patch(r.Context(), h.descriptor.Kind, id, patch)
			if err != nil {
				return nil, err
			}
			return presenters.PresentResource(resource), nil
		},
	}
	handle(w, r, cfg, http.StatusOK)
}

func (h *ResourceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]

			if err := h.verifyOwnership(r, id); err != nil {
				return nil, err
			}

			resource, svcErr := h.service.Delete(r.Context(), h.descriptor.Kind, id)
			if svcErr != nil {
				return nil, svcErr
			}
			return presenters.PresentResource(resource), nil
		},
	}
	handleSoftDelete(w, r, cfg)
}

// verifyOwnership confirms id belongs to the parent named by parent_id in the
// request path. No-op for flat (non-nested) routes, where parent_id is absent.
func (h *ResourceHandler) verifyOwnership(r *http.Request, id string) *errors.ServiceError {
	if parentID, hasParent := mux.Vars(r)["parent_id"]; hasParent {
		if _, err := h.service.GetByOwner(r.Context(), h.descriptor.Kind, id, parentID); err != nil {
			return err
		}
	}
	return nil
}

func convertResourcePatch(req *openapi.ResourcePatchRequest) *api.ResourcePatch {
	patch := &api.ResourcePatch{}
	if req.Spec != nil {
		patch.Spec = *req.Spec
	}
	if req.Labels != nil {
		patch.Labels = *req.Labels
	}
	if req.References != nil {
		patch.References = *req.References
	}
	return patch
}

// extractReferences unwraps the optional references pointer from an API request.
// Returns nil when no references are supplied (nil pointer), or the map value.
func extractReferences(refs *api.ReferenceMap) api.ReferenceMap {
	if refs == nil {
		return nil
	}
	return *refs
}

func (h *ResourceHandler) ForceDelete(w http.ResponseWriter, r *http.Request) {
	var req openapi.ForceDeleteRequest
	cfg := &handlerConfig{
		MarshalInto: &req,
		Validate: []validate{
			validateNotEmpty(&req, "Reason", "reason"),
			validateMaxLength(&req, "Reason", "reason", maxReasonLength),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]

			if err := h.verifyOwnership(r, id); err != nil {
				return nil, err
			}

			if err := h.service.ForceDelete(r.Context(), h.descriptor.Kind, id, req.Reason); err != nil {
				return nil, err
			}
			return nil, nil
		},
	}
	handleForceDelete(w, r, cfg)
}

func childCreateRejection(descriptor registry.EntityDescriptor) *errors.ServiceError {
	parent := registry.MustGet(descriptor.ParentKind)
	svcErr := errors.Validation(
		"Cannot create %s here. Use POST /%s/{%s_id}/%s",
		descriptor.Kind, parent.Plural, parent.Kind, descriptor.Plural,
	)
	svcErr.HTTPCode = http.StatusUnprocessableEntity
	return svcErr
}
