package handlers

import (
	"encoding/json"
	"fmt"
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
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

var channelDescriptor = registry.EntityDescriptor{
	Kind:           "Channel",
	Plural:         "channels",
	SpecSchemaName: "ChannelSpec",
}

var versionDescriptor = registry.EntityDescriptor{
	Kind:           "Version",
	Plural:         "versions",
	ParentKind:     "Channel",
	SpecSchemaName: "VersionSpec",
}

func newTestResourceHandler(
	ctrl *gomock.Controller,
) (*ResourceHandler, *services.MockResourceService) {
	mockResourceSvc := services.NewMockResourceService(ctrl)
	handler := NewResourceHandler(channelDescriptor, mockResourceSvc)
	return handler, mockResourceSvc
}

func newTestVersionHandler(
	ctrl *gomock.Controller,
) (*ResourceHandler, *services.MockResourceService) {
	mockResourceSvc := services.NewMockResourceService(ctrl)
	handler := NewResourceHandler(versionDescriptor, mockResourceSvc)
	return handler, mockResourceSvc
}

func TestResourceHandler_Create(t *testing.T) {
	now := time.Now()

	tests := []struct {
		setupMock          func(mock *services.MockResourceService)
		name               string
		body               string
		expectedStatusCode int
		expectedError      bool
	}{
		{
			name: "Success - creates channel",
			body: `{"kind":"Channel","name":"stable","spec":{"is_default":true}}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Create(
					gomock.Any(), "Channel", gomock.AssignableToTypeOf(&api.Resource{}), gomock.Any(),
				).Return(&api.Resource{
					Meta:       api.Meta{ID: "ch-123", CreatedTime: now, UpdatedTime: now},
					Kind:       "Channel",
					Name:       "stable",
					Href:       "/api/hyperfleet/v1/channels/ch-123",
					Spec:       datatypes.JSON(`{"is_default":true}`),
					Generation: 1,
					CreatedBy:  "system@hyperfleet.local",
					UpdatedBy:  "system@hyperfleet.local",
				}, nil)
			},
			expectedStatusCode: http.StatusCreated,
		},
		{
			name:               "Error 400 - wrong kind",
			body:               `{"kind":"WrongKind","name":"stable","spec":{"is_default":true}}`,
			setupMock:          func(mock *services.MockResourceService) {},
			expectedStatusCode: http.StatusBadRequest,
			expectedError:      true,
		},
		{
			name:               "Error 400 - missing spec",
			body:               `{"kind":"Channel","name":"stable"}`,
			setupMock:          func(mock *services.MockResourceService) {},
			expectedStatusCode: http.StatusBadRequest,
			expectedError:      true,
		},
		{
			name: "Error 409 - duplicate name",
			body: `{"kind":"Channel","name":"stable","spec":{"is_default":true}}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Create(gomock.Any(), "Channel", gomock.AssignableToTypeOf(&api.Resource{}), gomock.Any()).
					Return(nil, errors.Conflict("Channel with name 'stable' already exists"))
			},
			expectedStatusCode: http.StatusConflict,
			expectedError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, mockResourceSvc := newTestResourceHandler(ctrl)
			tt.setupMock(mockResourceSvc)

			req := httptest.NewRequest(http.MethodPost,
				"/api/hyperfleet/v1/channels", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			handler.Create(rr, req)
			Expect(rr.Code).To(Equal(tt.expectedStatusCode))

			if !tt.expectedError {
				var resp openapi.Resource
				err := json.Unmarshal(rr.Body.Bytes(), &resp)
				Expect(err).NotTo(HaveOccurred())
				Expect(resp.Id).To(Equal("ch-123"))
				Expect(resp.Kind).To(Equal("Channel"))
			}
		})
	}
}

func TestResourceHandler_Get(t *testing.T) {
	now := time.Now()

	tests := []struct {
		setupMock          func(mock *services.MockResourceService)
		name               string
		id                 string
		expectedStatusCode int
	}{
		{
			name: "Success",
			id:   "ch-123",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Get(gomock.Any(), "Channel", "ch-123").Return(&api.Resource{
					Meta:      api.Meta{ID: "ch-123", CreatedTime: now, UpdatedTime: now},
					Kind:      "Channel",
					Name:      "stable",
					Spec:      datatypes.JSON(`{}`),
					CreatedBy: "user@test.com",
					UpdatedBy: "user@test.com",
				}, nil)
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Not found",
			id:   "nonexistent",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Get(gomock.Any(), "Channel", "nonexistent").
					Return(nil, errors.NotFound("Channel not found"))
			},
			expectedStatusCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, mockResourceSvc := newTestResourceHandler(ctrl)
			tt.setupMock(mockResourceSvc)

			req := httptest.NewRequest(http.MethodGet,
				"/api/hyperfleet/v1/channels/"+tt.id, nil)
			req = mux.SetURLVars(req, map[string]string{"id": tt.id})
			rr := httptest.NewRecorder()

			handler.Get(rr, req)
			Expect(rr.Code).To(Equal(tt.expectedStatusCode))
		})
	}
}

