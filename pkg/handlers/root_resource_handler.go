package handlers

import (
	"fmt"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/presenters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validators"
)

type RootResourceHandler struct {
	service              services.ResourceService
	adapterStatusService services.AdapterStatusService
	validator            *validators.SchemaValidator
}

func NewRootResourceHandler(
	service services.ResourceService,
	adapterStatusService services.AdapterStatusService,
	validator *validators.SchemaValidator,
) *RootResourceHandler {
	return &RootResourceHandler{
		service:              service,
		adapterStatusService: adapterStatusService,
		validator:            validator,
	}
}

func (h *RootResourceHandler) List(w http.ResponseWriter, r *http.Request) {
	listArgs, svcErr := parseListParams(r.URL.Query())
	if svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	if kind := r.URL.Query().Get("kind"); kind != "" {
		descriptor, ok := registry.Get(kind)
		if !ok {
			handleError(r, w, errors.Validation("Unknown entity kind: %s", kind))
			return
		}
		kindFilter := fmt.Sprintf("kind = '%s'", descriptor.Kind)
		if listArgs.Search == "" {
			listArgs.Search = kindFilter
		} else {
			listArgs.Search = "(" + listArgs.Search + ") AND " + kindFilter
		}
	}

	resources, paging, svcErr := h.service.ListAll(r.Context(), listArgs)
	if svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	presented := presenters.PresentResourceList(resources, paging)
	if listArgs.Fields != nil {
		filtered, svcErr := presenters.SliceFilter(listArgs.Fields, presented)
		if svcErr != nil {
			handleError(r, w, svcErr)
			return
		}
		writeJSONResponse(w, r, http.StatusOK, filtered)
		return
	}
	writeJSONResponse(w, r, http.StatusOK, presented)
}

func (h *RootResourceHandler) Get(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resource, svcErr := h.service.GetByID(r.Context(), id)
	if svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	result, svcErr := applyFieldFilter(r, presenters.PresentResource(resource))
	if svcErr != nil {
		handleError(r, w, svcErr)
		return
	}
	writeJSONResponse(w, r, http.StatusOK, result)
}

func (h *RootResourceHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req openapi.ResourceCreateRequest
	validateFuncs := []validate{
		validateSpec(&req, "Spec", "spec"),
		validateLabels(&req, "Labels"),
	}
	if svcErr := decodeAndValidate(r, &req, validateFuncs); svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	descriptor, ok := registry.Get(req.Kind)
	if !ok {
		handleError(r, w, errors.Validation("Unknown entity kind: %s", req.Kind))
		return
	}

	if descriptor.ParentKind != "" {
		handleError(r, w, childCreateRejection(descriptor))
		return
	}
	if svcErr := validateName(&req, "Name", "name", descriptor.NameMinLen, descriptor.NameMaxLen)(); svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	resource, convErr := presenters.ConvertResource(&req)
	if convErr != nil {
		handleError(r, w, errors.GeneralError("failed to convert resource: %v", convErr))
		return
	}

	refs := extractReferences(req.References)
	resource, svcErr := h.service.Create(r.Context(), descriptor.Kind, resource, refs)
	if svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	writeJSONResponse(w, r, http.StatusCreated, presenters.PresentResource(resource))
}

func (h *RootResourceHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var req openapi.ResourcePatchRequest
	validateFuncs := []validate{
		validatePatchRequest(&req),
		validateLabels(&req, "Labels"),
	}
	if svcErr := decodeAndValidate(r, &req, validateFuncs, "strict"); svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	id := r.PathValue("id")
	ctx := r.Context()
	resource, svcErr := h.service.GetByID(ctx, id)
	if svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	if req.Spec != nil && h.validator != nil {
		descriptor, ok := registry.Get(resource.Kind)
		if !ok {
			handleError(r, w, errors.GeneralError("Resource kind %q is no longer registered", resource.Kind))
			return
		}
		if validationErr := h.validator.Validate(descriptor.Plural, *req.Spec); validationErr != nil {
			specErr, ok := validationErr.(*errors.ServiceError)
			if !ok {
				specErr = errors.Validation("Spec validation failed: %v", validationErr)
			}
			handleError(r, w, specErr)
			return
		}
	}

	patch := convertResourcePatch(&req)
	resource, svcErr = h.service.Patch(ctx, resource.Kind, id, patch)
	if svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	writeJSONResponse(w, r, http.StatusOK, presenters.PresentResource(resource))
}

