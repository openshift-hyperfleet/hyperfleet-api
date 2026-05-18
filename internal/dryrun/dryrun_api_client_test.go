package dryrun

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleetapi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewDryrunAPIClient_NilConfig(t *testing.T) {
	client, err := NewDryrunAPIClient(nil)
	require.NoError(t, err)
	require.NotNil(t, client)
	assert.Empty(t, client.endpoints)
	assert.Empty(t, client.Requests)
}

func TestNewDryrunAPIClient_InvalidRegex(t *testing.T) {
	mrf := &DryrunResponsesFile{
		Responses: []DryrunEndpoint{
			{
				Match: DryrunMatch{
					Method:     "GET",
					URLPattern: "[invalid",
				},
				Responses: []DryrunResponse{
					{StatusCode: 200},
				},
			},
		},
	}

	client, err := NewDryrunAPIClient(mrf)
	assert.Error(t, err)
	assert.Nil(t, client)
	assert.Contains(t, err.Error(), "invalid urlPattern")
}

func TestDo_MatchesEndpoint(t *testing.T) {
	expectedBody := map[string]interface{}{"key": "value"}

	mrf := &DryrunResponsesFile{
		Responses: []DryrunEndpoint{
			{
				Match: DryrunMatch{
					Method:     "GET",
					URLPattern: "/api/v1/tasks.*",
				},
				Responses: []DryrunResponse{
					{
						StatusCode: 201,
						Body:       expectedBody,
					},
				},
			},
		},
	}

	client, err := NewDryrunAPIClient(mrf)
	require.NoError(t, err)

	ctx := context.Background()
	req := &hyperfleetapi.Request{
		Method: "GET",
		URL:    "/api/v1/tasks/123",
	}

	resp, err := client.Do(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)
	assert.Equal(t, "201 Created", resp.Status)
	assert.Equal(t, 1, resp.Attempts)

	var body map[string]interface{}
	err = json.Unmarshal(resp.Body, &body)
	require.NoError(t, err)
	assert.Equal(t, "value", body["key"])

	// Verify request was recorded
	require.Len(t, client.Requests, 1)
	assert.Equal(t, "GET", client.Requests[0].Method)
	assert.Equal(t, "/api/v1/tasks/123", client.Requests[0].URL)
	assert.Equal(t, 201, client.Requests[0].StatusCode)
}

func TestDo_NoMatchDefaultOK(t *testing.T) {
	mrf := &DryrunResponsesFile{
		Responses: []DryrunEndpoint{
			{
				Match: DryrunMatch{
					Method:     "GET",
					URLPattern: "/specific-path",
				},
				Responses: []DryrunResponse{
					{StatusCode: 404},
				},
			},
		},
	}

	client, err := NewDryrunAPIClient(mrf)
	require.NoError(t, err)

	ctx := context.Background()
	req := &hyperfleetapi.Request{
		Method: "GET",
		URL:    "/unmatched-path",
	}

	resp, err := client.Do(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "{}", string(resp.Body))
}

func TestDo_MethodFiltering(t *testing.T) {
	mrf := &DryrunResponsesFile{
		Responses: []DryrunEndpoint{
			{
				Match: DryrunMatch{
					Method:     "POST",
					URLPattern: "/api/v1/tasks",
				},
				Responses: []DryrunResponse{
					{StatusCode: 201},
				},
			},
		},
	}

	client, err := NewDryrunAPIClient(mrf)
	require.NoError(t, err)

	ctx := context.Background()
	req := &hyperfleetapi.Request{
		Method: "GET",
		URL:    "/api/v1/tasks",
	}

	resp, err := client.Do(ctx, req)
	require.NoError(t, err)
	// GET does not match the POST endpoint, so we get the default 200 OK
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "{}", string(resp.Body))
}

func TestDo_WildcardMethod(t *testing.T) {
	mrf := &DryrunResponsesFile{
		Responses: []DryrunEndpoint{
			{
				Match: DryrunMatch{
					Method:     "*",
					URLPattern: "/api/v1/anything",
				},
				Responses: []DryrunResponse{
					{StatusCode: 204},
				},
			},
		},
	}

	client, err := NewDryrunAPIClient(mrf)
	require.NoError(t, err)

	ctx := context.Background()

	tests := []struct {
		name   string
		method string
	}{
		{"GET matches wildcard", "GET"},
		{"POST matches wildcard", "POST"},
		{"DELETE matches wildcard", "DELETE"},
		{"PATCH matches wildcard", "PATCH"},
		{"PUT matches wildcard", "PUT"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := &hyperfleetapi.Request{
				Method: tc.method,
				URL:    "/api/v1/anything",
			}
			resp, err := client.Do(ctx, req)
			require.NoError(t, err)
			assert.Equal(t, 204, resp.StatusCode)
		})
	}
}

