package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

func TestClusterNodePoolsHandler_Get(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	clusterID := "test-cluster-123"
	nodePoolID := "test-nodepool-456"

	tests := []struct {
		name               string
		clusterID          string
		nodePoolID         string
		setupMocks         func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService)
		expectedStatusCode int
		expectedError      bool
	}{
		{
			name:       "Success - Get nodepool by cluster and nodepool ID",
			clusterID:  clusterID,
			nodePoolID: nodePoolID,
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService) {
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
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService) {
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
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService) {
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
			setupMocks: func(ctrl *gomock.Controller) (*services.MockClusterService, *services.MockNodePoolService, *services.MockGenericService) {
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
			req := httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/clusters/"+tt.clusterID+"/nodepools/"+tt.nodePoolID, nil)
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
