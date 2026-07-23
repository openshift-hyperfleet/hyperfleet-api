package handlers

import (
	"context"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/presenters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

// ResourceStatusHandler handles GET/PUT on /{plural}/{id}/statuses for
// config-driven generic resources. Each method branches on whether "parent_id"
// is present in mux.Vars(r) to handle both flat and nested routes — matching
// the pattern established by ResourceHandler in HYPERFLEET-1157.
type ResourceStatusHandler struct {
	resourceService      services.ResourceService
	adapterStatusService services.AdapterStatusService
	descriptor           registry.EntityDescriptor
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
// Verifies ownership when parent_id is present in the route.
func (h *ResourceStatusHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]
			listArgs, err := parseListParams(r.URL.Query())
			if err != nil {
				return nil, err
			}

			if svcErr := h.verifyResource(r, id); svcErr != nil {
				return nil, svcErr
			}

			return h.listStatuses(ctx, id, listArgs)
		},
		ErrorHandler: handleError,
	}

	handleList(w, r, cfg)
}

// Create creates or updates an adapter status for a resource.
// Verifies ownership when parent_id is present in the route.
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

			if svcErr := h.verifyResource(r, id); svcErr != nil {
				return nil, svcErr
			}

			return h.processStatus(ctx, id, &req)
		},
		ErrorHandler: handleError,
	}

	handleCreateWithNoContent(w, r, cfg)
}

// verifyResource confirms the resource exists. For nested routes (parent_id
// present), also verifies the resource belongs to the parent. No-op ownership
// check for flat routes.
func (h *ResourceStatusHandler) verifyResource(r *http.Request, id string) *errors.ServiceError {
	ctx := r.Context()
	if parentID, hasParent := mux.Vars(r)["parent_id"]; hasParent {
		if _, err := h.resourceService.GetByOwner(ctx, h.descriptor.Kind, id, parentID); err != nil {
			return err
		}
	} else {
		if _, err := h.resourceService.Get(ctx, h.descriptor.Kind, id); err != nil {
			return err
		}
	}
	return nil
}

// listStatuses fetches paginated adapter statuses and presents them as an OpenAPI response.
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
			logger.WithError(ctx, presErr).Error("Failed to present adapter status")
			return nil, errors.GeneralError("Failed to present adapter status")
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
		logger.WithError(ctx, convErr).Error("Failed to convert adapter status")
		return nil, errors.GeneralError("Failed to convert adapter status")
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
		logger.WithError(ctx, presErr).Error("Failed to present adapter status")
		return nil, errors.GeneralError("Failed to present adapter status")
	}
	return &status, nil
}