func TestDo_SequentialResponses(t *testing.T) {
	mrf := &DryrunResponsesFile{
		Responses: []DryrunEndpoint{
			{
				Match: DryrunMatch{
					Method:     "GET",
					URLPattern: "/api/v1/resource",
				},
				Responses: []DryrunResponse{
					{StatusCode: 200, Body: map[string]interface{}{"call": "first"}},
					{StatusCode: 201, Body: map[string]interface{}{"call": "second"}},
					{StatusCode: 202, Body: map[string]interface{}{"call": "third"}},
				},
			},
		},
	}

	client, err := NewDryrunAPIClient(mrf)
	require.NoError(t, err)

	ctx := context.Background()
	req := &hyperfleetapi.Request{
		Method: "GET",
		URL:    "/api/v1/resource",
	}

	// First call → first response
	resp, err := client.Do(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 200, resp.StatusCode)

	// Second call → second response
	resp, err = client.Do(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 201, resp.StatusCode)

	// Third call → third response
	resp, err = client.Do(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 202, resp.StatusCode)

	// Fourth call → repeats last response (third)
	resp, err = client.Do(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 202, resp.StatusCode)

	// Fifth call → still repeats last response
	resp, err = client.Do(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 202, resp.StatusCode)
}

func TestDo_StatusCodeZeroDefaultsOK(t *testing.T) {
	mrf := &DryrunResponsesFile{
		Responses: []DryrunEndpoint{
			{
				Match: DryrunMatch{
					Method:     "GET",
					URLPattern: "/api/v1/zero",
				},
				Responses: []DryrunResponse{
					{StatusCode: 0, Body: map[string]interface{}{"ok": true}},
				},
			},
		},
	}

	client, err := NewDryrunAPIClient(mrf)
	require.NoError(t, err)

	ctx := context.Background()
	req := &hyperfleetapi.Request{
		Method: "GET",
		URL:    "/api/v1/zero",
	}

	resp, err := client.Do(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "200 OK", resp.Status)
}

func TestConvenienceMethods(t *testing.T) {
	tests := []struct {
		name           string
		call           func(ctx context.Context, client *DryrunAPIClient) (*hyperfleetapi.Response, error)
		expectedMethod string
	}{
		{
			name: "Get",
			call: func(ctx context.Context, c *DryrunAPIClient) (*hyperfleetapi.Response, error) {
				return c.Get(ctx, "/test")
			},
			expectedMethod: "GET",
		},
		{
			name: "Post",
			call: func(ctx context.Context, c *DryrunAPIClient) (*hyperfleetapi.Response, error) {
				return c.Post(ctx, "/test", []byte(`{"data":"post"}`))
			},
			expectedMethod: "POST",
		},
		{
			name: "Put",
			call: func(ctx context.Context, c *DryrunAPIClient) (*hyperfleetapi.Response, error) {
				return c.Put(ctx, "/test", []byte(`{"data":"put"}`))
			},
			expectedMethod: "PUT",
		},
		{
			name: "Patch",
			call: func(ctx context.Context, c *DryrunAPIClient) (*hyperfleetapi.Response, error) {
				return c.Patch(ctx, "/test", []byte(`{"data":"patch"}`))
			},
			expectedMethod: "PATCH",
		},
		{
			name: "Delete",
			call: func(ctx context.Context, c *DryrunAPIClient) (*hyperfleetapi.Response, error) {
				return c.Delete(ctx, "/test")
			},
			expectedMethod: "DELETE",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, err := NewDryrunAPIClient(nil)
			require.NoError(t, err)

			ctx := context.Background()
			resp, err := tc.call(ctx, client)
			require.NoError(t, err)
			assert.Equal(t, http.StatusOK, resp.StatusCode)

			require.Len(t, client.Requests, 1)
			assert.Equal(t, tc.expectedMethod, client.Requests[0].Method)
			assert.Equal(t, "/test", client.Requests[0].URL)
		})
	}
}

func TestBaseURL(t *testing.T) {
	client, err := NewDryrunAPIClient(nil)
	require.NoError(t, err)
	assert.Equal(t, "http://mock-api", client.BaseURL())
}
