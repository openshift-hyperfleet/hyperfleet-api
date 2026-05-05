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

var _ RestHandler = ClusterHandler{}

type ClusterHandler struct {
	cluster services.ClusterService
	generic services.GenericService
}

func NewClusterHandler(cluster services.ClusterService, generic services.GenericService) *ClusterHandler {
	return &ClusterHandler{
		cluster: cluster,
		generic: generic,
	}
}

func (h ClusterHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req openapi.ClusterCreateRequest
	cfg := &handlerConfig{
		MarshalInto: &req,
		Validate: []validate{
			validateEmpty(&req, "Id", "id"),
			validateName(&req, "Name", "name", 3, 53),
			validateKind(&req, "Kind", "kind", "Cluster"),
			validateSpec(&req, "Spec", "spec"),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			// Use the presenters.ConvertCluster helper to convert the request
			clusterModel, err := presenters.ConvertCluster(&req)
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
		ErrorHandler: handleError,
	}

	handle(w, r, cfg, http.StatusCreated)
}

func (h ClusterHandler) Patch(w http.ResponseWriter, r *http.Request) {
	var patch api.ClusterPatchRequest

	cfg := &handlerConfig{
		MarshalInto: &patch,
		Validate: []validate{
			validatePatchRequest(&patch),
		},
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()
			id := mux.Vars(r)["id"]

			clusterModel, err := h.cluster.Patch(ctx, id, &patch)
			if err != nil {
				return nil, err
			}
			presented, presErr := presenters.PresentCluster(clusterModel)
			if presErr != nil {
				return nil, errors.GeneralError("Failed to present cluster: %v", presErr)
			}
			return presented, nil
		},
		ErrorHandler: handleError,
	}

	handle(w, r, cfg, http.StatusOK)
}

func (h ClusterHandler) List(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			ctx := r.Context()

			listArgs := services.NewListArguments(r.URL.Query())
			var clusters []api.Cluster
			paging, err := h.generic.List(ctx, listArgs, &clusters)
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
				filteredItems, err := presenters.SliceFilter(listArgs.Fields, clusterList)
				if err != nil {
					return nil, err
				}
				return filteredItems, nil
			}
			return clusterList, nil
		},
		ErrorHandler: handleError,
	}

	handleList(w, r, cfg)
}

func (h ClusterHandler) Get(w http.ResponseWriter, r *http.Request) {
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
		ErrorHandler: handleError,
	}

	handleGet(w, r, cfg)
}

func (h ClusterHandler) SoftDelete(w http.ResponseWriter, r *http.Request) {
	cfg := &handlerConfig{
		Action: func() (interface{}, *errors.ServiceError) {
			id := mux.Vars(r)["id"]
			ctx := r.Context()
			cluster, err := h.cluster.SoftDelete(ctx, id)
			if err != nil {
				return nil, err
			}

			presented, presErr := presenters.PresentCluster(cluster)
			if presErr != nil {
				return nil, errors.GeneralError("Failed to present cluster: %v", presErr)
			}

			return presented, nil
		},
		ErrorHandler: handleError,
	}
	handleSoftDelete(w, r, cfg, http.StatusAccepted)
}
