package handlers

import (
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/presenters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

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

func (h NodePoolHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			listArgs := services.NewListArguments(r.URL.Query())
			var nodePools []api.NodePool
			paging, err := h.generic.List(ctx, listArgs, &nodePools)
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
				Items []openapi.NodePool `json:"items"`
				Page  int32              `json:"page"`
				Size  int32              `json:"size"`
				Total int32              `json:"total"`
			}{
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
