package handlers

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

type clusterStatusHandler struct {
	adapterStatusService services.AdapterStatusService
	clusterService       services.ClusterService
}

func NewClusterStatusHandler(adapterStatusService services.AdapterStatusService, clusterService services.ClusterService) *clusterStatusHandler {
	return &clusterStatusHandler{
		adapterStatusService: adapterStatusService,
		clusterService:       clusterService,
	}
}

// List returns all adapter statuses for a cluster with pagination
func (h clusterStatusHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			clusterID := mux.Vars(r)["id"]
			listArgs := services.NewListArguments(r.URL.Query())

			// Fetch adapter statuses with pagination
			adapterStatuses, total, err := h.adapterStatusService.FindByResourcePaginated(ctx, "Cluster", clusterID, listArgs)
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

// Create creates or updates an adapter status for a cluster
func (h clusterStatusHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req openapi.AdapterStatusCreateRequest

	cfg := &handlerConfig{
		&req,
		[]validate{
			validateNotEmpty(&req, "Adapter", "adapter"),
		},
		func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			clusterID := mux.Vars(r)["id"]

			// Verify cluster exists
			_, err := h.clusterService.Get(ctx, clusterID)
			if err != nil {
				return nil, err
			}

			// Create adapter status from request
			newStatus := api.AdapterStatusFromOpenAPICreate("Cluster", clusterID, &req)

			// Upsert (create or update based on resource_type + resource_id + adapter)
			adapterStatus, err := h.adapterStatusService.Upsert(ctx, newStatus)
			if err != nil {
				return nil, err
			}

			// Trigger status aggregation
			_, aggregateErr := h.clusterService.UpdateClusterStatusFromAdapters(ctx, clusterID)
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
