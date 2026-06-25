package errors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func resourceNotFoundBody() []byte {
	return []byte(`{
		"type": "https://api.hyperfleet.io/errors/resource-not-found",
		"title": "Resource Not Found",
		"status": 404,
		"detail": "Cluster with id='cls-123' not found",
		"instance": "/api/hyperfleet/v1/clusters/cls-123",
		"code": "HYPERFLEET-NTF-002",
		"trace_id": "019ed716-f3cf-7b8e-b400-0796be4722c3"
	}`)
}

func brokenEndpointBody() []byte {
	return []byte(`{
		"type": "https://api.hyperfleet.io/errors/endpoint-not-found",
		"title": "Endpoint Not Found",
		"status": 404,
		"detail": "The requested endpoint '/api/hyperfleet/v1/clusters-BROKEN/cls-123' does not exist",
		"instance": "/api/hyperfleet/v1/clusters-BROKEN/cls-123",
		"code": "HYPERFLEET-NTF-000",
		"trace_id": ""
	}`)
}

func new404APIError(url string, body []byte) *APIError {
	return NewAPIError(
		"GET", url, 404, "404 Not Found",
		body, 1, 0, fmt.Errorf("not found"),
	)
}

func TestIsResourceNotFound(t *testing.T) {
	tests := []struct {
		err  *APIError
		name string
		want bool
	}{
		{
			name: "resource not found with HYPERFLEET-NTF-002 code",
			err:  new404APIError("/clusters/cls-123", resourceNotFoundBody()),
			want: true,
		},
		{
			name: "nodepool not found with HYPERFLEET-NTF-003 code",
			err: new404APIError("/clusters/cls-123/nodepools/np-1", []byte(`{
				"type": "https://api.hyperfleet.io/errors/not-found",
				"title": "NodePool Not Found",
				"status": 404,
				"detail": "NodePool with id='np-1' not found",
				"code": "HYPERFLEET-NTF-003"
			}`)),
			want: true,
		},
		{
			name: "broken endpoint with HYPERFLEET-NTF-000 code",
			err:  new404APIError("/clusters-BROKEN/cls-123", brokenEndpointBody()),
			want: false,
		},
		{
			name: "404 without response body defaults to resource not found",
			err:  new404APIError("/clusters/cls-123", nil),
			want: true,
		},
		{
			name: "404 with unparseable body defaults to resource not found",
			err:  new404APIError("/clusters/cls-123", []byte("not json")),
			want: true,
		},
		{
			name: "404 with empty JSON object defaults to resource not found",
			err:  new404APIError("/clusters/cls-123", []byte("{}")),
			want: true,
		},
		{
			name: "non-404 with trace_id",
			err: NewAPIError(
				"GET", "/clusters/cls-123",
				500, "500 Internal Server Error",
				resourceNotFoundBody(), 1, 0,
				fmt.Errorf("server error"),
			),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.IsResourceNotFound(),
				"IsResourceNotFound mismatch for %q", tt.name)
		})
	}
}

func TestIsResourceNotFoundError(t *testing.T) {
	tests := []struct {
		err  error
		name string
		want bool
	}{
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "plain error",
			err:  fmt.Errorf("something went wrong"),
			want: false,
		},
		{
			name: "resource not found direct",
			err:  new404APIError("/clusters/cls-123", resourceNotFoundBody()),
			want: true,
		},
		{
			name: "resource not found wrapped",
			err: fmt.Errorf("precondition failed: %w",
				new404APIError("/clusters/cls-123", resourceNotFoundBody())),
			want: true,
		},
		{
			name: "broken endpoint direct",
			err:  new404APIError("/clusters-BROKEN/cls-123", brokenEndpointBody()),
			want: false,
		},
		{
			name: "broken endpoint wrapped",
			err: fmt.Errorf("precondition failed: %w",
				new404APIError("/clusters-BROKEN/cls-123", brokenEndpointBody())),
			want: false,
		},
		{
			name: "404 without body defaults to resource not found",
			err:  new404APIError("/clusters/cls-123", nil),
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsResourceNotFoundError(tt.err),
				"IsResourceNotFoundError mismatch for %q", tt.name)
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		err  error
		name string
		want bool
	}{
		// Non-API errors
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
		{
			name: "plain error",
			err:  fmt.Errorf("something went wrong"),
			want: false,
		},

		// 404 errors: should return true regardless of wrapping depth
		{
			name: "404 direct",
			err:  NewAPIError("GET", "/clusters/123", 404, "404 Not Found", nil, 1, 0, fmt.Errorf("not found")),
			want: true,
		},
		{
			name: "404 single-wrapped",
			err: fmt.Errorf("precondition failed: %w",
				NewAPIError("GET", "/clusters/123", 404, "404 Not Found", nil, 1, 0, fmt.Errorf("not found"))),
			want: true,
		},
		{
			name: "404 double-wrapped (ExecutorError -> APIError chain)",
			err: fmt.Errorf("[preconditions] checkCluster: API call failed: %w",
				NewAPIError("GET", "/clusters/123", 404, "404 Not Found", nil, 1, 0, fmt.Errorf("not found"))),
			want: true,
		},

		// Non-404 API errors: should return false
		{
			name: "500 direct",
			err:  NewAPIError("GET", "/clusters/123", 500, "500 Internal Server Error", nil, 1, 0, fmt.Errorf("server error")),
			want: false,
		},
		{
			name: "500 single-wrapped",
			err: fmt.Errorf("precondition failed: %w",
				NewAPIError("GET", "/clusters/123", 500, "500 Internal Server Error", nil, 1, 0, fmt.Errorf("server error"))),
			want: false,
		},
		{
			name: "500 double-wrapped (ExecutorError -> APIError chain)",
			err: fmt.Errorf("[preconditions] checkCluster: API call failed: %w",
				NewAPIError("GET", "/clusters/123", 500, "500 Internal Server Error", nil, 1, 0, fmt.Errorf("server error"))),
			want: false,
		},
		{
			name: "0 status (no response)",
			err:  NewAPIError("GET", "/clusters/123", 0, "", nil, 1, 0, fmt.Errorf("connection refused")),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsNotFoundError(tt.err))
		})
	}
}
