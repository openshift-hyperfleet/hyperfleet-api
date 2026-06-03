package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/mux"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

const (
	testClusterID  = "test-cluster-id"
	testNodePoolID = "test-nodepool-id"
	testSystemUser = "system@hyperfleet.local"
)

func TestClusterNodePoolsHandler_List(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	clusterUUID := uuid.New().String()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockClusterSvc := services.NewMockClusterService(ctrl)
	mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
	mockNodePoolSvc.EXPECT().ListByCluster(gomock.Any(), clusterUUID, gomock.Any()).
		Return(api.NodePoolList{
			&api.NodePool{
				Meta:             api.Meta{ID: "np-1", CreatedTime: now, UpdatedTime: now},
				Name:             "test-np",
				OwnerID:          clusterUUID,
				OwnerKind:        "Cluster",
				Spec:             []byte(`{}`),
				Labels:           []byte(`{}`),
				StatusConditions: []byte(`[]`),
				CreatedBy:        testSystemUser,
				UpdatedBy:        testSystemUser,
			},
		}, &api.PagingMeta{Page: 1, Size: 1, Total: 1}, nil)

	handler := NewClusterNodePoolsHandler(mockClusterSvc, mockNodePoolSvc)

	req := httptest.NewRequest(http.MethodGet,
		"/api/hyperfleet/v1/clusters/"+clusterUUID+"/nodepools", nil)
	req = mux.SetURLVars(req, map[string]string{"id": clusterUUID})
	rr := httptest.NewRecorder()

	handler.List(rr, req)
	Expect(rr.Code).To(Equal(http.StatusOK))

	var raw map[string]interface{}
	Expect(json.Unmarshal(rr.Body.Bytes(), &raw)).To(Succeed())
	Expect(raw).NotTo(HaveKey("kind"))
}

func TestClusterNodePoolsHandler_Get(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	clusterID := testClusterID
	nodePoolID := testNodePoolID

	tests := []struct {
		setupMocks func(ctrl *gomock.Controller) (
			*services.MockClusterService, *services.MockNodePoolService,
		)
		name               string
		clusterID          string
		nodePoolID         string
		expectedStatusCode int
		expectedError      bool
	}{
		{
			name:       "Success - Get nodepool by cluster and nodepool ID",
			clusterID:  clusterID,
			nodePoolID: nodePoolID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)

				mockNodePoolSvc.EXPECT().GetByIDAndOwner(gomock.Any(), nodePoolID, clusterID).Return(&api.NodePool{
					Meta: api.Meta{
						ID:          nodePoolID,
						CreatedTime: now,
						UpdatedTime: now,
					},
					Kind:             "NodePool",
					Name:             "test-nodepool",
					OwnerID:          clusterID,
					Spec:             []byte("{}"),
					Labels:           []byte("{}"),
					StatusConditions: []byte("[]"),
					CreatedBy:        "user@example.com",
					UpdatedBy:        "user@example.com",
				}, nil)

				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusOK,
			expectedError:      false,
		},
		{
			name:       "Error - NodePool not found for cluster",
			clusterID:  clusterID,
			nodePoolID: "non-existent",
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)

				mockNodePoolSvc.EXPECT().GetByIDAndOwner(gomock.Any(), "non-existent", clusterID).
					Return(nil, errors.NotFound("NodePool not found"))

				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusNotFound,
			expectedError:      true,
		},
		{
			name:       "Error - NodePool belongs to different cluster",
			clusterID:  clusterID,
			nodePoolID: nodePoolID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)

				mockNodePoolSvc.EXPECT().GetByIDAndOwner(gomock.Any(), nodePoolID, clusterID).
					Return(nil, errors.NotFound("NodePool '%s' not found for cluster '%s'", nodePoolID, clusterID))

				return mockClusterSvc, mockNodePoolSvc
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

			mockClusterSvc, mockNodePoolSvc := tt.setupMocks(ctrl)
			handler := NewClusterNodePoolsHandler(mockClusterSvc, mockNodePoolSvc)

			reqURL := "/api/hyperfleet/v1/clusters/" + tt.clusterID + "/nodepools/" + tt.nodePoolID
			req := httptest.NewRequest(http.MethodGet, reqURL, nil)
			req = mux.SetURLVars(req, map[string]string{
				"id":          tt.clusterID,
				"nodepool_id": tt.nodePoolID,
			})

			rr := httptest.NewRecorder()
			handler.Get(rr, req)

			Expect(rr.Code).To(Equal(tt.expectedStatusCode))

			if !tt.expectedError {
				var response openapi.NodePool
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(*response.Id).To(Equal(nodePoolID))
				Expect(response.Kind).NotTo(BeNil())
				Expect(*response.Kind).To(Equal("NodePool"))
			}
		})
	}
}

