package handlers

import (
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/presenters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

// ResourceHandler serves both flat and owner-nested routes for a single entity
// kind. Every method branches on whether "parent_id" is present via
// r.PathValue("parent_id") rather than dispatching statically per route. This is only correct because
// cmd/hyperfleet-api/server/routes_entities.go guarantees the invariant: a nested (ParentKind != "")
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
	validateFuncs := []validate{
		validateKind(&req, "Kind", "kind", h.descriptor.Kind),
		validateName(&req, "Name", "name", h.descriptor.NameMinLen, h.descriptor.NameMaxLen),
		validateSpec(&req, "Spec", "spec"),
		validateLabels(&req, "Labels"),
	}
	if err := decodeAndValidate(r, &req, validateFuncs); err != nil {
		handleError(r, w, err)
		return
	}

	ctx := r.Context()

	parentID := r.PathValue("parent_id")
	if parentID == "" && h.descriptor.ParentKind != "" {
		handleError(r, w, childCreateRejection(h.descriptor))
		return
	}

	var resource *api.Resource
	var convErr error
	if parentID != "" {
		parent, err := h.service.Get(ctx, h.descriptor.ParentKind, parentID)
		if err != nil {
			handleError(r, w, err)
			return
		}
		resource, convErr = presenters.ConvertResourceWithOwner(&req, parent.ID, parent.Kind, parent.Href)
	} else {
		resource, convErr = presenters.ConvertResource(&req)
	}
	if convErr != nil {
		handleError(r, w, errors.GeneralError("failed to convert resource: %v", convErr))
		return
	}

	refs := extractReferences(req.References)
	resource, err := h.service.Create(ctx, h.descriptor.Kind, resource, refs)
	if err != nil {
		handleError(r, w, err)
		return
	}

	writeJSONResponse(w, r, http.StatusCreated, presenters.PresentResource(resource))
}

func (h *ResourceHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	parentID, err := h.parentIDIfExists(r)
	if err != nil {
		handleError(r, w, err)
		return
	}

	var resource *api.Resource
	if parentID != "" {
		resource, err = h.service.GetByOwner(ctx, h.descriptor.Kind, id, parentID)
	} else {
		resource, err = h.service.Get(ctx, h.descriptor.Kind, id)
	}
	if err != nil {
		handleError(r, w, err)
		return
	}

	result, err := applyFieldFilter(r, presenters.PresentResource(resource))
	if err != nil {
		handleError(r, w, err)
		return
	}

	writeJSONResponse(w, r, http.StatusOK, result)
}

func (h *ResourceHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	parentID, err := h.parentIDIfExists(r)
	if err != nil {
		handleError(r, w, err)
		return
	}

	listArgs, err := parseListParams(r.URL.Query())
	if err != nil {
		handleError(r, w, err)
		return
	}

	var resources api.ResourceList
	var paging *api.PagingMeta
	if parentID != "" {
		resources, paging, err = h.service.ListByOwner(ctx, h.descriptor.Kind, parentID, listArgs)
	} else {
		resources, paging, err = h.service.List(ctx, h.descriptor.Kind, listArgs)
	}

	if err != nil {
		handleError(r, w, err)
		return
	}

	presented := presenters.PresentResourceList(resources, paging)
	if listArgs.Fields != nil {
		filtered, err := presenters.SliceFilter(listArgs.Fields, presented)
		if err != nil {
			handleError(r, w, err)
			return
		}
		writeJSONResponse(w, r, http.StatusOK, filtered)
		return
	}
	writeJSONResponse(w, r, http.StatusOK, presented)
}

func (h *ResourceHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var req openapi.ResourcePatchRequest
	validateFuncs := []validate{
		validatePatchRequest(&req),
		validateLabels(&req, "Labels"),
	}
	if err := decodeAndValidate(r, &req, validateFuncs, "strict"); err != nil {
		handleError(r, w, err)
		return
	}

	id := r.PathValue("id")
	ctx := r.Context()
	if err := h.checkOwnership(r, id); err != nil {
		handleError(r, w, err)
		return
	}

	patch := convertResourcePatch(&req)
	resource, err := h.service.Patch(ctx, h.descriptor.Kind, id, patch)
	if err != nil {
		handleError(r, w, err)
		return
	}

	writeJSONResponse(w, r, http.StatusOK, presenters.PresentResource(resource))
}

func (h *ResourceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()
	if err := h.checkOwnership(r, id); err != nil {
		handleError(r, w, err)
		return
	}

	resource, err := h.service.Delete(ctx, h.descriptor.Kind, id)
	if err != nil {
		handleError(r, w, err)
		return
	}

	writeJSONResponse(w, r, http.StatusAccepted, presenters.PresentResource(resource))
}

func (h *ResourceHandler) ForceDelete(w http.ResponseWriter, r *http.Request) {
	var req openapi.ForceDeleteRequest
	validateFuncs := []validate{
		validateNotEmpty(&req, "Reason", "reason"),
		validateMaxLength(&req, "Reason", "reason", maxReasonLength),
	}
	if err := decodeAndValidate(r, &req, validateFuncs); err != nil {
		handleError(r, w, err)
		return
	}

	id := r.PathValue("id")
	ctx := r.Context()
	if err := h.checkOwnership(r, id); err != nil {
		handleError(r, w, err)
		return
	}

	if err := h.service.ForceDelete(ctx, h.descriptor.Kind, id, req.Reason); err != nil {
		handleError(r, w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// checkOwnership verifies id belongs to parent_id, checking the parent first so
// a missing parent reports "not found" against the parent, not the child.
func (h *ResourceHandler) checkOwnership(r *http.Request, id string) *errors.ServiceError {
	parentID, err := h.parentIDIfExists(r)
	if err != nil {
		return err
	}
	if parentID != "" {
		if _, err := h.service.GetByOwner(r.Context(), h.descriptor.Kind, id, parentID); err != nil {
			return err
		}
	}
	return nil
}

// parentIDIfExists returns the parent_id if the parent exists, "" for flat
// routes, or a 404 if parent_id is present but the parent is missing.
func (h *ResourceHandler) parentIDIfExists(r *http.Request) (string, *errors.ServiceError) {
	parentID := r.PathValue("parent_id")
	if parentID == "" {
		return "", nil
	}
	_, err := h.service.Get(r.Context(), h.descriptor.ParentKind, parentID)
	if err != nil {
		return "", err
	}
	return parentID, nil
}
