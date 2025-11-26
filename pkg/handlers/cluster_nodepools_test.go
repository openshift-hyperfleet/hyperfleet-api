package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

// Mock ClusterService
type mockClusterService struct {
	getFunc func(ctx context.Context, id string) (*api.Cluster, *errors.ServiceError)
}

func (m *mockClusterService) Get(ctx context.Context, id string) (*api.Cluster, *errors.ServiceError) {
	if m.getFunc != nil {
		return m.getFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockClusterService) Create(ctx context.Context, cluster *api.Cluster) (*api.Cluster, *errors.ServiceError) {
	return nil, nil
}

func (m *mockClusterService) Replace(ctx context.Context, cluster *api.Cluster) (*api.Cluster, *errors.ServiceError) {
	return nil, nil
}

func (m *mockClusterService) Delete(ctx context.Context, id string) *errors.ServiceError {
	return nil
}

func (m *mockClusterService) All(ctx context.Context) (api.ClusterList, *errors.ServiceError) {
	return nil, nil
}

func (m *mockClusterService) FindByIDs(ctx context.Context, ids []string) (api.ClusterList, *errors.ServiceError) {
	return nil, nil
}

func (m *mockClusterService) UpdateClusterStatusFromAdapters(ctx context.Context, clusterID string) (*api.Cluster, *errors.ServiceError) {
	return nil, nil
}

func (m *mockClusterService) OnUpsert(ctx context.Context, id string) error {
	return nil
}

func (m *mockClusterService) OnDelete(ctx context.Context, id string) error {
	return nil
}

// Mock NodePoolService
type mockNodePoolService struct {
	getFunc func(ctx context.Context, id string) (*api.NodePool, *errors.ServiceError)
}

func (m *mockNodePoolService) Get(ctx context.Context, id string) (*api.NodePool, *errors.ServiceError) {
	if m.getFunc != nil {
		return m.getFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockNodePoolService) Create(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, *errors.ServiceError) {
	return nil, nil
}

func (m *mockNodePoolService) Replace(ctx context.Context, nodePool *api.NodePool) (*api.NodePool, *errors.ServiceError) {
	return nil, nil
}

func (m *mockNodePoolService) Delete(ctx context.Context, id string) *errors.ServiceError {
	return nil
}

func (m *mockNodePoolService) All(ctx context.Context) (api.NodePoolList, *errors.ServiceError) {
	return nil, nil
}

func (m *mockNodePoolService) FindByIDs(ctx context.Context, ids []string) (api.NodePoolList, *errors.ServiceError) {
	return nil, nil
}

func (m *mockNodePoolService) UpdateNodePoolStatusFromAdapters(ctx context.Context, nodePoolID string) (*api.NodePool, *errors.ServiceError) {
	return nil, nil
}

func (m *mockNodePoolService) OnUpsert(ctx context.Context, id string) error {
	return nil
}

func (m *mockNodePoolService) OnDelete(ctx context.Context, id string) error {
	return nil
}

// Mock GenericService
type mockGenericService struct{}

func (m *mockGenericService) Get(ctx context.Context, username string, id string, resource interface{}) *errors.ServiceError {
	return nil
}

func (m *mockGenericService) Create(ctx context.Context, username string, resource interface{}) *errors.ServiceError {
	return nil
}

func (m *mockGenericService) List(ctx context.Context, username string, listArgs *services.ListArguments, resources interface{}) (*api.PagingMeta, *errors.ServiceError) {
	return nil, nil
}

func (m *mockGenericService) Update(ctx context.Context, username string, resource interface{}) *errors.ServiceError {
	return nil
}

func (m *mockGenericService) Delete(ctx context.Context, username string, resource interface{}) *errors.ServiceError {
	return nil
}

func TestClusterNodePoolsHandler_Get(t *testing.T) {
	RegisterTestingT(t)

	now := time.Now()
	clusterID := "test-cluster-123"
	nodePoolID := "test-nodepool-456"

	tests := []struct {
		name               string
		clusterID          string
		nodePoolID         string
		mockClusterFunc    func(ctx context.Context, id string) (*api.Cluster, *errors.ServiceError)
		mockNodePoolFunc   func(ctx context.Context, id string) (*api.NodePool, *errors.ServiceError)
		expectedStatusCode int
		expectedError      bool
	}{
		{
			name:       "Success - Get nodepool by cluster and nodepool ID",
			clusterID:  clusterID,
			nodePoolID: nodePoolID,
			mockClusterFunc: func(ctx context.Context, id string) (*api.Cluster, *errors.ServiceError) {
				return &api.Cluster{
					Meta: api.Meta{
						ID:          clusterID,
						CreatedTime: now,
						UpdatedTime: now,
					},
					Name: "test-cluster",
				}, nil
			},
			mockNodePoolFunc: func(ctx context.Context, id string) (*api.NodePool, *errors.ServiceError) {
				return &api.NodePool{
					Meta: api.Meta{
						ID:          nodePoolID,
						CreatedTime: now,
						UpdatedTime: now,
					},
					Kind:    "NodePool",
					Name:    "test-nodepool",
					OwnerID: clusterID,
				}, nil
			},
			expectedStatusCode: http.StatusOK,
			expectedError:      false,
		},
		{
			name:       "Error - Cluster not found",
			clusterID:  "non-existent",
			nodePoolID: nodePoolID,
			mockClusterFunc: func(ctx context.Context, id string) (*api.Cluster, *errors.ServiceError) {
				return nil, errors.NotFound("Cluster not found")
			},
			mockNodePoolFunc: func(ctx context.Context, id string) (*api.NodePool, *errors.ServiceError) {
				return nil, nil
			},
			expectedStatusCode: http.StatusNotFound,
			expectedError:      true,
		},
		{
			name:       "Error - NodePool not found",
			clusterID:  clusterID,
			nodePoolID: "non-existent",
			mockClusterFunc: func(ctx context.Context, id string) (*api.Cluster, *errors.ServiceError) {
				return &api.Cluster{
					Meta: api.Meta{
						ID:          clusterID,
						CreatedTime: now,
						UpdatedTime: now,
					},
					Name: "test-cluster",
				}, nil
			},
			mockNodePoolFunc: func(ctx context.Context, id string) (*api.NodePool, *errors.ServiceError) {
				return nil, errors.NotFound("NodePool not found")
			},
			expectedStatusCode: http.StatusNotFound,
			expectedError:      true,
		},
		{
			name:       "Error - NodePool belongs to different cluster",
			clusterID:  clusterID,
			nodePoolID: nodePoolID,
			mockClusterFunc: func(ctx context.Context, id string) (*api.Cluster, *errors.ServiceError) {
				return &api.Cluster{
					Meta: api.Meta{
						ID:          clusterID,
						CreatedTime: now,
						UpdatedTime: now,
					},
					Name: "test-cluster",
				}, nil
			},
			mockNodePoolFunc: func(ctx context.Context, id string) (*api.NodePool, *errors.ServiceError) {
				return &api.NodePool{
					Meta: api.Meta{
						ID:          nodePoolID,
						CreatedTime: now,
						UpdatedTime: now,
					},
					Kind:    "NodePool",
					Name:    "test-nodepool",
					OwnerID: "different-cluster-789", // Different cluster
				}, nil
			},
			expectedStatusCode: http.StatusNotFound,
			expectedError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			RegisterTestingT(t)

			// Create mock services
			mockClusterSvc := &mockClusterService{getFunc: tt.mockClusterFunc}
			mockNodePoolSvc := &mockNodePoolService{getFunc: tt.mockNodePoolFunc}
			mockGenericSvc := &mockGenericService{}

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
