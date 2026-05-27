// Package testutil provides common utilities for integration tests.
package testutil

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// MockRequest represents a recorded HTTP request
type MockRequest struct {
	Method string
	Path   string
	Body   string
}

// MockAPIServer creates a test HTTP server that simulates the HyperFleet API.
// It provides methods to configure responses and inspect recorded requests.
//
// TEMPORARY: This mock server is a placeholder for development and early testing.
// It will be replaced with a real hyperfleet-api container image (via testcontainers)
// for proper integration testing once the API image is available.
//
// TODO: Replace with testcontainers using hyperfleet-api image when available.
type MockAPIServer struct {
	server          *httptest.Server
	t               *testing.T
	clusterResponse map[string]interface{}
	requests        []MockRequest
	statusResponses []map[string]interface{}
	mu              sync.Mutex

	failPrecondition     bool // If true, precondition GET returns 500 (ignored if preconditionNotFound is set)
	preconditionNotFound bool // If true, precondition GET returns 404 (takes precedence over failPrecondition)
	failPostAction       bool // If true, post-action PUT returns 500
	postActionNotFound   bool // If true, post-action PUT returns 404 (takes precedence over failPostAction)
}

// NewMockAPIServer creates a new MockAPIServer for testing.
//
// TEMPORARY: This will be replaced with a real hyperfleet-api testcontainer.
// See MockAPIServer documentation for details.
//
// The server simulates common HyperFleet API endpoints:
//   - GET /clusters/{id} - Returns cluster details
//   - PUT /clusters/{id}/statuses - Accepts status updates
//   - GET /validation/availability - Returns availability status
func NewMockAPIServer(t *testing.T) *MockAPIServer {
	mock := &MockAPIServer{
		t:        t,
		requests: make([]MockRequest, 0),
		clusterResponse: map[string]interface{}{
			"id":   "test-cluster-id",
			"name": "test-cluster",
			"kind": "Cluster",
			"spec": map[string]interface{}{
				"region":     "us-east-1",
				"provider":   "aws",
				"vpc_id":     "vpc-12345",
				"node_count": 3,
			},
			"status": map[string]interface{}{
				"conditions": []map[string]interface{}{
					{
						"type":   "Reconciled",
						"status": "True",
					},
				},
			},
		},
		statusResponses: make([]map[string]interface{}, 0),
	}

	mock.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mock.mu.Lock()
		defer mock.mu.Unlock()

		// Read body
		var bodyStr string
		if r.Body != nil {
			buf := make([]byte, 1024*1024)
			n, readErr := r.Body.Read(buf)
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				t.Logf("Warning: error reading request body: %v", readErr)
			}
			bodyStr = string(buf[:n])
		}

		mock.requests = append(mock.requests, MockRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Body:   bodyStr,
		})

		t.Logf("Mock API received: %s %s", r.Method, r.URL.Path)
		if bodyStr != "" {
			t.Logf("Body: %s", bodyStr)
		}

		// Route handling
		switch {
		case strings.Contains(r.URL.Path, "/clusters/") && strings.HasSuffix(r.URL.Path, "/statuses"):
			// PUT /clusters/{id}/statuses - Store status and return success (or fail if configured)
			if r.Method == http.MethodPut {
				if mock.postActionNotFound {
					w.WriteHeader(http.StatusNotFound)
					if encodeErr := json.NewEncoder(w).Encode(map[string]string{
						"error":   "not found",
						"message": "cluster not found",
					}); encodeErr != nil {
						t.Logf("Warning: failed to encode error response: %v", encodeErr)
					}
					return
				}
				if mock.failPostAction {
					w.WriteHeader(http.StatusInternalServerError)
					if encodeErr := json.NewEncoder(w).Encode(map[string]string{
						"error":   "internal server error",
						"message": "failed to update cluster status",
					}); encodeErr != nil {
						t.Logf("Warning: failed to encode error response: %v", encodeErr)
					}
					return
				}

				var statusBody map[string]interface{}
				if err := json.Unmarshal([]byte(bodyStr), &statusBody); err == nil {
					mock.statusResponses = append(mock.statusResponses, statusBody)
				}
				w.WriteHeader(http.StatusOK)
				if encodeErr := json.NewEncoder(w).Encode(map[string]string{"status": "accepted"}); encodeErr != nil {
					t.Logf("Warning: failed to encode status response: %v", encodeErr)
				}
				return
			}

		case strings.Contains(r.URL.Path, "/clusters/"):
			// GET /clusters/{id} - Return cluster details
			if r.Method == http.MethodGet {
				if mock.preconditionNotFound {
					w.WriteHeader(http.StatusNotFound)
					if encodeErr := json.NewEncoder(w).Encode(map[string]string{"error": "cluster not found"}); encodeErr != nil {
						t.Logf("Warning: failed to encode error response: %v", encodeErr)
					}
					return
				}
				if mock.failPrecondition {
					w.WriteHeader(http.StatusInternalServerError)
					if encodeErr := json.NewEncoder(w).Encode(map[string]string{"error": "internal server error"}); encodeErr != nil {
						t.Logf("Warning: failed to encode error response: %v", encodeErr)
					}
					return
				}
				w.WriteHeader(http.StatusOK)
				if encodeErr := json.NewEncoder(w).Encode(mock.clusterResponse); encodeErr != nil {
					t.Logf("Warning: failed to encode cluster response: %v", encodeErr)
				}
				return
			}

		case strings.Contains(r.URL.Path, "/validation/availability"):
			// GET validation availability
			w.WriteHeader(http.StatusOK)
			if encodeErr := json.NewEncoder(w).Encode("available"); encodeErr != nil {
				t.Logf("Warning: failed to encode availability response: %v", encodeErr)
			}
			return
		}

		// Default 404
		w.WriteHeader(http.StatusNotFound)
		if encodeErr := json.NewEncoder(w).Encode(map[string]string{"error": "not found"}); encodeErr != nil {
			t.Logf("Warning: failed to encode 404 response: %v", encodeErr)
		}
	}))

	return mock
}

