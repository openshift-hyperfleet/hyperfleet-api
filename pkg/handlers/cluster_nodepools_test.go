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

const (
	testClusterID  = "test-cluster-id"
	testNodePoolID = "test-nodepool-id"
	testSystemUser = "system@hyperfleet.local"
)

func TestClusterNodePoolsHandler_Get(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	clusterID := testClusterID
	nodePoolID := testNodePoolID

	tests := []struct {
		setupMocks func(ctrl *gomock.Controller) ( //nolint:lll
			*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
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
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Get(gomock.Any(), clusterID).Return(&api.Cluster{
					Meta: api.Meta{
						ID:          clusterID,
						CreatedTime: now,
						UpdatedTime: now,
					},
					Name: "test-cluster",
				}, nil)

				mockNodePoolSvc.EXPECT().Get(gomock.Any(), nodePoolID).Return(&api.NodePool{
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

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusOK,
			expectedError:      false,
		},
		{
			name:       "Error - Cluster not found",
			clusterID:  "non-existent",
			nodePoolID: nodePoolID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Get(gomock.Any(), "non-existent").Return(nil, errors.NotFound("Cluster not found"))

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusNotFound,
			expectedError:      true,
		},
		{
			name:       "Error - NodePool not found",
			clusterID:  clusterID,
			nodePoolID: "non-existent",
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Get(gomock.Any(), clusterID).Return(&api.Cluster{
					Meta: api.Meta{
						ID:          clusterID,
						CreatedTime: now,
						UpdatedTime: now,
					},
					Name: "test-cluster",
				}, nil)

				mockNodePoolSvc.EXPECT().Get(gomock.Any(), "non-existent").Return(nil, errors.NotFound("NodePool not found"))

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusNotFound,
			expectedError:      true,
		},
		{
			name:       "Error - NodePool belongs to different cluster",
			clusterID:  clusterID,
			nodePoolID: nodePoolID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Get(gomock.Any(), clusterID).Return(&api.Cluster{
					Meta: api.Meta{
						ID:          clusterID,
						CreatedTime: now,
						UpdatedTime: now,
					},
					Name: "test-cluster",
				}, nil)

				mockNodePoolSvc.EXPECT().Get(gomock.Any(), nodePoolID).Return(&api.NodePool{
					Meta: api.Meta{
						ID:          nodePoolID,
						CreatedTime: now,
						UpdatedTime: now,
					},
					Kind:    "NodePool",
					Name:    "test-nodepool",
					OwnerID: "different-cluster-789", // Different cluster
				}, nil)

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusNotFound,
			expectedError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			// Create gomock controller
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			// Setup mocks
			mockClusterSvc, mockNodePoolSvc, mockGenericSvc := tt.setupMocks(ctrl)

			// Create handler
			handler := NewClusterNodePoolsHandler(mockClusterSvc, mockNodePoolSvc, mockGenericSvc)

			// Create request
			reqURL := "/api/hyperfleet/v1/clusters/" + tt.clusterID + "/nodepools/" + tt.nodePoolID
			req := httptest.NewRequest(http.MethodGet, reqURL, nil)
			req = mux.SetURLVars(req, map[string]string{
				"id":          tt.clusterID,
				"nodepool_id": tt.nodePoolID,
			})

			// Create response recorder
			rr := httptest.NewRecorder()

			// Call handler
			handler.Get(rr, req)

			// Check status code
			Expect(rr.Code).To(Equal(tt.expectedStatusCode))

			if !tt.expectedError {
				// Parse response
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
		setupMocks func(ctrl *gomock.Controller) ( //nolint:lll
			*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
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
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Get(gomock.Any(), clusterID).Return(&api.Cluster{
					Meta: api.Meta{ID: clusterID, CreatedTime: now, UpdatedTime: now},
					Name: "test-cluster",
				}, nil)

				mockNodePoolSvc.EXPECT().Get(gomock.Any(), nodePoolID).Return(&api.NodePool{
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

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusAccepted,
			expectedError:      false,
		},
		{
			name:       "given cluster exists but nodepool belongs to a different cluster, when deleted, then returns 404 (ownership guard)", //nolint:lll
			clusterID:  clusterID,
			nodePoolID: nodePoolID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Get(gomock.Any(), clusterID).Return(&api.Cluster{
					Meta: api.Meta{ID: clusterID, CreatedTime: now, UpdatedTime: now},
					Name: "test-cluster",
				}, nil)

				mockNodePoolSvc.EXPECT().Get(gomock.Any(), nodePoolID).Return(&api.NodePool{
					Meta:    api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
					OwnerID: "different-cluster-789", // belongs to a different cluster
				}, nil)
				// SoftDelete must NOT be called when ownership check fails

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusNotFound,
			expectedError:      true,
		},
		{
			name:       "given cluster does not exist, when deleted, then returns 404",
			clusterID:  "non-existent",
			nodePoolID: nodePoolID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Get(gomock.Any(), "non-existent").Return(nil, errors.NotFound("Cluster not found"))

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusNotFound,
			expectedError:      true,
		},
		{
			name:       "given cluster exists but nodepool does not exist, when deleted, then returns 404",
			clusterID:  clusterID,
			nodePoolID: "non-existent",
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Get(gomock.Any(), clusterID).Return(&api.Cluster{
					Meta: api.Meta{ID: clusterID, CreatedTime: now, UpdatedTime: now},
					Name: "test-cluster",
				}, nil)

				mockNodePoolSvc.EXPECT().Get(gomock.Any(), "non-existent").Return(nil, errors.NotFound("NodePool not found"))

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
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

			mockClusterSvc, mockNodePoolSvc, mockGenericSvc := tt.setupMocks(ctrl)
			handler := NewClusterNodePoolsHandler(mockClusterSvc, mockNodePoolSvc, mockGenericSvc)

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
		setupMocks func(ctrl *gomock.Controller) ( //nolint:lll
			*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
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
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

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

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusCreated,
			expectedError:      false,
		},
		{
			name:      "Error 409 - Cluster is soft-deleted",
			clusterID: clusterID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				deletedTime := now
				mockClusterSvc.EXPECT().Get(gomock.Any(), clusterID).Return(&api.Cluster{
					Meta:        api.Meta{ID: clusterID, CreatedTime: now, UpdatedTime: now},
					Name:        "test-cluster",
					DeletedTime: &deletedTime,
				}, nil)

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusConflict,
			expectedError:      true,
		},
		{
			name:      "Error 404 - Cluster not found",
			clusterID: "non-existent",
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Get(gomock.Any(), "non-existent").Return(nil, errors.NotFound("Cluster not found"))

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
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

			mockClusterSvc, mockNodePoolSvc, mockGenericSvc := tt.setupMocks(ctrl)
			handler := NewClusterNodePoolsHandler(mockClusterSvc, mockNodePoolSvc, mockGenericSvc)

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
				var errResp openapi.Error
				err := json.Unmarshal(rr.Body.Bytes(), &errResp)
				Expect(err).NotTo(HaveOccurred())
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
		setupMocks func(ctrl *gomock.Controller) ( //nolint:lll
			*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
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
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Get(gomock.Any(), clusterID).Return(&api.Cluster{
					Meta: api.Meta{ID: clusterID, CreatedTime: now, UpdatedTime: now},
					Name: "test-cluster",
				}, nil)

				mockNodePoolSvc.EXPECT().Get(gomock.Any(), nodePoolID).Return(&api.NodePool{
					Meta:             api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
					Kind:             "NodePool",
					Name:             "test-nodepool",
					OwnerID:          clusterID,
					Spec:             []byte("{}"),
					Labels:           []byte("{}"),
					StatusConditions: []byte("[]"),
					CreatedBy:        "user@example.com",
					UpdatedBy:        "user@example.com",
				}, nil)

				mockNodePoolSvc.EXPECT().Replace(gomock.Any(), gomock.Any()).Return(&api.NodePool{
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

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusOK,
			expectedError:      false,
		},
		{
			name:       "Error 409 - Cluster is soft-deleted",
			clusterID:  clusterID,
			nodePoolID: nodePoolID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				deletedTime := now
				mockClusterSvc.EXPECT().Get(gomock.Any(), clusterID).Return(&api.Cluster{
					Meta:        api.Meta{ID: clusterID, CreatedTime: now, UpdatedTime: now},
					Name:        "test-cluster",
					DeletedTime: &deletedTime,
				}, nil)

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusConflict,
			expectedError:      true,
		},
		{
			name:       "Error 409 - NodePool is soft-deleted",
			clusterID:  clusterID,
			nodePoolID: nodePoolID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Get(gomock.Any(), clusterID).Return(&api.Cluster{
					Meta: api.Meta{ID: clusterID, CreatedTime: now, UpdatedTime: now},
					Name: "test-cluster",
				}, nil)

				deletedTime := now
				mockNodePoolSvc.EXPECT().Get(gomock.Any(), nodePoolID).Return(&api.NodePool{
					Meta:        api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
					Kind:        "NodePool",
					Name:        "test-nodepool",
					OwnerID:     clusterID,
					DeletedTime: &deletedTime,
				}, nil)

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusConflict,
			expectedError:      true,
		},
		{
			name:       "Error 404 - Cluster not found",
			clusterID:  "non-existent",
			nodePoolID: nodePoolID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Get(gomock.Any(), "non-existent").Return(nil, errors.NotFound("Cluster not found"))

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusNotFound,
			expectedError:      true,
		},
		{
			name:       "Error 404 - NodePool not found",
			clusterID:  clusterID,
			nodePoolID: "non-existent-np",
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Get(gomock.Any(), clusterID).Return(&api.Cluster{
					Meta: api.Meta{ID: clusterID, CreatedTime: now, UpdatedTime: now},
					Name: "test-cluster",
				}, nil)

				mockNodePoolSvc.EXPECT().Get(gomock.Any(), "non-existent-np").Return(nil, errors.NotFound("NodePool not found"))

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
			},
			expectedStatusCode: http.StatusNotFound,
			expectedError:      true,
		},
		{
			name:       "Error 404 - NodePool belongs to different cluster",
			clusterID:  clusterID,
			nodePoolID: nodePoolID,
			setupMocks: func(ctrl *gomock.Controller) (
				*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService,
			) {
				mockClusterSvc := services.NewMockClusterService(ctrl)
				mockNodePoolSvc := services.NewMockNodePoolService(ctrl)
				mockGenericSvc := services.NewMockGenericService(ctrl)

				mockClusterSvc.EXPECT().Get(gomock.Any(), clusterID).Return(&api.Cluster{
					Meta: api.Meta{ID: clusterID, CreatedTime: now, UpdatedTime: now},
					Name: "test-cluster",
				}, nil)

				mockNodePoolSvc.EXPECT().Get(gomock.Any(), nodePoolID).Return(&api.NodePool{
					Meta:             api.Meta{ID: nodePoolID, CreatedTime: now, UpdatedTime: now},
					Kind:             "NodePool",
					Name:             "test-nodepool",
					OwnerID:          "different-cluster-id",
					Spec:             []byte("{}"),
					Labels:           []byte("{}"),
					StatusConditions: []byte("[]"),
					CreatedBy:        "user@example.com",
					UpdatedBy:        "user@example.com",
				}, nil)

				return mockClusterSvc, mockNodePoolSvc, mockGenericSvc
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

			mockClusterSvc, mockNodePoolSvc, mockGenericSvc := tt.setupMocks(ctrl)
			handler := NewClusterNodePoolsHandler(mockClusterSvc, mockNodePoolSvc, mockGenericSvc)

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
				var errResp openapi.Error
				err := json.Unmarshal(rr.Body.Bytes(), &errResp)
				Expect(err).NotTo(HaveOccurred())
				Expect(errResp.Status).To(Equal(http.StatusConflict))
				Expect(*errResp.Detail).To(ContainSubstring("marked for deletion"))
				Expect(*errResp.Code).To(Equal("HYPERFLEET-CNF-003"))
			}
		})
	}
}