func TestClusterNodePoolsHandler_SoftDelete(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	clusterID := testClusterID
	nodePoolID := testNodePoolID

	tests := []struct {
		setupMocks func(ctrl *gomock.Controller) (
			*services.MockClusterService, *services.MockNodePoolService,
		)
		name               string
		clusterID          string
		nodePoolID         string
		expectedStatusCode int
		expectedError      bool
	}{
		{
			name:       "given cluster exists and nodepool is owned by it, when deleted, then returns 202 with nodepool body",
			clusterID:  clusterID,
			nodePoolID: nodePoolID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)

				mockNodePoolSvc.EXPECT().GetByIDAndOwner(gomock.Any(), nodePoolID, clusterID).Return(&api.NodePool{
					Meta:    api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
					OwnerID: clusterID,
				}, nil)

				deletedTime := now
				deletedByUser := testSystemUser
				mockNodePoolSvc.EXPECT().SoftDelete(gomock.Any(), nodePoolID).Return(&api.NodePool{
					Meta:             api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
					Kind:             "NodePool",
					Name:             "test-nodepool",
					OwnerID:          clusterID,
					DeletedTime:      &deletedTime,
					DeletedBy:        &deletedByUser,
					Spec:             []byte("{}"),
					Labels:           []byte("{}"),
					StatusConditions: []byte("[]"),
					CreatedBy:        "user@example.com",
					UpdatedBy:        "user@example.com",
				}, nil)

				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusAccepted,
			expectedError:      false,
		},
		{
			name:       "given nodepool belongs to a different cluster, when deleted, then returns 404",
			clusterID:  clusterID,
			nodePoolID: nodePoolID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)

				mockNodePoolSvc.EXPECT().GetByIDAndOwner(gomock.Any(), nodePoolID, clusterID).
					Return(nil, errors.NotFound("NodePool not found for cluster"))

				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusNotFound,
			expectedError:      true,
		},
		{
			name:       "given nodepool does not exist, when deleted, then returns 404",
			clusterID:  clusterID,
			nodePoolID: "non-existent",
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)

				mockNodePoolSvc.EXPECT().GetByIDAndOwner(gomock.Any(), "non-existent", clusterID).
					Return(nil, errors.NotFound("NodePool not found"))

				return mockClusterSvc, mockNodePoolSvc
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

			mockClusterSvc, mockNodePoolSvc := tt.setupMocks(ctrl)
			handler := NewClusterNodePoolsHandler(mockClusterSvc, mockNodePoolSvc)

			reqURL := "/api/hyperfleet/v1/clusters/" + tt.clusterID + "/nodepools/" + tt.nodePoolID
			req := httptest.NewRequest(http.MethodDelete, reqURL, nil)
			req = mux.SetURLVars(req, map[string]string{
				"id":          tt.clusterID,
				"nodepool_id": tt.nodePoolID,
			})

			rr := httptest.NewRecorder()
			handler.SoftDelete(rr, req)

			Expect(rr.Code).To(Equal(tt.expectedStatusCode))

			if !tt.expectedError {
				var response openapi.NodePool
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(*response.Id).To(Equal(nodePoolID))
				Expect(response.DeletedTime).NotTo(BeNil())
				Expect(string(*response.DeletedBy)).To(Equal(testSystemUser))
			}
		})
	}
}