func TestResourceHandler_List(t *testing.T) {
	now := time.Now()

	tests := []struct {
		setupMock          func(mock *services.MockResourceService)
		name               string
		expectedStatusCode int
		expectedItems      int
	}{
		{
			name: "Success with results",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().List(gomock.Any(), "Channel", gomock.AssignableToTypeOf(&services.ListArguments{})).
					Return(api.ResourceList{
						&api.Resource{
							Meta: api.Meta{ID: "ch-1", CreatedTime: now, UpdatedTime: now},
							Kind: "Channel", Name: "stable",
							Spec:      datatypes.JSON(`{}`),
							CreatedBy: "u@test.com", UpdatedBy: "u@test.com",
						},
					}, &api.PagingMeta{Page: 1, Size: 1, Total: 1}, nil)
			},
			expectedStatusCode: http.StatusOK,
			expectedItems:      1,
		},
		{
			name: "Empty list",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().List(gomock.Any(), "Channel", gomock.AssignableToTypeOf(&services.ListArguments{})).
					Return(api.ResourceList{}, &api.PagingMeta{Page: 1, Size: 0, Total: 0}, nil)
			},
			expectedStatusCode: http.StatusOK,
			expectedItems:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, mockResourceSvc := newTestResourceHandler(ctrl)
			tt.setupMock(mockResourceSvc)

			req := httptest.NewRequest(http.MethodGet,
				"/api/hyperfleet/v1/channels", nil)
			rr := httptest.NewRecorder()

			handler.List(rr, req)
			Expect(rr.Code).To(Equal(tt.expectedStatusCode))

			var resp openapi.ResourceList
			err := json.Unmarshal(rr.Body.Bytes(), &resp)
			Expect(err).NotTo(HaveOccurred())
			Expect(resp.Items).To(HaveLen(tt.expectedItems))
		})
	}
}

func TestResourceHandler_Patch(t *testing.T) {
	now := time.Now()

	tests := []struct {
		setupMock          func(mock *services.MockResourceService)
		name               string
		id                 string
		body               string
		expectedStatusCode int
	}{
		{
			name: "Success",
			id:   "ch-123",
			body: `{"spec":{"is_default":false}}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Patch(gomock.Any(), "Channel", "ch-123", gomock.AssignableToTypeOf(&api.ResourcePatch{})).
					Return(&api.Resource{
						Meta:       api.Meta{ID: "ch-123", CreatedTime: now, UpdatedTime: now},
						Kind:       "Channel",
						Name:       "stable",
						Spec:       datatypes.JSON(`{"is_default":false}`),
						Generation: 2,
						CreatedBy:  "user@test.com",
						UpdatedBy:  "user@test.com",
					}, nil)
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			// References-only patch must be accepted, not rejected as "no field provided" — HYPERFLEET-1376 finding 8.
			name: "Success - references only",
			id:   "ch-123",
			body: `{"references":{"parents":[{"kind":"Cluster","id":"cluster-1"}]}}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Patch(gomock.Any(), "Channel", "ch-123", gomock.AssignableToTypeOf(&api.ResourcePatch{})).
					Return(&api.Resource{
						Meta:       api.Meta{ID: "ch-123", CreatedTime: now, UpdatedTime: now},
						Kind:       "Channel",
						Name:       "stable",
						Spec:       datatypes.JSON(`{"is_default":false}`),
						Generation: 2,
						CreatedBy:  "user@test.com",
						UpdatedBy:  "user@test.com",
					}, nil)
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Not found",
			id:   "nonexistent",
			body: `{"spec":{"is_default":false}}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Patch(gomock.Any(), "Channel", "nonexistent", gomock.AssignableToTypeOf(&api.ResourcePatch{})).
					Return(nil, errors.NotFound("Channel not found"))
			},
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name: "Conflict 409 - soft-deleted resource",
			id:   "ch-123",
			body: `{"spec":{"is_default":false}}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Patch(gomock.Any(), "Channel", "ch-123", gomock.AssignableToTypeOf(&api.ResourcePatch{})).
					Return(nil, errors.ConflictState("Channel 'ch-123' is marked for deletion"))
			},
			expectedStatusCode: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, mockResourceSvc := newTestResourceHandler(ctrl)
			tt.setupMock(mockResourceSvc)

			req := httptest.NewRequest(http.MethodPatch,
				"/api/hyperfleet/v1/channels/"+tt.id, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"id": tt.id})
			rr := httptest.NewRecorder()

			handler.Patch(rr, req)
			Expect(rr.Code).To(Equal(tt.expectedStatusCode))
		})
	}
}

