package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	hfl "github.com/openshift-hyperfleet/hyperfleet-logger"
	"go.uber.org/mock/gomock"
	"gorm.io/datatypes"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

const testChannelID = "ch-1"

func TestStatusHandlers_AdapterContextReachesServiceAndErrorResponse(t *testing.T) {
	for _, route := range []string{"entity", "root"} {
		t.Run(route, func(t *testing.T) {
			var output bytes.Buffer
			previous := slog.Default()
			slog.SetDefault(logger.NewLogger("test", logger.HandlerConfig{
				Level: slog.LevelInfo, Format: hfl.FormatJSON, Output: &output,
			}))
			t.Cleanup(func() { slog.SetDefault(previous) })
			ctrl := gomock.NewController(t)
			resourceSvc := services.NewMockResourceService(ctrl)
			adapterSvc := services.NewMockAdapterStatusService(ctrl)
			resource := &api.Resource{Kind: "Channel"}
			resource.ID = testChannelID
			resourceSvc.EXPECT().ProcessAdapterStatus(
				gomock.Any(), resource.Kind, resource.ID, gomock.Any(),
			).DoAndReturn(func(
				ctx context.Context, _, _ string, _ *api.AdapterStatus,
			) (*api.AdapterStatus, *errors.ServiceError) {
				if adapter, _ := hfl.Get(ctx, logger.AdapterKey); adapter != "adapter1" {
					t.Errorf("service adapter = %q, want adapter1", adapter)
				}
				if requestID, _ := logger.GetRequestID(ctx); requestID != "request-1" {
					t.Errorf("service request ID = %q, want request-1", requestID)
				}
				return nil, errors.GeneralError("test failure")
			})
			body := openapi.AdapterStatusCreateRequest{
				Adapter: "adapter1", ObservedGeneration: 1, ObservedTime: time.Now().UTC(),
				Conditions: []openapi.ConditionRequest{
					{Type: "Available", Status: openapi.AdapterConditionStatusTrue},
					{Type: "Applied", Status: openapi.AdapterConditionStatusTrue},
					{Type: "Health", Status: openapi.AdapterConditionStatusTrue},
				},
			}
			bodyJSON, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			parent := hfl.Set(t.Context(), logger.ReqIDKey, "request-1")
			parent = hfl.WithTraceID(parent, "trace-1")
			req := httptest.NewRequest(http.MethodPut, "/statuses", bytes.NewReader(bodyJSON)).WithContext(parent)
			req.SetPathValue("id", testChannelID)
			res := httptest.NewRecorder()
			if route == "entity" {
				resourceSvc.EXPECT().Get(gomock.Any(), resource.Kind, resource.ID).Return(resource, nil)
				NewResourceStatusHandler(channelDescriptor, resourceSvc, adapterSvc).Create(res, req)
			} else {
				resourceSvc.EXPECT().GetByID(gomock.Any(), resource.ID).Return(resource, nil)
				NewRootResourceHandler(resourceSvc, adapterSvc, nil).CreateStatus(res, req)
			}
			if res.Code != http.StatusInternalServerError {
				t.Fatalf("HTTP status = %d, want 500", res.Code)
			}
			var record map[string]any
			if err := json.Unmarshal(output.Bytes(), &record); err != nil {
				t.Fatal(err)
			}
			for key, want := range map[string]string{
				"adapter": "adapter1", "request_id": "request-1", "trace_id": "trace-1",
			} {
				if record[key] != want {
					t.Errorf("error log %s = %v, want %q", key, record[key], want)
				}
			}
			if _, ok := hfl.Get(parent, logger.AdapterKey); ok {
				t.Error("handler must not modify the caller's context")
			}
		})
	}
}

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
	r.SetPathValue("id", testChannelID)
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
	r.SetPathValue("id", testChannelID)
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
	r.SetPathValue("id", testChannelID)
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
	r.SetPathValue("id", testChannelID)
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
	r.SetPathValue("id", testChannelID)
	w := httptest.NewRecorder()

	handler.Create(w, r)

	Expect(w.Code).To(Equal(http.StatusBadRequest))
}

