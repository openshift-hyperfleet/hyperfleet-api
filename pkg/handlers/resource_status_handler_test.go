package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	"gorm.io/datatypes"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

const testChannelID = "ch-1"

func newTestResourceStatusHandler(
	ctrl *gomock.Controller,
) (*ResourceStatusHandler, *services.MockResourceService, *services.MockAdapterStatusService) {
	mockResourceSvc := services.NewMockResourceService(ctrl)
	mockAdapterSvc := services.NewMockAdapterStatusService(ctrl)
	handler := NewResourceStatusHandler(channelDescriptor, mockResourceSvc, mockAdapterSvc)
	return handler, mockResourceSvc, mockAdapterSvc
}

func TestResourceStatusHandler_List(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)

	handler, mockResourceSvc, mockAdapterSvc := newTestResourceStatusHandler(ctrl)

	resource := &api.Resource{Kind: "Channel"}
	resource.ID = testChannelID
	mockResourceSvc.EXPECT().Get(gomock.Any(), "Channel", testChannelID).Return(resource, nil)

	now := time.Now().UTC()
	statuses := api.AdapterStatusList{
		{
			Adapter:            "adapter1",
			ResourceType:       "Channel",
			ResourceID:         testChannelID,
			ObservedGeneration: 1,
			LastReportTime:     now,
			Conditions:         datatypes.JSON(`[{"type":"Available","status":"True"}]`),
		},
	}
	mockAdapterSvc.EXPECT().FindByResourcePaginated(
		gomock.Any(), "Channel", testChannelID, gomock.Any(),
	).Return(statuses, int64(1), nil)

	r := httptest.NewRequest(http.MethodGet, "/channels/ch-1/statuses", nil)
	r = mux.SetURLVars(r, map[string]string{"id": testChannelID})
	w := httptest.NewRecorder()

	handler.List(w, r)

	Expect(w.Code).To(Equal(http.StatusOK))

	var response openapi.AdapterStatusList
	Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
	Expect(response.Items).To(HaveLen(1))
	Expect(response.Total).To(Equal(int32(1)))
}

func TestResourceStatusHandler_List_ResourceNotFound(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)

	handler, mockResourceSvc, _ := newTestResourceStatusHandler(ctrl)

	mockResourceSvc.EXPECT().Get(gomock.Any(), "Channel", testChannelID).
		Return(nil, errors.NotFound("Channel 'ch-1' not found"))

	r := httptest.NewRequest(http.MethodGet, "/channels/ch-1/statuses", nil)
	r = mux.SetURLVars(r, map[string]string{"id": testChannelID})
	w := httptest.NewRecorder()

	handler.List(w, r)

	Expect(w.Code).To(Equal(http.StatusNotFound))
}

func TestResourceStatusHandler_Create_HappyPath(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)

	handler, mockResourceSvc, _ := newTestResourceStatusHandler(ctrl)

	resource := &api.Resource{Kind: "Channel"}
	resource.ID = testChannelID
	mockResourceSvc.EXPECT().Get(gomock.Any(), "Channel", testChannelID).Return(resource, nil)

	now := time.Now().UTC()
	returnedStatus := &api.AdapterStatus{
		Adapter:            "adapter1",
		ResourceType:       "Channel",
		ResourceID:         testChannelID,
		ObservedGeneration: 1,
		LastReportTime:     now,
		Conditions: datatypes.JSON( //nolint:lll
			`[{"type":"Available","status":"True"},{"type":"Applied","status":"True"},{"type":"Health","status":"True"}]`),
	}
	mockResourceSvc.EXPECT().ProcessAdapterStatus(
		gomock.Any(), "Channel", testChannelID, gomock.Any(),
	).Return(returnedStatus, nil)

	observedTime := now
	body := openapi.AdapterStatusCreateRequest{
		Adapter:            "adapter1",
		ObservedGeneration: 1,
		ObservedTime:       observedTime,
		Conditions: []openapi.ConditionRequest{
			{Type: "Available", Status: openapi.AdapterConditionStatusTrue},
			{Type: "Applied", Status: openapi.AdapterConditionStatusTrue},
			{Type: "Health", Status: openapi.AdapterConditionStatusTrue},
		},
	}
	bodyJSON, _ := json.Marshal(body)

	r := httptest.NewRequest(http.MethodPut, "/channels/ch-1/statuses", strings.NewReader(string(bodyJSON)))
	r.Header.Set("Content-Type", "application/json")
	r = mux.SetURLVars(r, map[string]string{"id": testChannelID})
	w := httptest.NewRecorder()

	handler.Create(w, r)

	Expect(w.Code).To(Equal(http.StatusCreated))
}

