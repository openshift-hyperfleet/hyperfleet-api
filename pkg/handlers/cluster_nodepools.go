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

type clusterNodePoolsHandler struct {
	clusterService  services.ClusterService
	nodePoolService services.NodePoolService
	generic         services.GenericService
}

func NewClusterNodePoolsHandler(
	clusterService services.ClusterService,
	nodePoolService services.NodePoolService,
	generic services.GenericService,
) *clusterNodePoolsHandler {
	return &clusterNodePoolsHandler{
		clusterService:  clusterService,
		nodePoolService: nodePoolService,
		generic:         generic,
	}
}

// List returns all nodepools for a cluster
func (h clusterNodePoolsHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			clusterID := mux.Vars(r)["id"]

			// Verify cluster exists
			_, err := h.clusterService.Get(ctx, clusterID)
			if err != nil {
				return nil, err
			}

			// Get nodepools with owner_id = clusterID
			listArgs := services.NewListArguments(r.URL.Query())
			// Add filter for owner_id
			if listArgs.Search == "" {
				listArgs.Search = "owner_id = '" + clusterID + "'"
			} else {
				listArgs.Search = listArgs.Search + " AND owner_id = '" + clusterID + "'"
			}

			var nodePools []api.NodePool
			paging, err := h.generic.List(ctx, "username", listArgs, &nodePools)
			if err != nil {
				return nil, err
			}

			// Build list response
			items := make([]openapi.NodePool, 0, len(nodePools))
			for _, nodePool := range nodePools {
				presented, err := presenters.PresentNodePool(&nodePool)
				if err != nil {
					return nil, errors.GeneralError("Failed to present nodepool: %v", err)
				}
				items = append(items, presented)
			}

			nodePoolList := struct {
				Kind  string             `json:"kind"`
				Page  int32              `json:"page"`
				Size  int32              `json:"size"`
				Total int32              `json:"total"`
				Items []openapi.NodePool `json:"items"`
			}{
				Kind:  "NodePoolList",
				Page:  int32(paging.Page),
				Size:  int32(paging.Size),
				Total: int32(paging.Total),
				Items: items,
			}

			if listArgs.Fields != nil {
				filteredItems, err := presenters.SliceFilter(listArgs.Fields, nodePoolList.Items)
				if err != nil {
					return nil, err
				}
				return filteredItems, nil
			}
			return nodePoolList, nil
		},
	}

	handleList(w, r, cfg)
}

// Get returns a specific nodepool for a cluster
func (h clusterNodePoolsHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			clusterID := mux.Vars(r)["id"]
			nodePoolID := mux.Vars(r)["nodepool_id"]

			// Verify cluster exists
			_, err := h.clusterService.Get(ctx, clusterID)
			if err != nil {
				return nil, err
			}

			// Get nodepool
			nodePool, err := h.nodePoolService.Get(ctx, nodePoolID)
			if err != nil {
				return nil, err
			}

			// Verify nodepool belongs to this cluster
			if nodePool.OwnerID != clusterID {
				return nil, errors.NotFound("NodePool '%s' not found for cluster '%s'", nodePoolID, clusterID)
			}

			presented, presErr := presenters.PresentNodePool(nodePool)
			if presErr != nil {
				return nil, errors.GeneralError("Failed to present nodepool: %v", presErr)
			}
			return presented, nil
		},
	}

	handleGet(w, r, cfg)
}

// Create creates a new nodepool for a cluster
func (h clusterNodePoolsHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req openapi.NodePoolCreateRequest
	cfg := &handlerConfig{
		&req,
		[]validate{
			validateEmpty(&req, "Id", "id"),
			validateName(&req, "Name", "name", 3, 15),
			validateKind(&req, "Kind", "kind", "NodePool"),
		},
		func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			clusterID := mux.Vars(r)["id"]

			// Verify cluster exists
			cluster, err := h.clusterService.Get(ctx, clusterID)
			if err != nil {
				return nil, err
			}

			// Use the presenters.ConvertNodePool helper to convert the request
			nodePoolModel, convErr := presenters.ConvertNodePool(&req, cluster.ID, "system@hyperfleet.local")
			if convErr != nil {
				return nil, errors.GeneralError("Failed to convert nodepool: %v", convErr)
			}

			// Create nodepool
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
		handleError,
	}

	handle(w, r, cfg, http.StatusCreated)
}