func TestResourceHandler_Delete(t *testing.T) {
	tests := []struct {
		setupMock          func(mock *services.MockResourceService)
		name               string
		id                 string
		expectedStatusCode int
	}{
		{
			name: "Success",
			id:   "ch-123",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Delete(gomock.Any(), "Channel", "ch-123").
					Return(&api.Resource{
						Meta: api.Meta{ID: "ch-123"},
						Kind: "Channel", Name: "stable",
						Spec:      datatypes.JSON(`{}`),
						CreatedBy: "u@test.com", UpdatedBy: "u@test.com",
					}, nil)
			},
			expectedStatusCode: http.StatusAccepted,
		},
		{
			name: "Not found",
			id:   "nonexistent",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Delete(gomock.Any(), "Channel", "nonexistent").
					Return(nil, errors.NotFound("Channel not found"))
			},
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name: "Conflict 409 - has active children",
			id:   "ch-123",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Delete(gomock.Any(), "Channel", "ch-123").
					Return(nil, errors.Conflict("cannot delete Channel 'ch-123': active Version children exist"))
			},
			expectedStatusCode: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, mockResourceSvc := newTestResourceHandler(ctrl)
			tt.setupMock(mockResourceSvc)

			req := httptest.NewRequest(http.MethodDelete,
				"/api/hyperfleet/v1/channels/"+tt.id, nil)
			req = mux.SetURLVars(req, map[string]string{"id": tt.id})
			rr := httptest.NewRecorder()

			handler.Delete(rr, req)
			Expect(rr.Code).To(Equal(tt.expectedStatusCode))
		})
	}
}

