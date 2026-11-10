package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/fleet"
)

func TestPocFleetHandlerListsManagementClusters(t *testing.T) {
	RegisterTestingT(t)
	handler := NewPocFleetHandler(func() ([]fleet.MC, error) {
		return []fleet.MC{{Name: "mc-1"}, {Name: "mc-0"}}, nil
	}, "clm-test")

	recorder := httptest.NewRecorder()
	handler.GetPocFleet(recorder, httptest.NewRequest(http.MethodGet, "/poc-fleet", nil))

	Expect(recorder.Code).To(Equal(http.StatusOK))
	var response pocFleetResponse
	Expect(json.Unmarshal(recorder.Body.Bytes(), &response)).To(Succeed())
	Expect(response.ServiceCluster.Name).To(Equal("clm-test"))
	Expect(response.ServiceCluster.Version).To(Equal(api.Version))
	Expect(response.MCs).To(Equal([]string{"mc-1", "mc-0"}))
}

func TestPocFleetHandlerEmptyFleetReturnsEmptyList(t *testing.T) {
	RegisterTestingT(t)
	handler := NewPocFleetHandler(func() ([]fleet.MC, error) {
		return nil, nil
	}, "clm-test")

	recorder := httptest.NewRecorder()
	handler.GetPocFleet(recorder, httptest.NewRequest(http.MethodGet, "/poc-fleet", nil))

	Expect(recorder.Code).To(Equal(http.StatusOK))
	Expect(recorder.Body.String()).To(ContainSubstring(`"mcs":[]`))
}

func TestPocFleetHandlerReportsFleetErrors(t *testing.T) {
	RegisterTestingT(t)
	handler := NewPocFleetHandler(func() ([]fleet.MC, error) {
		return nil, fmt.Errorf("fleet directory is unreadable")
	}, "clm-test")

	recorder := httptest.NewRecorder()
	handler.GetPocFleet(recorder, httptest.NewRequest(http.MethodGet, "/poc-fleet", nil))

	Expect(recorder.Code).To(Equal(http.StatusInternalServerError))
}