// TestResourceStatusHandler_Create_ConditionsTypeMismatch verifies that a wrong JSON
// type for "conditions" returns a clean validation message without leaking the Go
// struct/package name (e.g. "AdapterStatusCreateRequest", "openapi.ConditionRequest")
// — HYPERFLEET-1376 finding 7.
func TestResourceStatusHandler_Create_ConditionsTypeMismatch(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)

	handler, _, _ := newTestResourceStatusHandler(ctrl)

	body := `{"adapter":"x","conditions":"not-an-array"}`

	r := httptest.NewRequest(http.MethodPut, "/channels/ch-1/statuses", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.SetPathValue("id", testChannelID)
	w := httptest.NewRecorder()

	handler.Create(w, r)

	Expect(w.Code).To(Equal(http.StatusBadRequest))
	Expect(w.Body.String()).To(ContainSubstring("field 'conditions' must be an array"))
	Expect(w.Body.String()).ToNot(ContainSubstring("AdapterStatusCreateRequest"))
	Expect(w.Body.String()).ToNot(ContainSubstring("openapi."))
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
	r.SetPathValue("id", testChannelID)
	w := httptest.NewRecorder()

	handler.Create(w, r)

	Expect(w.Code).To(Equal(http.StatusNotFound))
}

// ─── Nested (parent_id) branch tests ──────────────────────────────

func newTestVersionStatusHandler(
	ctrl *gomock.Controller,
) (*ResourceStatusHandler, *services.MockResourceService, *services.MockAdapterStatusService) {
	mockResourceSvc := services.NewMockResourceService(ctrl)
	mockAdapterSvc := services.NewMockAdapterStatusService(ctrl)
	handler := NewResourceStatusHandler(versionDescriptor, mockResourceSvc, mockAdapterSvc)
	return handler, mockResourceSvc, mockAdapterSvc
}

func TestResourceStatusHandler_List_NestedWithParentID(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)

	handler, mockResourceSvc, mockAdapterSvc := newTestVersionStatusHandler(ctrl)

	parentID := "ch-parent"
	versionID := "ver-1"

	resource := &api.Resource{Kind: "Version"}
	resource.ID = versionID
	mockResourceSvc.EXPECT().GetByOwner(gomock.Any(), "Version", versionID, parentID).Return(resource, nil)

	mockAdapterSvc.EXPECT().FindByResourcePaginated(
		gomock.Any(), "Version", versionID, gomock.Any(),
	).Return(api.AdapterStatusList{}, int64(0), nil)

	r := httptest.NewRequest(http.MethodGet, "/channels/"+parentID+"/versions/"+versionID+"/statuses", nil)
	r.SetPathValue("parent_id", parentID)
	r.SetPathValue("id", versionID)
	w := httptest.NewRecorder()

	handler.List(w, r)

	Expect(w.Code).To(Equal(http.StatusOK))
}

func TestResourceStatusHandler_Create_NestedWithParentID(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)

	handler, mockResourceSvc, _ := newTestVersionStatusHandler(ctrl)

	parentID := "ch-parent"
	versionID := "ver-1"

	resource := &api.Resource{Kind: "Version"}
	resource.ID = versionID
	mockResourceSvc.EXPECT().GetByOwner(gomock.Any(), "Version", versionID, parentID).Return(resource, nil)

	now := time.Now().UTC()
	returnedStatus := &api.AdapterStatus{
		Adapter:            "adapter1",
		ResourceType:       "Version",
		ResourceID:         versionID,
		ObservedGeneration: 1,
		LastReportTime:     now,
		Conditions: datatypes.JSON( //nolint:lll
			`[{"type":"Available","status":"True"},{"type":"Applied","status":"True"},{"type":"Health","status":"True"}]`),
	}
	mockResourceSvc.EXPECT().ProcessAdapterStatus(
		gomock.Any(), "Version", versionID, gomock.Any(),
	).Return(returnedStatus, nil)

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

	r := httptest.NewRequest(http.MethodPut, "/channels/"+parentID+"/versions/"+versionID+"/statuses",
		strings.NewReader(string(bodyJSON)))
	r.Header.Set("Content-Type", "application/json")
	r.SetPathValue("parent_id", parentID)
	r.SetPathValue("id", versionID)
	w := httptest.NewRecorder()

	handler.Create(w, r)

	Expect(w.Code).To(Equal(http.StatusCreated))
}