func TestResourceHandler_CreateWithOwner(t *testing.T) {
	now := time.Now()

	tests := []struct {
		setupMock          func(mock *services.MockResourceService)
		name               string
		body               string
		expectedStatusCode int
	}{
		{
			name: "Success",
			body: `{"kind":"Version","name":"4-17-3","spec":{"raw_version":"4.17.3"}}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Get(gomock.Any(), "Channel", "ch-1").Return(&api.Resource{
					Meta: api.Meta{ID: "ch-1"}, Kind: "Channel", Name: "stable",
					Href: "/api/hyperfleet/v1/channels/ch-1",
					Spec: datatypes.JSON(`{}`), CreatedBy: "u@t.com", UpdatedBy: "u@t.com",
				}, nil)
				mock.EXPECT().Create(gomock.Any(), "Version", gomock.AssignableToTypeOf(&api.Resource{}), gomock.Any()).
					Return(&api.Resource{
						Meta: api.Meta{ID: "v-1", CreatedTime: now, UpdatedTime: now},
						Kind: "Version", Name: "4-17-3",
						Spec: datatypes.JSON(`{"raw_version":"4.17.3"}`), Generation: 1,
						CreatedBy: "u@t.com", UpdatedBy: "u@t.com",
					}, nil)
			},
			expectedStatusCode: http.StatusCreated,
		},
		{
			name: "Parent not found",
			body: `{"kind":"Version","name":"4-17-3","spec":{"raw_version":"4.17.3"}}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Get(gomock.Any(), "Channel", "ch-1").
					Return(nil, errors.NotFound("Channel not found"))
			},
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name: "Conflict 409 - parent soft-deleted",
			body: `{"kind":"Version","name":"4-17-3","spec":{"raw_version":"4.17.3"}}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Get(gomock.Any(), "Channel", "ch-1").Return(&api.Resource{
					Meta: api.Meta{ID: "ch-1"}, Kind: "Channel", Name: "stable",
					Href: "/api/hyperfleet/v1/channels/ch-1",
					Spec: datatypes.JSON(`{}`), CreatedBy: "u@t.com", UpdatedBy: "u@t.com",
				}, nil)
				mock.EXPECT().Create(gomock.Any(), "Version", gomock.AssignableToTypeOf(&api.Resource{}), gomock.Any()).
					Return(nil, errors.ConflictState("Channel 'ch-1' is marked for deletion"))
			},
			expectedStatusCode: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, mockSvc := newTestVersionHandler(ctrl)
			tt.setupMock(mockSvc)

			req := httptest.NewRequest(http.MethodPost,
				"/api/hyperfleet/v1/channels/ch-1/versions", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"parent_id": "ch-1"})
			rr := httptest.NewRecorder()

			handler.Create(rr, req)
			Expect(rr.Code).To(Equal(tt.expectedStatusCode))
		})
	}
}

func TestResourceHandler_GetByOwner(t *testing.T) {
	now := time.Now()

	tests := []struct {
		setupMock          func(mock *services.MockResourceService)
		name               string
		expectedStatusCode int
	}{
		{
			name: "Success",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Get(gomock.Any(), "Channel", "ch-1").Return(&api.Resource{
					Meta: api.Meta{ID: "ch-1", CreatedTime: now, UpdatedTime: now},
					Kind: "Channel", Name: "stable",
					Spec: datatypes.JSON(`{}`), CreatedBy: "u@t.com", UpdatedBy: "u@t.com",
				}, nil)
				mock.EXPECT().GetByOwner(gomock.Any(), "Version", "v-1", "ch-1").Return(&api.Resource{
					Meta: api.Meta{ID: "v-1", CreatedTime: now, UpdatedTime: now},
					Kind: "Version", Name: "4-17-3",
					Spec: datatypes.JSON(`{}`), CreatedBy: "u@t.com", UpdatedBy: "u@t.com",
				}, nil)
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Child not found",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Get(gomock.Any(), "Channel", "ch-1").Return(&api.Resource{
					Meta: api.Meta{ID: "ch-1", CreatedTime: now, UpdatedTime: now},
					Kind: "Channel", Name: "stable",
					Spec: datatypes.JSON(`{}`), CreatedBy: "u@t.com", UpdatedBy: "u@t.com",
				}, nil)
				mock.EXPECT().GetByOwner(gomock.Any(), "Version", "v-1", "ch-1").
					Return(nil, errors.NotFound("Version not found"))
			},
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name: "Parent not found",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Get(gomock.Any(), "Channel", "ch-1").
					Return(nil, errors.NotFound("Channel not found"))
			},
			expectedStatusCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, mockSvc := newTestVersionHandler(ctrl)
			tt.setupMock(mockSvc)

			req := httptest.NewRequest(http.MethodGet,
				"/api/hyperfleet/v1/channels/ch-1/versions/v-1", nil)
			req = mux.SetURLVars(req, map[string]string{"parent_id": "ch-1", "id": "v-1"})
			rr := httptest.NewRecorder()

			handler.Get(rr, req)
			Expect(rr.Code).To(Equal(tt.expectedStatusCode))
		})
	}
}

func TestResourceHandler_ListByOwner(t *testing.T) {
	now := time.Now()

	tests := []struct {
		setupMock          func(mock *services.MockResourceService)
		name               string
		expectedStatusCode int
		expectedItems      int
	}{
		{
			name: "Success",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Get(gomock.Any(), "Channel", "ch-1").Return(&api.Resource{
					Meta: api.Meta{ID: "ch-1", CreatedTime: now, UpdatedTime: now},
					Kind: "Channel", Name: "stable",
					Spec: datatypes.JSON(`{}`), CreatedBy: "u@t.com", UpdatedBy: "u@t.com",
				}, nil)
				mock.EXPECT().ListByOwner(gomock.Any(), "Version", "ch-1",
					gomock.AssignableToTypeOf(&services.ListArguments{})).
					Return(api.ResourceList{
						&api.Resource{
							Meta: api.Meta{ID: "v-1", CreatedTime: now, UpdatedTime: now},
							Kind: "Version", Name: "4-17-3",
							Spec: datatypes.JSON(`{}`), CreatedBy: "u@t.com", UpdatedBy: "u@t.com",
						},
					}, &api.PagingMeta{Page: 1, Size: 1, Total: 1}, nil)
			},
			expectedStatusCode: http.StatusOK,
			expectedItems:      1,
		},
		{
			name: "Parent not found",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Get(gomock.Any(), "Channel", "ch-1").
					Return(nil, errors.NotFound("Channel not found"))
			},
			expectedStatusCode: http.StatusNotFound,
			expectedItems:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, mockSvc := newTestVersionHandler(ctrl)
			tt.setupMock(mockSvc)

			req := httptest.NewRequest(http.MethodGet,
				"/api/hyperfleet/v1/channels/ch-1/versions", nil)
			req = mux.SetURLVars(req, map[string]string{"parent_id": "ch-1"})
			rr := httptest.NewRecorder()

			handler.List(rr, req)
			Expect(rr.Code).To(Equal(tt.expectedStatusCode))
		})
	}
}

func TestResourceHandler_PatchByOwner(t *testing.T) {
	now := time.Now()

	tests := []struct {
		setupMock          func(mock *services.MockResourceService)
		name               string
		expectedStatusCode int
	}{
		{
			name: "Success",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().GetByOwner(gomock.Any(), "Version", "v-1", "ch-1").
					Return(&api.Resource{Meta: api.Meta{ID: "v-1"}, Kind: "Version",
						Spec: datatypes.JSON(`{}`), CreatedBy: "u@t.com", UpdatedBy: "u@t.com"}, nil)
				mock.EXPECT().Patch(gomock.Any(), "Version", "v-1",
					gomock.AssignableToTypeOf(&api.ResourcePatch{})).
					Return(&api.Resource{
						Meta: api.Meta{ID: "v-1", CreatedTime: now, UpdatedTime: now},
						Kind: "Version", Name: "4-17-3", Generation: 2,
						Spec: datatypes.JSON(`{"enabled":false}`), CreatedBy: "u@t.com", UpdatedBy: "u@t.com",
					}, nil)
			},
			expectedStatusCode: http.StatusOK,
		},
		{
			name: "Child not owned by parent",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().GetByOwner(gomock.Any(), "Version", "v-1", "ch-1").
					Return(nil, errors.NotFound("Version not found for channel"))
			},
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name: "Conflict 409 - soft-deleted child",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().GetByOwner(gomock.Any(), "Version", "v-1", "ch-1").
					Return(&api.Resource{Meta: api.Meta{ID: "v-1"}, Kind: "Version",
						Spec: datatypes.JSON(`{}`), CreatedBy: "u@t.com", UpdatedBy: "u@t.com"}, nil)
				mock.EXPECT().Patch(gomock.Any(), "Version", "v-1",
					gomock.AssignableToTypeOf(&api.ResourcePatch{})).
					Return(nil, errors.ConflictState("Version 'v-1' is marked for deletion"))
			},
			expectedStatusCode: http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, mockSvc := newTestVersionHandler(ctrl)
			tt.setupMock(mockSvc)

			req := httptest.NewRequest(http.MethodPatch,
				"/api/hyperfleet/v1/channels/ch-1/versions/v-1",
				strings.NewReader(`{"spec":{"enabled":false}}`))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"parent_id": "ch-1", "id": "v-1"})
			rr := httptest.NewRecorder()

			handler.Patch(rr, req)
			Expect(rr.Code).To(Equal(tt.expectedStatusCode))
		})
	}
}

func TestResourceHandler_DeleteByOwner(t *testing.T) {
	tests := []struct {
		setupMock          func(mock *services.MockResourceService)
		name               string
		expectedStatusCode int
	}{
		{
			name: "Success",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().GetByOwner(gomock.Any(), "Version", "v-1", "ch-1").
					Return(&api.Resource{Meta: api.Meta{ID: "v-1"}, Kind: "Version",
						Spec: datatypes.JSON(`{}`), CreatedBy: "u@t.com", UpdatedBy: "u@t.com"}, nil)
				mock.EXPECT().Delete(gomock.Any(), "Version", "v-1").
					Return(&api.Resource{
						Meta: api.Meta{ID: "v-1"}, Kind: "Version", Name: "4-17-3",
						Spec: datatypes.JSON(`{}`), CreatedBy: "u@t.com", UpdatedBy: "u@t.com",
					}, nil)
			},
			expectedStatusCode: http.StatusAccepted,
		},
		{
			name: "Child not owned by parent",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().GetByOwner(gomock.Any(), "Version", "v-1", "ch-1").
					Return(nil, errors.NotFound("Version not found for channel"))
			},
			expectedStatusCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, mockSvc := newTestVersionHandler(ctrl)
			tt.setupMock(mockSvc)

			req := httptest.NewRequest(http.MethodDelete,
				"/api/hyperfleet/v1/channels/ch-1/versions/v-1", nil)
			req = mux.SetURLVars(req, map[string]string{"parent_id": "ch-1", "id": "v-1"})
			rr := httptest.NewRecorder()

			handler.Delete(rr, req)
			Expect(rr.Code).To(Equal(tt.expectedStatusCode))
		})
	}
}

func TestResourceHandler_ForceDelete(t *testing.T) {
	RegisterTestingT(t)

	resourceID := "ch-123"

	tests := []struct {
		setupMock          func(mock *services.MockResourceService)
		name               string
		body               string
		expectedStatusCode int
	}{
		{
			name: "Success 204 - resource force-deleted",
			body: `{"reason": "Stuck in finalizing for 2 hours"}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().
					ForceDelete(gomock.Any(), "Channel", resourceID, "Stuck in finalizing for 2 hours").
					Return(nil)
			},
			expectedStatusCode: http.StatusNoContent,
		},
		{
			name: "Error 400 - malformed JSON",
			body: `not json`,
			setupMock: func(mock *services.MockResourceService) {
			},
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			// Wrong JSON type for "reason" must return a clean message without leaking
			// the Go struct name (ForceDeleteRequest) — HYPERFLEET-1376.
			name: "Error 400 - reason wrong type",
			body: `{"reason": 123}`,
			setupMock: func(mock *services.MockResourceService) {
			},
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name: "Error 400 - empty reason",
			body: `{"reason": ""}`,
			setupMock: func(mock *services.MockResourceService) {
			},
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name: "Error 400 - reason exceeds max length",
			body: `{"reason": "` + strings.Repeat("x", maxReasonLength+1) + `"}`,
			setupMock: func(mock *services.MockResourceService) {
			},
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name: "Error 404 - resource not found",
			body: `{"reason": "some reason"}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().
					ForceDelete(gomock.Any(), "Channel", resourceID, "some reason").
					Return(errors.NotFound("Channel with id='%s' not found", resourceID))
			},
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name: "Error 409 - resource not in Finalizing state",
			body: `{"reason": "some reason"}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().
					ForceDelete(gomock.Any(), "Channel", resourceID, "some reason").
					Return(errors.ConflictState("Channel '%s' is not in Finalizing state", resourceID))
			},
			expectedStatusCode: http.StatusConflict,
		},
		{
			name: "Error 500 - service internal error",
			body: `{"reason": "some reason"}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().
					ForceDelete(gomock.Any(), "Channel", resourceID, "some reason").
					Return(errors.GeneralError("database connection lost"))
			},
			expectedStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, mockSvc := newTestResourceHandler(ctrl)
			tt.setupMock(mockSvc)

			reqURL := "/api/hyperfleet/v1/channels/" + resourceID + "/force-delete"
			req := httptest.NewRequest(http.MethodPost, reqURL, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"id": resourceID})

			rr := httptest.NewRecorder()
			handler.ForceDelete(rr, req)

			Expect(rr.Code).To(Equal(tt.expectedStatusCode))

			if tt.expectedStatusCode == http.StatusNoContent {
				Expect(rr.Body.Len()).To(Equal(0))
			}
			if tt.name == "Error 400 - reason wrong type" {
				Expect(rr.Body.String()).To(ContainSubstring("field 'reason' must be a string"))
				Expect(rr.Body.String()).ToNot(ContainSubstring("ForceDeleteRequest"))
			}
		})
	}
}

func TestResourceHandler_ForceDeleteByOwner(t *testing.T) {
	RegisterTestingT(t)

	parentID := "ch-1"
	versionID := "v-1"

	tests := []struct {
		setupMock          func(mock *services.MockResourceService)
		name               string
		body               string
		expectedStatusCode int
	}{
		{
			name: "Success 204 - nested resource force-deleted",
			body: `{"reason": "Stuck in finalizing"}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().
					GetByOwner(gomock.Any(), "Version", versionID, parentID).
					Return(&api.Resource{Meta: api.Meta{ID: versionID}, Kind: "Version"}, nil)
				mock.EXPECT().
					ForceDelete(gomock.Any(), "Version", versionID, "Stuck in finalizing").
					Return(nil)
			},
			expectedStatusCode: http.StatusNoContent,
		},
		{
			name: "Error 404 - ownership mismatch",
			body: `{"reason": "some reason"}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().
					GetByOwner(gomock.Any(), "Version", versionID, parentID).
					Return(nil, errors.NotFound("Version with id='%s' not found for owner '%s'", versionID, parentID))
			},
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name: "Error 400 - empty reason",
			body: `{"reason": ""}`,
			setupMock: func(mock *services.MockResourceService) {
			},
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name: "Error 409 - not in Finalizing state",
			body: `{"reason": "some reason"}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().
					GetByOwner(gomock.Any(), "Version", versionID, parentID).
					Return(&api.Resource{Meta: api.Meta{ID: versionID}, Kind: "Version"}, nil)
				mock.EXPECT().
					ForceDelete(gomock.Any(), "Version", versionID, "some reason").
					Return(errors.ConflictState("Version '%s' is not in Finalizing state", versionID))
			},
			expectedStatusCode: http.StatusConflict,
		},
		{
			name: "Error 500 - service internal error",
			body: `{"reason": "some reason"}`,
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().
					GetByOwner(gomock.Any(), "Version", versionID, parentID).
					Return(&api.Resource{Meta: api.Meta{ID: versionID}, Kind: "Version"}, nil)
				mock.EXPECT().
					ForceDelete(gomock.Any(), "Version", versionID, "some reason").
					Return(errors.GeneralError("database connection lost"))
			},
			expectedStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, mockSvc := newTestVersionHandler(ctrl)
			tt.setupMock(mockSvc)

			reqURL := "/api/hyperfleet/v1/channels/" + parentID + "/versions/" + versionID + "/force-delete"
			req := httptest.NewRequest(http.MethodPost, reqURL, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"parent_id": parentID, "id": versionID})

			rr := httptest.NewRecorder()
			handler.ForceDelete(rr, req)

			Expect(rr.Code).To(Equal(tt.expectedStatusCode))

			if tt.expectedStatusCode == http.StatusNoContent {
				Expect(rr.Body.Len()).To(Equal(0))
			}
		})
	}
}

