package handlers

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/presenters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

type ClusterNodePoolsHandler struct {
	clusterService  services.ClusterService
	nodePoolService services.NodePoolService
}

func NewClusterNodePoolsHandler(
	clusterService services.ClusterService,
	nodePoolService services.NodePoolService,
) *ClusterNodePoolsHandler {
	return &ClusterNodePoolsHandler{
		clusterService:  clusterService,
		nodePoolService: nodePoolService,
	}
}

// List returns all nodepools for a cluster
func (h ClusterNodePoolsHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			clusterID := mux.Vars(r)["id"]

			if err := validatePathID(clusterID, "cluster id"); err != nil {
				return nil, err
			}

			listArgs := services.NewListArguments(r.URL.Query())

			nodePools, paging, err := h.nodePoolService.ListByCluster(ctx, clusterID, listArgs)
			if err != nil {
				return nil, err
			}

			items := make([]openapi.NodePool, 0, len(nodePools))
			for _, nodePool := range nodePools {
				presented, err := presenters.PresentNodePool(nodePool)
				if err != nil {
					return nil, errors.GeneralError("Failed to present nodepool: %v", err)
				}
				items = append(items, presented)
			}

			nodePoolList := struct {
				Kind  string             `json:"kind"`
				Items []openapi.NodePool `json:"items"`
				Page  int32              `json:"page"`
				Size  int32              `json:"size"`
				Total int32              `json:"total"`
			}{
				Kind:  "NodePoolList",
				Page:  int32(paging.Page),
				Size:  int32(paging.Size),
				Total: int32(paging.Total),
				Items: items,
			}

			if listArgs.Fields != nil {
				filteredItems, err := presenters.SliceFilter(listArgs.Fields, nodePoolList)
				if err != nil {
					return nil, err
				}
				return filteredItems, nil
			}
			return nodePoolList, nil
		},
		ErrorHandler: handleError,
	}

	handleList(w, r, cfg)
}

// Get returns a specific nodepool for a cluster
func (h ClusterNodePoolsHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			clusterID := mux.Vars(r)["id"]
			nodePoolID := mux.Vars(r)["nodepool_id"]

			nodePool, err := h.nodePoolService.GetByIDAndOwner(ctx, nodePoolID, clusterID)
			if err != nil {
				return nil, err
			}

			presented, presErr := presenters.PresentNodePool(nodePool)
			if presErr != nil {
				return nil, errors.GeneralError("Failed to present nodepool: %v", presErr)
			}
			return presented, nil
		},
		ErrorHandler: handleError,
	}

	handleGet(w, r, cfg)
}

// SoftDelete soft-deletes a specific nodepool for a cluster
func (h ClusterNodePoolsHandler) SoftDelete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			clusterID := mux.Vars(r)["id"]
			nodePoolID := mux.Vars(r)["nodepool_id"]

			_, err := h.nodePoolService.GetByIDAndOwner(ctx, nodePoolID, clusterID)
			if err != nil {
				return nil, err
			}

			nodePool, err := h.nodePoolService.SoftDelete(ctx, nodePoolID)
			if err != nil {
				return nil, err
			}

			presented, presErr := presenters.PresentNodePool(nodePool)
			if presErr != nil {
				return nil, errors.GeneralError("Failed to present nodepool: %v", presErr)
			}
			return presented, nil
		},
		ErrorHandler: handleError,
	}

	handleSoftDelete(w, r, cfg, http.StatusAccepted)
}

// Patch patches a specific nodepool for a cluster
func (h ClusterNodePoolsHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch api.NodePoolPatchRequest

	cfg := &handlerConfig{
		MarshalInto: &patch,
		Validate: []validate{
			validatePatchRequest(&patch),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			clusterID := mux.Vars(r)["id"]
			nodePoolID := mux.Vars(r)["nodepool_id"]

			_, err := h.nodePoolService.GetByIDAndOwner(ctx, nodePoolID, clusterID)
			if err != nil {
				return nil, err
			}

			found, err := h.nodePoolService.Patch(ctx, nodePoolID, &patch)
			if err != nil {
				return nil, err
			}

			presented, presErr := presenters.PresentNodePool(found)
			if presErr != nil {
				return nil, errors.GeneralError("Failed to present nodepool: %v", presErr)
			}
			return presented, nil
		},
		ErrorHandler: handleError,
	}

	handle(w, r, cfg, http.StatusOK)
}

// Create creates a new nodepool for a cluster
func (h ClusterNodePoolsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req openapi.NodePoolCreateRequest
	cfg := &handlerConfig{
		MarshalInto: &req,
		Validate: []validate{
			validateEmpty(&req, "Id", "id"),
			validateName(&req, "Name", "name", 3, 15),
			validateKind(&req, "Kind", "kind", "NodePool"),
			validateSpec(&req, "Spec", "spec"),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			clusterID := mux.Vars(r)["id"]

			cluster, err := h.clusterService.Get(ctx, clusterID)
			if err != nil {
				return nil, err
			}

			if cluster.DeletedTime != nil {
				return nil, errors.ConflictState("Cluster '%s' is marked for deletion", clusterID)
			}

			nodePoolModel, convErr := presenters.ConvertNodePool(&req, cluster.ID)
			if convErr != nil {
				return nil, errors.GeneralError("Failed to convert nodepool: %v", convErr)
			}

			nodePoolModel, err = h.nodePoolService.Create(ctx, nodePoolModel)
			if err != nil {
				return nil, err
			}

			presented, presErr := presenters.PresentNodePool(nodePoolModel)
			if presErr != nil {
				return nil, errors.GeneralError("Failed to present nodepool: %v", presErr)
			}
			return presented, nil
		},
		ErrorHandler: handleError,
	}

	handle(w, r, cfg, http.StatusCreated)
}
