package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

type nodePoolStatusHandler struct {
	adapterStatusService services.AdapterStatusService
	nodePoolService      services.NodePoolService
}

func NewNodePoolStatusHandler(adapterStatusService services.AdapterStatusService, nodePoolService services.NodePoolService) *nodePoolStatusHandler {
	return &nodePoolStatusHandler{
		adapterStatusService: adapterStatusService,
		nodePoolService:      nodePoolService,
	}
}

// List returns all adapter statuses for a nodepool with pagination
func (h nodePoolStatusHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			nodePoolID := mux.Vars(r)["nodepool_id"]
			listArgs := services.NewListArguments(r.URL.Query())

			// Fetch adapter statuses with pagination
			adapterStatuses, total, err := h.adapterStatusService.FindByResourcePaginated(ctx, "NodePool", nodePoolID, listArgs)
			if err != nil {
				return nil, err
			}

			// Convert to OpenAPI models
			items := make([]openapi.AdapterStatus, 0, len(adapterStatuses))
			for _, as := range adapterStatuses {
				items = append(items, *as.ToOpenAPI())
			}

			// Return list response with pagination metadata
			response := openapi.AdapterStatusList{
				Kind:  "AdapterStatusList",
				Items: items,
				Page:  int32(listArgs.Page),
				Size:  int32(len(items)),
				Total: int32(total),
			}

			return response, nil
		},
	}

	handleList(w, r, cfg)
}

// Create creates or updates an adapter status for a nodepool
func (h nodePoolStatusHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req openapi.AdapterStatusCreateRequest

	cfg := &handlerConfig{
		&req,
		[]validate{
			validateNotEmpty(&req, "Adapter", "adapter"),
		},
		func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			nodePoolID := mux.Vars(r)["nodepool_id"]

			// Verify nodepool exists
			_, err := h.nodePoolService.Get(ctx, nodePoolID)
			if err != nil {
				return nil, err
			}

			// Check if adapter status already exists
			existing, _ := h.adapterStatusService.FindByResourceAndAdapter(ctx, "NodePool", nodePoolID, req.Adapter)

			var adapterStatus *api.AdapterStatus
			if existing != nil {
				// Update existing
				existing.ObservedGeneration = req.ObservedGeneration
				conditionsJSON, _ := json.Marshal(req.Conditions)
				existing.Conditions = conditionsJSON
				if req.Data != nil {
					dataJSON, _ := json.Marshal(req.Data)
					existing.Data = dataJSON
				}
				adapterStatus, err = h.adapterStatusService.Replace(ctx, existing)
			} else {
				// Create new
				newStatus := api.AdapterStatusFromOpenAPICreate("NodePool", nodePoolID, &req)
				adapterStatus, err = h.adapterStatusService.Create(ctx, newStatus)
			}

			if err != nil {
				return nil, err
			}

			// Trigger status aggregation
			_, aggregateErr := h.nodePoolService.UpdateNodePoolStatusFromAdapters(ctx, nodePoolID)
			if aggregateErr != nil {
				// Log error but don't fail the request
				// The status will be computed on next update
			}

			return adapterStatus.ToOpenAPI(), nil
		},
		handleError,
	}

	handle(w, r, cfg, http.StatusCreated)
}