func TestResourceHandler_Patch_RejectsUnknownFields(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		body string
	}{
		{"rejects name", `{"name":"new-name","spec":{"is_default":true}}`},
		{"rejects id", `{"id":"some-id","spec":{"is_default":true}}`},
		{"rejects generation", `{"generation":5,"spec":{"is_default":true}}`},
		{"rejects kind", `{"kind":"Channel","spec":{"is_default":true}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, _ := newTestResourceHandler(ctrl)

			reqURL := "/api/hyperfleet/v1/channels/ch-123"
			req := httptest.NewRequest(http.MethodPatch, reqURL, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"id": "ch-123"})

			rr := httptest.NewRecorder()
			handler.Patch(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
		})
	}
}

// TestResourceHandler_Patch_LabelsTypeMismatch verifies that a wrong JSON type for
// "labels" returns a clean validation message without leaking the Go struct name
// (e.g. "ResourcePatchRequest") or type (e.g. "map[string]string") — HYPERFLEET-1376
// findings 5 & 6.
func TestResourceHandler_Patch_LabelsTypeMismatch(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		body string
	}{
		{"labels as string", `{"labels":"not-an-object"}`},
		{"labels as array", `{"labels":["a","b"]}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, _ := newTestResourceHandler(ctrl)

			reqURL := "/api/hyperfleet/v1/channels/ch-123"
			req := httptest.NewRequest(http.MethodPatch, reqURL, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"id": "ch-123"})

			rr := httptest.NewRecorder()
			handler.Patch(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			Expect(rr.Body.String()).To(ContainSubstring("field 'labels' must be an object"))
			Expect(rr.Body.String()).ToNot(ContainSubstring("ResourcePatchRequest"))
			Expect(rr.Body.String()).ToNot(ContainSubstring("openapi."))
		})
	}
}

