package handlers

import (
	"fmt"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/presenters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validators"
)

type RootResourceHandler struct {
	service   services.ResourceService
	validator *validators.SchemaValidator
}

func NewRootResourceHandler(
	service services.ResourceService,
	validator *validators.SchemaValidator,
) *RootResourceHandler {
	return &RootResourceHandler{service: service, validator: validator}
}

func (h *RootResourceHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			listArgs, err := services.NewListArguments(r.URL.Query())
			if err != nil {
				return nil, err
			}

			if kind := r.URL.Query().Get("kind"); kind != "" {
				descriptor, ok := registry.Get(kind)
				if !ok {
					return nil, errors.Validation("Unknown entity kind: %s", kind)
				}
				kindFilter := fmt.Sprintf("kind = '%s'", descriptor.Kind)
				if listArgs.Search == "" {
					listArgs.Search = kindFilter
				} else {
					listArgs.Search = "(" + listArgs.Search + ") AND " + kindFilter
				}
			}

			resources, paging, err := h.service.ListAll(r.Context(), listArgs)
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

func (h *RootResourceHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			resource, err := h.service.GetByID(r.Context(), id)
			if err != nil {
				return nil, err
			}
			presented := presenters.PresentResource(resource)
			return applyFieldFilter(r, presented)
		},
	}
	handleGet(w, r, cfg)
}

func (h *RootResourceHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req openapi.ResourceCreateRequest
	cfg := &handlerConfig{
		MarshalInto: &req,
		Validate: []validate{
			validateSpec(&req, "Spec", "spec"),
			validateLabels(&req, "Labels"),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			descriptor, ok := registry.Get(req.Kind)
			if !ok {
				return nil, errors.Validation("Unknown entity kind: %s", req.Kind)
			}
			if descriptor.ParentKind != "" {
				return nil, childCreateRejection(descriptor)
			}

			resource, convErr := presenters.ConvertResource(&req)
			if convErr != nil {
				return nil, errors.GeneralError("failed to convert resource: %v", convErr)
			}
			resource, svcErr := h.service.Create(r.Context(), descriptor.Kind, resource)
			if svcErr != nil {
				return nil, svcErr
			}
			return presenters.PresentResource(resource), nil
		},
	}
	handle(w, r, cfg, http.StatusCreated)
}

func (h *RootResourceHandler) Patch(w http.ResponseWriter, r *http.Request) {
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
			resource, err := h.service.GetByID(r.Context(), id)
			if err != nil {
				return nil, err
			}

			if req.Spec != nil && h.validator != nil {
				descriptor, ok := registry.Get(resource.Kind)
				if !ok {
					return nil, errors.GeneralError("Resource kind %q is no longer registered", resource.Kind)
				}
				if validationErr := h.validator.Validate(descriptor.Plural, *req.Spec); validationErr != nil {
					if svcErr, ok := validationErr.(*errors.ServiceError); ok {
						return nil, svcErr
					}
					return nil, errors.Validation("Spec validation failed: %v", validationErr)
				}
			}

			patch := convertResourcePatch(&req)
			resource, err = h.service.Patch(r.Context(), resource.Kind, id, patch)
			if err != nil {
				return nil, err
			}
			return presenters.PresentResource(resource), nil
		},
	}
	handle(w, r, cfg, http.StatusOK)
}

func (h *RootResourceHandler) ForceDelete(w http.ResponseWriter, r *http.Request) {
	var req openapi.ForceDeleteRequest
	cfg := &handlerConfig{
		MarshalInto: &req,
		Validate: []validate{
			validateNotEmpty(&req, "Reason", "reason"),
			validateMaxLength(&req, "Reason", "reason", maxReasonLength),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			resource, err := h.service.GetByID(r.Context(), id)
			if err != nil {
				return nil, err
			}
			if err := h.service.ForceDelete(r.Context(), resource.Kind, id, req.Reason); err != nil {
				return nil, err
			}
			return nil, nil
		},
	}
	handleForceDelete(w, r, cfg)
}

func (h *RootResourceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			resource, err := h.service.GetByID(r.Context(), id)
			if err != nil {
				return nil, err
			}
			resource, svcErr := h.service.Delete(r.Context(), resource.Kind, id)
			if svcErr != nil {
				return nil, svcErr
			}
			return presenters.PresentResource(resource), nil
		},
	}
	handleSoftDelete(w, r, cfg)
}