func (h *RootResourceHandler) ForceDelete(w http.ResponseWriter, r *http.Request) {
	var req openapi.ForceDeleteRequest
	validateFuncs := []validate{
		validateNotEmpty(&req, "Reason", "reason"),
		validateMaxLength(&req, "Reason", "reason", maxReasonLength),
	}
	if svcErr := decodeAndValidate(r, &req, validateFuncs); svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	id := r.PathValue("id")
	ctx := r.Context()
	resource, svcErr := h.service.GetByID(ctx, id)
	if svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	if svcErr := h.service.ForceDelete(ctx, resource.Kind, id, req.Reason); svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *RootResourceHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	ctx := r.Context()
	resource, svcErr := h.service.GetByID(ctx, id)
	if svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	resource, svcErr = h.service.Delete(ctx, resource.Kind, id)
	if svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	writeJSONResponse(w, r, http.StatusAccepted, presenters.PresentResource(resource))
}

// ListStatuses returns adapter statuses for a resource resolved by ID.
func (h *RootResourceHandler) ListStatuses(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	listArgs, svcErr := parseListParams(r.URL.Query())
	if svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	resource, svcErr := h.service.GetByID(ctx, id)
	if svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	statuses, total, svcErr := h.adapterStatusService.FindByResourcePaginated(
		ctx, resource.Kind, id, listArgs,
	)
	if svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	items := make([]openapi.AdapterStatus, 0, len(statuses))
	for _, as := range statuses {
		presented, presErr := presenters.PresentAdapterStatus(as)
		if presErr != nil {
			logger.WithError(ctx, presErr).Error("Failed to present adapter status")
			handleError(r, w, errors.GeneralError("Failed to present adapter status"))
			return
		}
		items = append(items, presented)
	}

	result := openapi.AdapterStatusList{
		Items: items,
		Page:  int32(listArgs.Page),
		Size:  int32(len(items)),
		Total: int32(total),
	}

	writeJSONResponse(w, r, http.StatusOK, result)
}

// CreateStatus creates or updates an adapter status for a resource resolved by ID.
func (h *RootResourceHandler) CreateStatus(w http.ResponseWriter, r *http.Request) {
	var req openapi.AdapterStatusCreateRequest
	validateFuncs := []validate{
		validateNotEmpty(&req, "Adapter", "adapter"),
		validateObservedGeneration(&req),
		validateConditions(&req, "Conditions"),
		validateObservedTimeRange(&req.ObservedTime),
	}
	if svcErr := decodeAndValidate(r, &req, validateFuncs); svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	ctx := r.Context()
	id := r.PathValue("id")
	resource, svcErr := h.service.GetByID(ctx, id)
	if svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	newStatus, convErr := presenters.ConvertAdapterStatus(resource.Kind, id, &req)
	if convErr != nil {
		logger.WithError(ctx, convErr).Error("Failed to convert adapter status")
		handleError(r, w, errors.GeneralError("Failed to convert adapter status"))
		return
	}

	adapterStatus, svcErr := h.service.ProcessAdapterStatus(ctx, resource.Kind, id, newStatus)
	if svcErr != nil {
		handleError(r, w, svcErr)
		return
	}

	if adapterStatus == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	status, presErr := presenters.PresentAdapterStatus(adapterStatus)
	if presErr != nil {
		logger.WithError(ctx, presErr).Error("Failed to present adapter status")
		handleError(r, w, errors.GeneralError("Failed to present adapter status"))
		return
	}
	writeJSONResponse(w, r, http.StatusCreated, status)
}