func TestClusterNodePoolsHandler_Create(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	clusterID := testClusterID
	nodePoolID := testNodePoolID
	validBody := `{"name":"test-np","kind":"NodePool","spec":{"replicas":1}}`

	tests := []struct {
		setupMocks func(ctrl *gomock.Controller) (
			*services.MockClusterService, *services.MockNodePoolService,
		)
		name               string
		clusterID          string
		expectedStatusCode int
		expectedError      bool
	}{
		{
			name:      "Success - Create nodepool for active cluster",
			clusterID: clusterID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)

				mockClusterSvc.EXPECT().Get(gomock.Any(), clusterID).Return(&api.Cluster{
					Meta: api.Meta{ID: clusterID, CreatedTime: now, UpdatedTime: now},
					Name: "test-cluster",
				}, nil)

				mockNodePoolSvc.EXPECT().Create(gomock.Any(), gomock.Any()).Return(&api.NodePool{
					Meta:             api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
					Kind:             "NodePool",
					Name:             "test-np",
					OwnerID:          clusterID,
					Spec:             []byte(`{"replicas":1}`),
					Labels:           []byte("{}"),
					StatusConditions: []byte("[]"),
					CreatedBy:        testSystemUser,
					UpdatedBy:        testSystemUser,
				}, nil)

				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusCreated,
			expectedError:      false,
		},
		{
			name:      "Error 409 - Cluster is soft-deleted",
			clusterID: clusterID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)

				deletedTime := now
				mockClusterSvc.EXPECT().Get(gomock.Any(), clusterID).Return(&api.Cluster{
					Meta:        api.Meta{ID: clusterID, CreatedTime: now, UpdatedTime: now},
					Name:        "test-cluster",
					DeletedTime: &deletedTime,
				}, nil)

				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusConflict,
			expectedError:      true,
		},
		{
			name:      "Error 404 - Cluster not found",
			clusterID: "non-existent",
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)

				mockClusterSvc.EXPECT().Get(gomock.Any(), "non-existent").Return(nil, errors.NotFound("Cluster not found"))

				return mockClusterSvc, mockNodePoolSvc
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

			mockClusterSvc, mockNodePoolSvc := tt.setupMocks(ctrl)
			handler := NewClusterNodePoolsHandler(mockClusterSvc, mockNodePoolSvc)

			reqURL := "/api/hyperfleet/v1/clusters/" + tt.clusterID + "/nodepools"
			req := httptest.NewRequest(http.MethodPost, reqURL, strings.NewReader(validBody))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{
				"id": tt.clusterID,
			})

			rr := httptest.NewRecorder()
			handler.Create(rr, req)

			Expect(rr.Code).To(Equal(tt.expectedStatusCode))

			if !tt.expectedError {
				var response openapi.NodePool
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(*response.Id).To(Equal(nodePoolID))
				Expect(response.Kind).NotTo(BeNil())
				Expect(*response.Kind).To(Equal("NodePool"))
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

func TestClusterNodePoolsHandler_Patch(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	clusterID := testClusterID
	nodePoolID := testNodePoolID
	validBody := `{"spec":{"replicas":2}}`

	tests := []struct {
		setupMocks func(ctrl *gomock.Controller) (
			*services.MockClusterService, *services.MockNodePoolService,
		)
		name               string
		clusterID          string
		nodePoolID         string
		expectedStatusCode int
		expectedError      bool
	}{
		{
			name:       "Success - Patch nodepool for active cluster",
			clusterID:  clusterID,
			nodePoolID: nodePoolID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)

				mockNodePoolSvc.EXPECT().GetByIDAndOwner(gomock.Any(), nodePoolID, clusterID).Return(&api.NodePool{
					Meta:    api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
					OwnerID: clusterID,
				}, nil)

				mockNodePoolSvc.EXPECT().Patch(gomock.Any(), nodePoolID, gomock.Any()).Return(&api.NodePool{
					Meta:             api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
					Kind:             "NodePool",
					Name:             "test-nodepool",
					OwnerID:          clusterID,
					Spec:             []byte(`{"replicas":2}`),
					Labels:           []byte("{}"),
					StatusConditions: []byte("[]"),
					CreatedBy:        "user@example.com",
					UpdatedBy:        "user@example.com",
				}, nil)

				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusOK,
			expectedError:      false,
		},
		{
			name:       "Error 409 - NodePool is soft-deleted",
			clusterID:  clusterID,
			nodePoolID: nodePoolID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)

				mockNodePoolSvc.EXPECT().GetByIDAndOwner(gomock.Any(), nodePoolID, clusterID).Return(&api.NodePool{
					Meta:    api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
					OwnerID: clusterID,
				}, nil)

				mockNodePoolSvc.EXPECT().Patch(gomock.Any(), nodePoolID, gomock.Any()).
					Return(nil, errors.ConflictState("NodePool '%s' is marked for deletion", nodePoolID))

				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusConflict,
			expectedError:      true,
		},
		{
			name:       "Error 404 - NodePool not found for cluster",
			clusterID:  clusterID,
			nodePoolID: "non-existent-np",
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)

				mockNodePoolSvc.EXPECT().GetByIDAndOwner(gomock.Any(), "non-existent-np", clusterID).
					Return(nil, errors.NotFound("NodePool not found for cluster"))

				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusNotFound,
			expectedError:      true,
		},
		{
			name:       "Error 404 - NodePool belongs to different cluster",
			clusterID:  clusterID,
			nodePoolID: nodePoolID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)

				mockNodePoolSvc.EXPECT().GetByIDAndOwner(gomock.Any(), nodePoolID, clusterID).
					Return(nil, errors.NotFound("NodePool '%s' not found for cluster '%s'", nodePoolID, clusterID))

				return mockClusterSvc, mockNodePoolSvc
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

			mockClusterSvc, mockNodePoolSvc := tt.setupMocks(ctrl)
			handler := NewClusterNodePoolsHandler(mockClusterSvc, mockNodePoolSvc)

			reqURL := "/api/hyperfleet/v1/clusters/" + tt.clusterID + "/nodepools/" + tt.nodePoolID
			req := httptest.NewRequest(http.MethodPatch, reqURL, strings.NewReader(validBody))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{
				"id":          tt.clusterID,
				"nodepool_id": tt.nodePoolID,
			})

			rr := httptest.NewRecorder()
			handler.Patch(rr, req)

			Expect(rr.Code).To(Equal(tt.expectedStatusCode))

			if !tt.expectedError {
				var response openapi.NodePool
				err := json.Unmarshal(rr.Body.Bytes(), &response)
				Expect(err).NotTo(HaveOccurred())
				Expect(*response.Id).To(Equal(nodePoolID))
				Expect(response.Kind).NotTo(BeNil())
				Expect(*response.Kind).To(Equal("NodePool"))
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

func TestClusterNodePoolsHandler_ForceDelete(t *testing.T) {
	RegisterTestingT(t)

	clusterID := testClusterID
	nodePoolID := testNodePoolID

	tests := []struct {
		setupMocks         func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockNodePoolService)
		name               string
		nodePoolID         string
		body               string
		expectedStatusCode int
	}{
		{
			name:       "Success 204 - nodepool force-deleted",
			nodePoolID: nodePoolID,
			body:       `{"reason": "Stuck in finalizing for 2 hours"}`,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockNodePoolService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockNodePoolSvc.EXPECT().
					GetByIDAndOwner(gomock.Any(), nodePoolID, clusterID).
					Return(&api.NodePool{}, nil)
				mockNodePoolSvc.EXPECT().
					ForceDelete(gomock.Any(), nodePoolID, "Stuck in finalizing for 2 hours").
					Return(nil)
				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusNoContent,
		},
		{
			name:       "Error 400 - malformed JSON",
			nodePoolID: nodePoolID,
			body:       `not json`,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockNodePoolService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:       "Error 400 - empty reason",
			nodePoolID: nodePoolID,
			body:       `{"reason": ""}`,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockNodePoolService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:       "Error 400 - reason exceeds max length",
			nodePoolID: nodePoolID,
			body:       `{"reason": "` + strings.Repeat("x", maxReasonLength+1) + `"}`,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockNodePoolService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusBadRequest,
		},
		{
			name:       "Error 404 - nodepool not found",
			nodePoolID: "non-existent-id",
			body:       `{"reason": "some reason"}`,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockNodePoolService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockNodePoolSvc.EXPECT().
					GetByIDAndOwner(gomock.Any(), "non-existent-id", clusterID).
					Return(nil, errors.NotFound("NodePool with id='non-existent-id' not found"))
				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusNotFound,
		},
		{
			name:       "Error 409 - nodepool not in Finalizing state",
			nodePoolID: nodePoolID,
			body:       `{"reason": "some reason"}`,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockNodePoolService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockNodePoolSvc.EXPECT().
					GetByIDAndOwner(gomock.Any(), nodePoolID, clusterID).
					Return(&api.NodePool{}, nil)
				mockNodePoolSvc.EXPECT().
					ForceDelete(gomock.Any(), nodePoolID, "some reason").
					Return(errors.ConflictState("NodePool '%s' is not in Finalizing state", nodePoolID))
				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusConflict,
		},
		{
			name:       "Error 500 - ownership lookup fails",
			nodePoolID: nodePoolID,
			body:       `{"reason": "some reason"}`,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockNodePoolService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockNodePoolSvc.EXPECT().
					GetByIDAndOwner(gomock.Any(), nodePoolID, clusterID).
					Return(nil, errors.GeneralError("database connection lost"))
				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusInternalServerError,
		},
		{
			name:       "Error 500 - force-delete service error",
			nodePoolID: nodePoolID,
			body:       `{"reason": "some reason"}`,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockNodePoolService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockNodePoolSvc.EXPECT().
					GetByIDAndOwner(gomock.Any(), nodePoolID, clusterID).
					Return(&api.NodePool{}, nil)
				mockNodePoolSvc.EXPECT().
					ForceDelete(gomock.Any(), nodePoolID, "some reason").
					Return(errors.GeneralError("failed to delete adapter statuses"))
				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClusterSvc, mockNodePoolSvc := tt.setupMocks(ctrl)
			handler := NewClusterNodePoolsHandler(mockClusterSvc, mockNodePoolSvc)

			reqURL := "/api/hyperfleet/v1/clusters/" + clusterID + "/nodepools/" + tt.nodePoolID + "/force-delete"
			req := httptest.NewRequest(http.MethodPost, reqURL, strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			req = mux.SetURLVars(req, map[string]string{
				"id":          clusterID,
				"nodepool_id": tt.nodePoolID,
			})

			rr := httptest.NewRecorder()
			handler.ForceDelete(rr, req)

			Expect(rr.Code).To(Equal(tt.expectedStatusCode))

			if tt.expectedStatusCode == http.StatusNoContent {
				Expect(rr.Body.Len()).To(Equal(0))
			}

			if tt.expectedStatusCode == http.StatusConflict {
				var errResp openapi.ProblemDetails
				err := json.Unmarshal(rr.Body.Bytes(), &errResp)
				Expect(err).NotTo(HaveOccurred())
				Expect(errResp.Type).To(Equal(errors.ErrorTypeConflict))
				Expect(errResp.Status).To(Equal(http.StatusConflict))
				Expect(*errResp.Detail).To(ContainSubstring("not in Finalizing state"))
				Expect(*errResp.Code).To(Equal("HYPERFLEET-CNF-003"))
			}
		})
	}
}