func TestResourceHandler_PatchByOwner_RejectsUnknownFields(t *testing.T) {
	RegisterTestingT(t)

	tests := []struct {
		name string
		body string
	}{
		{"rejects name", `{"name":"new-name","spec":{"enabled":true}}`},
		{"rejects id", `{"id":"some-id","spec":{"enabled":true}}`},
		{"rejects generation", `{"generation":5,"spec":{"enabled":true}}`},
		{"rejects kind", `{"kind":"Version","spec":{"enabled":true}}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, _ := newTestVersionHandler(ctrl)

			reqURL := "/api/hyperfleet/v1/channels/ch-1/versions/v-1"
			req := httptest.NewRequest(http.MethodPatch, reqURL, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"parent_id": "ch-1", "id": "v-1"})

			rr := httptest.NewRecorder()
			handler.Patch(rr, req)

			Expect(rr.Code).To(Equal(http.StatusBadRequest))
		})
	}
}

func TestResourceHandler_Get_WithFieldsFilter(t *testing.T) {
	now := time.Now()

	tests := []struct {
		setupMock          func(mock *services.MockResourceService)
		validateResponse   func(body []byte)
		name               string
		queryParams        string
		expectedStatusCode int
	}{
		{
			name:        "Success - Filter to id, name, kind",
			queryParams: "?fields=id,name,kind",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Get(gomock.Any(), "Channel", "ch-123").Return(&api.Resource{
					Meta:       api.Meta{ID: "ch-123", CreatedTime: now, UpdatedTime: now},
					Kind:       "Channel",
					Name:       "stable",
					Spec:       datatypes.JSON(`{"region":"us-east-1"}`),
					Generation: 3,
					CreatedBy:  "user@test.com",
					UpdatedBy:  "user@test.com",
				}, nil)
			},
			expectedStatusCode: http.StatusOK,
			validateResponse: func(body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				Expect(err).NotTo(HaveOccurred())

				Expect(response).To(HaveKey("id"))
				Expect(response).To(HaveKey("name"))
				Expect(response).To(HaveKey("kind"))
				Expect(response["name"]).To(Equal("stable"))
				Expect(response["kind"]).To(Equal("Channel"))

				Expect(response).ToNot(HaveKey("generation"))
				Expect(response).ToNot(HaveKey("spec"))
				Expect(response).ToNot(HaveKey("labels"))
				Expect(response).ToNot(HaveKey("created_time"))
			},
		},
		{
			name:        "Error 400 - Invalid field name",
			queryParams: "?fields=invalid_field",
			setupMock: func(mock *services.MockResourceService) {
				mock.EXPECT().Get(gomock.Any(), "Channel", "ch-123").Return(&api.Resource{
					Meta:      api.Meta{ID: "ch-123", CreatedTime: now, UpdatedTime: now},
					Kind:      "Channel",
					Name:      "stable",
					Spec:      datatypes.JSON(`{}`),
					CreatedBy: "user@test.com",
					UpdatedBy: "user@test.com",
				}, nil)
			},
			expectedStatusCode: http.StatusBadRequest,
			validateResponse: func(body []byte) {
				var errResp openapi.ProblemDetails
				err := json.Unmarshal(body, &errResp)
				Expect(err).NotTo(HaveOccurred())
				Expect(errResp.Status).To(Equal(http.StatusBadRequest))
				Expect(*errResp.Detail).To(ContainSubstring("doesn't exist"))
				Expect(*errResp.Detail).To(ContainSubstring("invalid_field"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, mockResourceSvc := newTestResourceHandler(ctrl)
			tt.setupMock(mockResourceSvc)

			reqURL := "/api/hyperfleet/v1/channels/ch-123" + tt.queryParams
			req := httptest.NewRequest(http.MethodGet, reqURL, nil)
			req = mux.SetURLVars(req, map[string]string{"id": "ch-123"})
			rr := httptest.NewRecorder()

			handler.Get(rr, req)
			Expect(rr.Code).To(Equal(tt.expectedStatusCode))
			tt.validateResponse(rr.Body.Bytes())
		})
	}
}

func TestRootResourceHandler_Create_RejectsInvalidName(t *testing.T) {
	tests := []struct {
		name       string
		inputName  string
		wantStatus int
	}{
		{"too short for Cluster", "ab", http.StatusBadRequest},
		{"too long for Cluster", strings.Repeat("a", 54), http.StatusBadRequest},
		{"uppercase rejected", "InvalidName", http.StatusBadRequest},
		{"valid name accepted", "my-cluster", http.StatusCreated},
	}
	// Register Cluster with name constraints so the root handler can resolve it.
	registry.Register(registry.EntityDescriptor{
		Kind:       "Cluster",
		Plural:     "clusters",
		NameMinLen: 3,
		NameMaxLen: 53,
	})
	t.Cleanup(func() { registry.Reset() })

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			handler, mockSvc, _ := newTestRootResourceHandler(ctrl)

			if tt.wantStatus == http.StatusCreated {
				now := time.Now()
				mockSvc.EXPECT().Create(
					gomock.Any(), "Cluster", gomock.AssignableToTypeOf(&api.Resource{}), gomock.Any(),
				).Return(&api.Resource{
					Meta:       api.Meta{ID: "c-1", CreatedTime: now, UpdatedTime: now},
					Kind:       "Cluster",
					Name:       tt.inputName,
					Href:       "/api/hyperfleet/v1/clusters/c-1",
					Spec:       []byte(`{"region":"us-east-1"}`),
					Generation: 1,
					CreatedBy:  "system@hyperfleet.local",
					UpdatedBy:  "system@hyperfleet.local",
				}, nil)
			}

			body := fmt.Sprintf(`{"kind":"Cluster","name":%q,"spec":{"region":"us-east-1"}}`, tt.inputName)
			req := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/resources", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()

			handler.Create(rr, req)
			Expect(rr.Code).To(Equal(tt.wantStatus))
		})
	}
}
