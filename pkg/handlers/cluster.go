package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/api/presenters"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/services"
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
		},
		func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			// Use the ClusterFromOpenAPICreate helper to convert the request
			clusterModel := api.ClusterFromOpenAPICreate(&req, "system")
			clusterModel, err := h.cluster.Create(ctx, clusterModel)
			if err != nil {
				return nil, err
			}
			return presenters.PresentCluster(clusterModel), nil
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

			//patch a field
			if patch.Name != nil {
				found.Name = *patch.Name
			}
			if patch.Spec != nil {
				specJSON, _ := json.Marshal(*patch.Spec)
				found.Spec = specJSON
			}
			if patch.Generation != nil {
				found.Generation = *patch.Generation
			}

			clusterModel, err := h.cluster.Replace(ctx, found)
			if err != nil {
				return nil, err
			}
			return presenters.PresentCluster(clusterModel), nil
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
				converted := presenters.PresentCluster(&cluster)
				clusterList.Items = append(clusterList.Items, converted)
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

			return presenters.PresentCluster(cluster), nil
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
