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

var _ RestHandler = nodePoolHandler{}

type nodePoolHandler struct {
	nodePool services.NodePoolService
	generic  services.GenericService
}

func NewNodePoolHandler(nodePool services.NodePoolService, generic services.GenericService) *nodePoolHandler {
	return &nodePoolHandler{
		nodePool: nodePool,
		generic:  generic,
	}
}

func (h nodePoolHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req openapi.NodePoolCreateRequest
	cfg := &handlerConfig{
		&req,
		[]validate{
			validateEmpty(&req, "Id", "id"),
		},
		func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			// For standalone nodepools, owner_id would need to come from somewhere
			// This is likely not a supported use case, but using empty string for now
			nodePoolModel := api.NodePoolFromOpenAPICreate(&req, "", "system")
			nodePoolModel, err := h.nodePool.Create(ctx, nodePoolModel)
			if err != nil {
				return nil, err
			}
			return presenters.PresentNodePool(nodePoolModel), nil
		},
		handleError,
	}

	handle(w, r, cfg, http.StatusCreated)
}

func (h nodePoolHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch api.NodePoolPatchRequest

	cfg := &handlerConfig{
		&patch,
		[]validate{},
		func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]
			found, err := h.nodePool.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			//patch a field
			if patch.Name != nil {
				found.Name = *patch.Name
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
			return presenters.PresentNodePool(nodePoolModel), nil
		},
		handleError,
	}

	handle(w, r, cfg, http.StatusOK)
}

func (h nodePoolHandler) List(w http.ResponseWriter, r *http.Request) {
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
				converted := presenters.PresentNodePool(&nodePool)
				items = append(items, converted)
			}

			nodePoolList := struct {
				Kind  string              `json:"kind"`
				Page  int32               `json:"page"`
				Size  int32               `json:"size"`
				Total int32               `json:"total"`
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

func (h nodePoolHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			nodePool, err := h.nodePool.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			return presenters.PresentNodePool(nodePool), nil
		},
	}

	handleGet(w, r, cfg)
}

func (h nodePoolHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			return nil, errors.NotImplemented("delete")
		},
	}
	handleDelete(w, r, cfg, http.StatusNoContent)
}
