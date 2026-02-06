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

var _ RestHandler = clusterHandler{}

type clusterHandler struct {
	cluster services.ClusterService
	generic services.GenericService
}

func NewClusterHandler(cluster services.ClusterService, generic services.GenericService) *clusterHandler {
	return &clusterHandler{
		cluster: cluster,
		generic: generic,
	}
}

func (h clusterHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req openapi.ClusterCreateRequest
	cfg := &handlerConfig{
		&req,
		[]validate{
			validateEmpty(&req, "Id", "id"),
			validateName(&req, "Name", "name", 3, 53),
			validateKind(&req, "Kind", "kind", "Cluster"),
		},
		func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			// Use the presenters.ConvertCluster helper to convert the request
			clusterModel, err := presenters.ConvertCluster(&req, "system@hyperfleet.local")
			if err != nil {
				return nil, errors.GeneralError("Failed to convert cluster: %v", err)
			}
			clusterModel, svcErr := h.cluster.Create(ctx, clusterModel)
			if svcErr != nil {
				return nil, svcErr
			}
			presented, err := presenters.PresentCluster(clusterModel)
			if err != nil {
				return nil, errors.GeneralError("Failed to present cluster: %v", err)
			}
			return presented, nil
		},
		handleError,
	}

	handle(w, r, cfg, http.StatusCreated)
}

func (h clusterHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch api.ClusterPatchRequest

	cfg := &handlerConfig{
		&patch,
		[]validate{},
		func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]
			found, err := h.cluster.Get(ctx, id)
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

			clusterModel, err := h.cluster.Replace(ctx, found)
			if err != nil {
				return nil, err
			}
			presented, presErr := presenters.PresentCluster(clusterModel)
			if presErr != nil {
				return nil, errors.GeneralError("Failed to present cluster: %v", presErr)
			}
			return presented, nil
		},
		handleError,
	}

	handle(w, r, cfg, http.StatusOK)
}

func (h clusterHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			listArgs := services.NewListArguments(r.URL.Query())
			var clusters []api.Cluster
			paging, err := h.generic.List(ctx, "username", listArgs, &clusters)
			if err != nil {
				return nil, err
			}
			clusterList := openapi.ClusterList{
				Kind:  "ClusterList",
				Page:  int32(paging.Page),
				Size:  int32(paging.Size),
				Total: int32(paging.Total),
				Items: []openapi.Cluster{},
			}

			for _, cluster := range clusters {
				presented, err := presenters.PresentCluster(&cluster)
				if err != nil {
					return nil, errors.GeneralError("Failed to present cluster: %v", err)
				}
				clusterList.Items = append(clusterList.Items, presented)
			}
			if listArgs.Fields != nil {
				filteredItems, err := presenters.SliceFilter(listArgs.Fields, clusterList.Items)
				if err != nil {
					return nil, err
				}
				return filteredItems, nil
			}
			return clusterList, nil
		},
	}

	handleList(w, r, cfg)
}

func (h clusterHandler) Get(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			cluster, err := h.cluster.Get(ctx, id)
			if err != nil {
				return nil, err
			}

			presented, presErr := presenters.PresentCluster(cluster)
			if presErr != nil {
				return nil, errors.GeneralError("Failed to present cluster: %v", presErr)
			}
			return presented, nil
		},
	}

	handleGet(w, r, cfg)
}

func (h clusterHandler) Delete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			return nil, errors.NotImplemented("delete")
		},
	}
	handleDelete(w, r, cfg, http.StatusNoContent)
}
