package handlers

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/presenters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

// ResourceStatusHandler handles GET/PUT on /{plural}/{id}/statuses for
// config-driven generic resources. Follows the same pattern as
// ClusterStatusHandler but uses descriptor.Kind as the resource type
// and delegates to ResourceService.ProcessAdapterStatus.
type ResourceStatusHandler struct {
	descriptor           registry.EntityDescriptor
	resourceService      services.ResourceService
	adapterStatusService services.AdapterStatusService
}

func NewResourceStatusHandler(
	descriptor registry.EntityDescriptor,
	resourceService services.ResourceService,
	adapterStatusService services.AdapterStatusService,
) *ResourceStatusHandler {
	return &ResourceStatusHandler{
		descriptor:           descriptor,
		resourceService:      resourceService,
		adapterStatusService: adapterStatusService,
	}
}

// List returns all adapter statuses for a resource with pagination.
func (h *ResourceStatusHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]
			listArgs, err := services.NewListArguments(r.URL.Query())
			if err != nil {
				return nil, err
			}

			if _, err = h.resourceService.Get(ctx, h.descriptor.Kind, id); err != nil {
				return nil, err
			}

			return h.listStatuses(ctx, id, listArgs)
		},
		ErrorHandler: handleError,
	}

	handleList(w, r, cfg)
}

// Create creates or updates an adapter status for a resource.
func (h *ResourceStatusHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req openapi.AdapterStatusCreateRequest

	cfg := &handlerConfig{
		MarshalInto: &req,
		Validate: []validate{
			validateNotEmpty(&req, "Adapter", "adapter"),
			validateObservedGeneration(&req),
			validateConditions(&req, "Conditions"),
			validateObservedTimeRange(&req.ObservedTime),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]

			if _, err := h.resourceService.Get(ctx, h.descriptor.Kind, id); err != nil {
				return nil, err
			}

			return h.processStatus(ctx, id, &req)
		},
		ErrorHandler: handleError,
	}

	handleCreateWithNoContent(w, r, cfg)
}

// ListByOwner returns adapter statuses for a child resource, validating ownership.
func (h *ResourceStatusHandler) ListByOwner(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			vars := mux.Vars(r)
			parentID, id := vars["parent_id"], vars["id"]
			listArgs, err := services.NewListArguments(r.URL.Query())
			if err != nil {
				return nil, err
			}

			if _, err = h.resourceService.GetByOwner(ctx, h.descriptor.Kind, id, parentID); err != nil {
				return nil, err
			}

			return h.listStatuses(ctx, id, listArgs)
		},
		ErrorHandler: handleError,
	}

	handleList(w, r, cfg)
}

// CreateByOwner creates or updates an adapter status for a child resource, validating ownership.
func (h *ResourceStatusHandler) CreateByOwner(w http.ResponseWriter, r *http.Request) {
	var req openapi.AdapterStatusCreateRequest

	cfg := &handlerConfig{
		MarshalInto: &req,
		Validate: []validate{
			validateNotEmpty(&req, "Adapter", "adapter"),
			validateObservedGeneration(&req),
			validateConditions(&req, "Conditions"),
			validateObservedTimeRange(&req.ObservedTime),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			vars := mux.Vars(r)
			parentID, id := vars["parent_id"], vars["id"]

			if _, err := h.resourceService.GetByOwner(ctx, h.descriptor.Kind, id, parentID); err != nil {
				return nil, err
			}

			return h.processStatus(ctx, id, &req)
		},
		ErrorHandler: handleError,
	}

	handleCreateWithNoContent(w, r, cfg)
}

// listStatuses fetches paginated adapter statuses and presents them as an OpenAPI response.
// Shared by List and ListByOwner — the caller handles resource existence/ownership checks.
func (h *ResourceStatusHandler) listStatuses(
	ctx context.Context, resourceID string, listArgs *services.ListArguments,
) (interface{}, *errors.ServiceError) {
	adapterStatuses, total, err := h.adapterStatusService.FindByResourcePaginated(
		ctx, h.descriptor.Kind, resourceID, listArgs,
	)
	if err != nil {
		return nil, err
	}

	items := make([]openapi.AdapterStatus, 0, len(adapterStatuses))
	for _, as := range adapterStatuses {
		presented, presErr := presenters.PresentAdapterStatus(as)
		if presErr != nil {
			return nil, errors.GeneralError("Failed to present adapter status: %v", presErr)
		}
		items = append(items, presented)
	}

	return openapi.AdapterStatusList{
		Items: items,
		Page:  int32(listArgs.Page),
		Size:  int32(len(items)),
		Total: int32(total),
	}, nil
}

// processStatus converts the request, delegates to ProcessAdapterStatus, and presents the result.
// Returns (nil, nil) when the status was silently discarded (triggers 204 No Content).
func (h *ResourceStatusHandler) processStatus(
	ctx context.Context, resourceID string, req *openapi.AdapterStatusCreateRequest,
) (interface{}, *errors.ServiceError) {
	newStatus, convErr := presenters.ConvertAdapterStatus(h.descriptor.Kind, resourceID, req)
	if convErr != nil {
		return nil, errors.GeneralError("Failed to convert adapter status: %v", convErr)
	}

	adapterStatus, err := h.resourceService.ProcessAdapterStatus(
		ctx, h.descriptor.Kind, resourceID, newStatus,
	)
	if err != nil {
		return nil, err
	}

	if adapterStatus == nil {
		return nil, nil
	}

	status, presErr := presenters.PresentAdapterStatus(adapterStatus)
	if presErr != nil {
		return nil, errors.GeneralError("Failed to present adapter status: %v", presErr)
	}
	return &status, nil
}
