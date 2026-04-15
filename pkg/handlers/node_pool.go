package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/presenters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

var _ RestHandler = NodePoolHandler{}

type NodePoolHandler struct {
	nodePool services.NodePoolService
	generic  services.GenericService
}

func NewNodePoolHandler(nodePool services.NodePoolService, generic services.GenericService) *NodePoolHandler {
	return &NodePoolHandler{
		nodePool: nodePool,
		generic:  generic,
	}
}

func (h NodePoolHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req openapi.NodePoolCreateRequest
	cfg := &handlerConfig{
		MarshalInto: &req,
		Validate: []validate{
			validateEmpty(&req, "Id", "id"),
			validateName(&req, "Name", "name", 3, 15),
			validateKind(&req, "Kind", "kind", "NodePool"),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			// For standalone nodepools, owner_id would need to come from somewhere
			// This is likely not a supported use case, but using empty string for now
			nodePoolModel, convErr := presenters.ConvertNodePool(&req, "", "system@hyperfleet.local")
			if convErr != nil {
				return nil, errors.GeneralError("Failed to convert nodepool: %v", convErr)
			}
			nodePoolModel, err := h.nodePool.Create(ctx, nodePoolModel)
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

func (h NodePoolHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch api.NodePoolPatchRequest

	cfg := &handlerConfig{
		MarshalInto: &patch,
		Validate:    []validate{},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]
			found, err := h.nodePool.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			if patch.Spec != nil {
				specJSON, err := json.Marshal(*patch.Spec)
				if err != nil {
					return nil, errors.GeneralError("Failed to marshal spec: %v", err)
				}
				found.Spec = specJSON
			}
			// Note: OwnerID should not be changed after creation
			// if patch.OwnerID != nil {
			// 	found.OwnerID = *patch.OwnerID
			// }

			nodePoolModel, err := h.nodePool.Replace(ctx, found)
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

	handle(w, r, cfg, http.StatusOK)
}

func (h NodePoolHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			listArgs := services.NewListArguments(r.URL.Query())
			var nodePools []api.NodePool
			paging, err := h.generic.List(ctx, "username", listArgs, &nodePools)
			if err != nil {
				return nil, err
			}
			// Build list response manually since there's no NodePoolList in OpenAPI
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

func (h NodePoolHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			nodePool, err := h.nodePool.Get(ctx, id)
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

