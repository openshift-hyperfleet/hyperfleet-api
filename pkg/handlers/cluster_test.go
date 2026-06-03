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

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

func TestClusterHandler_Patch(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	clusterID := testClusterID
	validBody := `{"spec":{"region":"us-east1"}}`

	tests := []struct {
		setupMocks         func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockGenericService)
		name               string
		clusterID          string
		expectedStatusCode int
		expectedError      bool
	}{
		{
			name:      "Success - Patch active cluster",
			clusterID: clusterID,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockGenericService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Patch(gomock.Any(), clusterID, gomock.Any()).Return(&api.Cluster{
					Meta:             api.Meta{ID: clusterID, CreatedTime: now, UpdatedTime: now},
					Name:             "test-cluster",
					Spec:             []byte(`{"region":"us-east1"}`),
					Labels:           []byte("{}"),
					StatusConditions: []byte("[]"),
					CreatedBy:        testSystemUser,
					UpdatedBy:        testSystemUser,
				}, nil)

				return mockClusterSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusOK,
			expectedError:      false,
		},
		{
			name:      "Error 409 - Cluster is soft-deleted",
			clusterID: clusterID,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockGenericService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Patch(gomock.Any(), clusterID, gomock.Any()).
					Return(nil, errors.ConflictState("Cluster '%s' is marked for deletion", clusterID))

				return mockClusterSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusConflict,
			expectedError:      true,
		},
		{
			name:      "Error 404 - Cluster not found",
			clusterID: "non-existent",
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockGenericService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Patch(gomock.Any(), "non-existent", gomock.Any()).
					Return(nil, errors.NotFound("Cluster not found"))

				return mockClusterSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusNotFound,
			expectedError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClusterSvc, mockGenericSvc := tt.setupMocks(ctrl)
			handler := NewClusterHandler(mockClusterSvc, mockGenericSvc)

			reqURL := "/api/hyperfleet/v1/clusters/" + tt.clusterID
			req := httptest.NewRequest(http.MethodPatch, reqURL, strings.NewReader(validBody))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{
				"id": tt.clusterID,
			})

			rr := httptest.NewRecorder()
			handler.Patch(rr, req)

			Expect(rr.Code).To(Equal(tt.expectedStatusCode))

			if !tt.expectedError {
				var response openapi.Cluster
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(*response.Id).To(Equal(clusterID))
			}

			if tt.expectedStatusCode == http.StatusConflict {
				var errResp openapi.ProblemDetails
				err := json.Unmarshal(rr.Body.Bytes(), &errResp)
				Expect(err).NotTo(HaveOccurred())
				Expect(errResp.Type).To(Equal(errors.ErrorTypeConflict))
				Expect(errResp.Status).To(Equal(http.StatusConflict))
				Expect(*errResp.Detail).To(ContainSubstring("marked for deletion"))
				Expect(*errResp.Code).To(Equal("HYPERFLEET-CNF-003"))
			}
		})
	}
}

func TestClusterHandler_ForceDelete(t *testing.T) {
	RegisterTestingT(t)

	clusterID := testClusterID

	tests := []struct {
		setupMocks         func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockGenericService)
		name               string
		body               string
		expectedStatusCode int
	}{
		{
			name: "Success 204 - cluster force-deleted",
			body: `{"reason": "Stuck in finalizing for 2 hours"}`,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockGenericService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)
				mockClusterSvc.EXPECT().
					ForceDelete(gomock.Any(), clusterID, "Stuck in finalizing for 2 hours").
					Return(nil)
				return mockClusterSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusNoContent,
		},
		{
			name: "Error 400 - malformed JSON",
			body: `not json`,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockGenericService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)
				return mockClusterSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name: "Error 400 - empty reason",
			body: `{"reason": ""}`,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockGenericService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)
				return mockClusterSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name: "Error 400 - reason exceeds max length",
			body: `{"reason": "` + strings.Repeat("x", maxReasonLength+1) + `"}`,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockGenericService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)
				return mockClusterSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name: "Error 404 - cluster not found",
			body: `{"reason": "some reason"}`,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockGenericService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)
				mockClusterSvc.EXPECT().
					ForceDelete(gomock.Any(), clusterID, "some reason").
					Return(errors.NotFound("Cluster with id='%s' not found", clusterID))
				return mockClusterSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name: "Error 409 - cluster not in Finalizing state",
			body: `{"reason": "some reason"}`,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockGenericService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)
				mockClusterSvc.EXPECT().
					ForceDelete(gomock.Any(), clusterID, "some reason").
					Return(errors.ConflictState("Cluster '%s' is not in Finalizing state", clusterID))
				return mockClusterSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusConflict,
		},
		{
			name: "Error 500 - service internal error",
			body: `{"reason": "some reason"}`,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockGenericService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)
				mockClusterSvc.EXPECT().
					ForceDelete(gomock.Any(), clusterID, "some reason").
					Return(errors.GeneralError("database connection lost"))
				return mockClusterSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClusterSvc, mockGenericSvc := tt.setupMocks(ctrl)
			handler := NewClusterHandler(mockClusterSvc, mockGenericSvc)

			reqURL := "/api/hyperfleet/v1/clusters/" + clusterID + "/force-delete"
			req := httptest.NewRequest(http.MethodPost, reqURL, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{"id": clusterID})

			rr := httptest.NewRecorder()
			handler.ForceDelete(rr, req)

			Expect(rr.Code).To(Equal(tt.expectedStatusCode))

			if tt.expectedStatusCode == http.StatusNoContent {
				Expect(rr.Body.Len()).To(Equal(0))
			}
		})
	}
}