func TestResourceStatusHandler_Create_Discarded_Returns204(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)

	handler, mockResourceSvc, _ := newTestResourceStatusHandler(ctrl)

	resource := &api.Resource{Kind: "Channel"}
	resource.ID = testChannelID
	mockResourceSvc.EXPECT().Get(gomock.Any(), "Channel", testChannelID).Return(resource, nil)
	mockResourceSvc.EXPECT().ProcessAdapterStatus(
		gomock.Any(), "Channel", testChannelID, gomock.Any(),
	).Return(nil, nil)

	now := time.Now().UTC()
	body := openapi.AdapterStatusCreateRequest{
		Adapter:            "adapter1",
		ObservedGeneration: 1,
		ObservedTime:       now,
		Conditions: []openapi.ConditionRequest{
			{Type: "Available", Status: openapi.AdapterConditionStatusTrue},
			{Type: "Applied", Status: openapi.AdapterConditionStatusTrue},
			{Type: "Health", Status: openapi.AdapterConditionStatusTrue},
		},
	}
	bodyJSON, _ := json.Marshal(body)

	r := httptest.NewRequest(http.MethodPut, "/channels/ch-1/statuses", strings.NewReader(string(bodyJSON)))
	r.Header.Set("Content-Type", "application/json")
	r = mux.SetURLVars(r, map[string]string{"id": testChannelID})
	w := httptest.NewRecorder()

	handler.Create(w, r)

	Expect(w.Code).To(Equal(http.StatusNoContent))
}

func TestResourceStatusHandler_Create_MissingAdapter_Returns400(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)

	handler, _, _ := newTestResourceStatusHandler(ctrl)

	body := openapi.AdapterStatusCreateRequest{
		ObservedGeneration: 1,
		Conditions: []openapi.ConditionRequest{
			{Type: "Available", Status: openapi.AdapterConditionStatusTrue},
		},
	}
	bodyJSON, _ := json.Marshal(body)

	r := httptest.NewRequest(http.MethodPut, "/channels/ch-1/statuses", strings.NewReader(string(bodyJSON)))
	r.Header.Set("Content-Type", "application/json")
	r = mux.SetURLVars(r, map[string]string{"id": testChannelID})
	w := httptest.NewRecorder()

	handler.Create(w, r)

	Expect(w.Code).To(Equal(http.StatusBadRequest))
}

func TestResourceStatusHandler_Create_ResourceNotFound(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)

	handler, mockResourceSvc, _ := newTestResourceStatusHandler(ctrl)

	mockResourceSvc.EXPECT().Get(gomock.Any(), "Channel", testChannelID).
		Return(nil, errors.NotFound("Channel 'ch-1' not found"))

	now := time.Now().UTC()
	body := openapi.AdapterStatusCreateRequest{
		Adapter:            "adapter1",
		ObservedGeneration: 1,
		ObservedTime:       now,
		Conditions: []openapi.ConditionRequest{
			{Type: "Available", Status: openapi.AdapterConditionStatusTrue},
			{Type: "Applied", Status: openapi.AdapterConditionStatusTrue},
			{Type: "Health", Status: openapi.AdapterConditionStatusTrue},
		},
	}
	bodyJSON, _ := json.Marshal(body)

	r := httptest.NewRequest(http.MethodPut, "/channels/ch-1/statuses", strings.NewReader(string(bodyJSON)))
	r.Header.Set("Content-Type", "application/json")
	r = mux.SetURLVars(r, map[string]string{"id": testChannelID})
	w := httptest.NewRecorder()

	handler.Create(w, r)

	Expect(w.Code).To(Equal(http.StatusNotFound))
}
