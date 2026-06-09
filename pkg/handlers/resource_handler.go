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
			validateSpec(&req, "Spec", "spec"),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			resource, err := presenters.ConvertResource(&req)
			if err != nil {
				return nil, errors.GeneralError("failed to convert resource: %v", err)
			}
			resource, svcErr := h.service.Create(r.Context(), h.descriptor.Kind, resource)
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
			id := mux.Vars(r)["id"]
			resource, err := h.service.Get(r.Context(), h.descriptor.Kind, id)
			if err != nil {
				return nil, err
			}
			presented := presenters.PresentResource(resource)

			return applyFieldFilter(r, presented)
		},
	}
	handleGet(w, r, cfg)
}

func (h *ResourceHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			listArgs := services.NewListArguments(r.URL.Query())
			resources, paging, err := h.service.List(r.Context(), h.descriptor.Kind, listArgs)
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
	var patch api.ResourcePatchRequest
	cfg := &handlerConfig{
		MarshalInto: &patch,
		Validate: []validate{
			validatePatchRequest(&patch),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			resource, err := h.service.Patch(r.Context(), h.descriptor.Kind, id, &patch)
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
			resource, svcErr := h.service.Delete(r.Context(), h.descriptor.Kind, id)
			if svcErr != nil {
				return nil, svcErr
			}
			return presenters.PresentResource(resource), nil
		},
	}
	handleSoftDelete(w, r, cfg)
}

// --- Nested resource (ByOwner) methods ---

func (h *ResourceHandler) CreateWithOwner(w http.ResponseWriter, r *http.Request) {
	var req openapi.ResourceCreateRequest
	cfg := &handlerConfig{
		MarshalInto: &req,
		Validate: []validate{
			validateKind(&req, "Kind", "kind", h.descriptor.Kind),
			validateSpec(&req, "Spec", "spec"),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			parentID := mux.Vars(r)["parent_id"]

			parent, svcErr := h.service.Get(ctx, h.descriptor.ParentKind, parentID)
			if svcErr != nil {
				return nil, svcErr
			}

			resource, err := presenters.ConvertResourceWithOwner(
				&req,
				parent.ID, parent.Kind, parent.Href,
			)
			if err != nil {
				return nil, errors.GeneralError("failed to convert resource: %v", err)
			}

			resource, svcErr = h.service.Create(ctx, h.descriptor.Kind, resource)
			if svcErr != nil {
				return nil, svcErr
			}
			return presenters.PresentResource(resource), nil
		},
	}
	handle(w, r, cfg, http.StatusCreated)
}

func (h *ResourceHandler) GetByOwner(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			vars := mux.Vars(r)
			parentID, id := vars["parent_id"], vars["id"]

			if _, err := h.service.Get(r.Context(), h.descriptor.ParentKind, parentID); err != nil {
				return nil, err
			}

			resource, err := h.service.GetByOwner(r.Context(), h.descriptor.Kind, id, parentID)
			if err != nil {
				return nil, err
			}
			presented := presenters.PresentResource(resource)

			return applyFieldFilter(r, presented)
		},
	}
	handleGet(w, r, cfg)
}

func (h *ResourceHandler) ListByOwner(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			parentID := mux.Vars(r)["parent_id"]

			if _, err := h.service.Get(r.Context(), h.descriptor.ParentKind, parentID); err != nil {
				return nil, err
			}

			listArgs := services.NewListArguments(r.URL.Query())
			resources, paging, err := h.service.ListByOwner(
				r.Context(), h.descriptor.Kind, parentID, listArgs,
			)
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

func (h *ResourceHandler) PatchByOwner(w http.ResponseWriter, r *http.Request) {
	var patch api.ResourcePatchRequest
	cfg := &handlerConfig{
		MarshalInto: &patch,
		Validate: []validate{
			validatePatchRequest(&patch),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			vars := mux.Vars(r)
			parentID, id := vars["parent_id"], vars["id"]

			if _, err := h.service.GetByOwner(r.Context(), h.descriptor.Kind, id, parentID); err != nil {
				return nil, err
			}

			resource, err := h.service.Patch(r.Context(), h.descriptor.Kind, id, &patch)
			if err != nil {
				return nil, err
			}
			return presenters.PresentResource(resource), nil
		},
	}
	handle(w, r, cfg, http.StatusOK)
}

func (h *ResourceHandler) DeleteByOwner(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			vars := mux.Vars(r)
			parentID, id := vars["parent_id"], vars["id"]

			if _, err := h.service.GetByOwner(r.Context(), h.descriptor.Kind, id, parentID); err != nil {
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