func TestClusterNodePoolsHandler_Get_WithFieldsFilter(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	clusterID := testClusterID
	nodePoolID := testNodePoolID

	tests := []struct {
		setupMocks         func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockNodePoolService)
		validateResponse   func(body []byte)
		name               string
		queryParams        string
		expectedStatusCode int
	}{
		{
			name:        "Success - Filter with spec and owner_references.id - HYPERFLEET-1142 regression test",
			queryParams: "?fields=id,name,labels,spec,owner_references.id",
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockNodePoolService) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)

				mockNodePoolSvc.EXPECT().GetByIDAndOwner(gomock.Any(), nodePoolID, clusterID).Return(&api.NodePool{
					Meta:             api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
					Kind:             "NodePool",
					Name:             "worker-pool",
					Spec:             []byte(`{"replicas": 3, "instanceType": "m5.large"}`),
					Labels:           []byte(`{"tier": "worker"}`),
					StatusConditions: []byte("[]"),
					OwnerID:          clusterID,
					OwnerKind:        "Cluster",
					OwnerHref:        "/api/hyperfleet/v1/clusters/" + clusterID,
					Generation:       1,
					CreatedBy:        testSystemUser,
					UpdatedBy:        testSystemUser,
				}, nil)

				return mockClusterSvc, mockNodePoolSvc
			},
			expectedStatusCode: http.StatusOK,
			validateResponse: func(body []byte) {
				var response map[string]interface{}
				err := json.Unmarshal(body, &response)
				Expect(err).NotTo(HaveOccurred())

				// Should have requested fields
				Expect(response).To(HaveKey("id"))
				Expect(response).To(HaveKey("name"))
				Expect(response).To(HaveKey("labels"))
				Expect(response).To(HaveKey("spec"))
				Expect(response).To(HaveKey("owner_references"))

				// Verify owner_references.id is present
				ownerRefs := response["owner_references"].(map[string]interface{})
				Expect(ownerRefs).To(HaveKey("id"))

				// Should NOT have other fields
				Expect(response).ToNot(HaveKey("generation"))
				Expect(response).ToNot(HaveKey("status"))
				Expect(response).ToNot(HaveKey("created_time"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockClusterSvc, mockNodePoolSvc := tt.setupMocks(ctrl)
			handler := NewClusterNodePoolsHandler(mockClusterSvc, mockNodePoolSvc)

			reqURL := "/api/hyperfleet/v1/clusters/" + clusterID + "/nodepools/" + nodePoolID + tt.queryParams
			req := httptest.NewRequest(http.MethodGet, reqURL, nil)
			req = mux.SetURLVars(req, map[string]string{
				"id":          clusterID,
				"nodepool_id": nodePoolID,
			})

			rr := httptest.NewRecorder()
			handler.Get(rr, req)

			Expect(rr.Code).To(Equal(tt.expectedStatusCode))
			tt.validateResponse(rr.Body.Bytes())
		})
	}
}
