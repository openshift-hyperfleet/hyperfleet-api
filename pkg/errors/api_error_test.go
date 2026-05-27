package errors

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

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
