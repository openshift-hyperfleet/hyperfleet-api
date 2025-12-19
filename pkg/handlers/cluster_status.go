package handlers

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/presenters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
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
				presented, presErr := presenters.PresentAdapterStatus(as)
				if presErr != nil {
					return nil, errors.GeneralError("Failed to present adapter status: %v", presErr)
				}
				items = append(items, presented)
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
			newStatus, convErr := presenters.ConvertAdapterStatus("Cluster", clusterID, &req)
			if convErr != nil {
				return nil, errors.GeneralError("Failed to convert adapter status: %v", convErr)
			}

			// Upsert (create or update based on resource_type + resource_id + adapter)
			adapterStatus, err := h.adapterStatusService.Upsert(ctx, newStatus)
			if err != nil {
				return nil, err
			}

			// Trigger status aggregation
			if _, aggregateErr := h.clusterService.UpdateClusterStatusFromAdapters(ctx, clusterID); aggregateErr != nil {
				// Log error but don't fail the request - the status will be computed on next update
				log := logger.NewOCMLogger(ctx)
				log.Extra("cluster_id", clusterID).Extra("error", aggregateErr).Warning("Failed to aggregate cluster status")
			}

			status, presErr := presenters.PresentAdapterStatus(adapterStatus)
			if presErr != nil {
				return nil, errors.GeneralError("Failed to present adapter status: %v", presErr)
			}
			return &status, nil
		},
		handleError,
	}

	handle(w, r, cfg, http.StatusCreated)
}