func TestResourceStatusHandler_List_NestedWrongParent_Returns404(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)

	handler, mockResourceSvc, _ := newTestVersionStatusHandler(ctrl)

	mockResourceSvc.EXPECT().GetByOwner(gomock.Any(), "Version", "ver-1", "wrong-parent").
		Return(nil, errors.NotFound("Version 'ver-1' not found under parent 'wrong-parent'"))

	r := httptest.NewRequest(http.MethodGet, "/channels/wrong-parent/versions/ver-1/statuses", nil)
	r.SetPathValue("parent_id", "wrong-parent")
	r.SetPathValue("id", "ver-1")
	w := httptest.NewRecorder()

	handler.List(w, r)

	Expect(w.Code).To(Equal(http.StatusNotFound))
}

// ─── RootResourceHandler status tests ──────────────────────────────

func newTestRootResourceHandler(
	ctrl *gomock.Controller,
) (*RootResourceHandler, *services.MockResourceService, *services.MockAdapterStatusService) {
	mockResourceSvc := services.NewMockResourceService(ctrl)
	mockAdapterSvc := services.NewMockAdapterStatusService(ctrl)
	handler := NewRootResourceHandler(mockResourceSvc, mockAdapterSvc, nil)
	return handler, mockResourceSvc, mockAdapterSvc
}

func TestRootResourceHandler_ListStatuses(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)

	handler, mockResourceSvc, mockAdapterSvc := newTestRootResourceHandler(ctrl)

	resource := &api.Resource{Kind: "Channel"}
	resource.ID = testChannelID
	mockResourceSvc.EXPECT().GetByID(gomock.Any(), testChannelID).Return(resource, nil)

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

	r := httptest.NewRequest(http.MethodGet, "/resources/"+testChannelID+"/statuses", nil)
	r.SetPathValue("id", testChannelID)
	w := httptest.NewRecorder()

	handler.ListStatuses(w, r)

	Expect(w.Code).To(Equal(http.StatusOK))

	var response openapi.AdapterStatusList
	Expect(json.Unmarshal(w.Body.Bytes(), &response)).To(Succeed())
	Expect(response.Items).To(HaveLen(1))
	Expect(response.Total).To(Equal(int32(1)))
}

func TestRootResourceHandler_ListStatuses_ResourceNotFound(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)

	handler, mockResourceSvc, _ := newTestRootResourceHandler(ctrl)

	mockResourceSvc.EXPECT().GetByID(gomock.Any(), testChannelID).
		Return(nil, errors.NotFound("Resource '%s' not found", testChannelID))

	r := httptest.NewRequest(http.MethodGet, "/resources/"+testChannelID+"/statuses", nil)
	r.SetPathValue("id", testChannelID)
	w := httptest.NewRecorder()

	handler.ListStatuses(w, r)

	Expect(w.Code).To(Equal(http.StatusNotFound))
}

func TestRootResourceHandler_CreateStatus_HappyPath(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)

	handler, mockResourceSvc, _ := newTestRootResourceHandler(ctrl)

	resource := &api.Resource{Kind: "Channel"}
	resource.ID = testChannelID
	mockResourceSvc.EXPECT().GetByID(gomock.Any(), testChannelID).Return(resource, nil)

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

	r := httptest.NewRequest(http.MethodPut, "/resources/"+testChannelID+"/statuses",
		strings.NewReader(string(bodyJSON)))
	r.Header.Set("Content-Type", "application/json")
	r.SetPathValue("id", testChannelID)
	w := httptest.NewRecorder()

	handler.CreateStatus(w, r)

	Expect(w.Code).To(Equal(http.StatusCreated))
}

func TestRootResourceHandler_CreateStatus_Discarded_Returns204(t *testing.T) {
	RegisterTestingT(t)
	ctrl := gomock.NewController(t)

	handler, mockResourceSvc, _ := newTestRootResourceHandler(ctrl)

	resource := &api.Resource{Kind: "Channel"}
	resource.ID = testChannelID
	mockResourceSvc.EXPECT().GetByID(gomock.Any(), testChannelID).Return(resource, nil)
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

	r := httptest.NewRequest(http.MethodPut, "/resources/"+testChannelID+"/statuses",
		strings.NewReader(string(bodyJSON)))
	r.Header.Set("Content-Type", "application/json")
	r.SetPathValue("id", testChannelID)
	w := httptest.NewRecorder()

	handler.CreateStatus(w, r)

	Expect(w.Code).To(Equal(http.StatusNoContent))
}
