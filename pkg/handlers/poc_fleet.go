package handlers

import (
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/fleet"
)

// PocFleetHandler serves the dashboard's read-through view of the management fleet.
type PocFleetHandler struct {
	list               func() ([]fleet.MC, error)
	serviceClusterName string
}

func NewPocFleetHandler(
	list func() ([]fleet.MC, error), serviceClusterName string,
) *PocFleetHandler {
	return &PocFleetHandler{list: list, serviceClusterName: serviceClusterName}
}

type pocFleetResponse struct {
	ServiceCluster pocServiceCluster `json:"serviceCluster"`
	MCs            []string          `json:"mcs"`
}

type pocServiceCluster struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

func (h *PocFleetHandler) GetPocFleet(w http.ResponseWriter, r *http.Request) {
	mcs, err := h.list()
	if err != nil {
		handleError(r, w, errors.GeneralError("list management cluster fleet: %v", err))
		return
	}

	names := make([]string, len(mcs))
	for index, mc := range mcs {
		names[index] = mc.Name
	}

	writeJSONResponse(w, r, http.StatusOK, pocFleetResponse{
		ServiceCluster: pocServiceCluster{
			Name:    h.serviceClusterName,
			Version: api.Version,
		},
		MCs: names,
	})
}