// Close stops the mock server
func (m *MockAPIServer) Close() {
	m.server.Close()
}

// URL returns the base URL of the mock server
func (m *MockAPIServer) URL() string {
	return m.server.URL
}

// GetRequests returns a copy of all recorded requests
func (m *MockAPIServer) GetRequests() []MockRequest {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]MockRequest{}, m.requests...)
}

// GetStatusResponses returns a copy of all status responses received
func (m *MockAPIServer) GetStatusResponses() []map[string]interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]map[string]interface{}{}, m.statusResponses...)
}

// SetClusterResponse sets the response for GET /clusters/{id}
func (m *MockAPIServer) SetClusterResponse(resp map[string]interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clusterResponse = resp
}

// SetFailPrecondition configures whether precondition API calls should fail with 500
func (m *MockAPIServer) SetFailPrecondition(fail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failPrecondition = fail
}

// SetPreconditionNotFound configures whether precondition API calls should return 404
func (m *MockAPIServer) SetPreconditionNotFound(notFound bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.preconditionNotFound = notFound
}

// SetPostActionNotFound configures whether post-action API calls should return 404
func (m *MockAPIServer) SetPostActionNotFound(notFound bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.postActionNotFound = notFound
}

// SetFailPostAction configures whether post-action API calls should fail
func (m *MockAPIServer) SetFailPostAction(fail bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failPostAction = fail
}

// ClearRequests clears all recorded requests
func (m *MockAPIServer) ClearRequests() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = make([]MockRequest, 0)
}

// ClearStatusResponses clears all recorded status responses
func (m *MockAPIServer) ClearStatusResponses() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statusResponses = make([]map[string]interface{}, 0)
}

// Reset resets the mock server to its initial state
func (m *MockAPIServer) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests = make([]MockRequest, 0)
	m.statusResponses = make([]map[string]interface{}, 0)
	m.failPrecondition = false
	m.preconditionNotFound = false
	m.failPostAction = false
	m.postActionNotFound = false
	m.clusterResponse = map[string]interface{}{
		"id":   "test-cluster-id",
		"name": "test-cluster",
		"kind": "Cluster",
		"spec": map[string]interface{}{
			"region":     "us-east-1",
			"provider":   "aws",
			"vpc_id":     "vpc-12345",
			"node_count": 3,
		},
		"status": map[string]interface{}{
			"conditions": []map[string]interface{}{
				{
					"type":   "Reconciled",
					"status": "True",
				},
			},
		},
	}
}
