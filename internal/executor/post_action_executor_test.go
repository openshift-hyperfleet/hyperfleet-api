package executor

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/criteria"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleetapi"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// testPAE creates a PostActionExecutor for tests
func testPAE() *PostActionExecutor {
	return newPostActionExecutor(&ExecutorConfig{
		Logger:    logger.NewTestLogger(),
		APIClient: newMockAPIClient(),
	})
}

func TestBuildPayload(t *testing.T) {
	pae := testPAE()

	tests := []struct {
		expected    interface{}
		build       interface{}
		params      map[string]interface{}
		name        string
		expectError bool
	}{
		{
			name:     "nil build returns nil",
			build:    nil,
			params:   map[string]interface{}{},
			expected: nil,
		},
		{
			name:     "string value passthrough",
			build:    "simple string",
			params:   map[string]interface{}{},
			expected: "simple string",
		},
		{
			name:     "int value passthrough",
			build:    42,
			params:   map[string]interface{}{},
			expected: 42,
		},
		{
			name: "simple map",
			build: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
			params: map[string]interface{}{},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": "value2",
			},
		},
		{
			name: "map with template key",
			build: map[string]interface{}{
				"{{ .keyName }}": "value",
			},
			params: map[string]interface{}{
				"keyName": "dynamicKey",
			},
			expected: map[string]interface{}{
				"dynamicKey": "value",
			},
		},
		{
			name: "map[any]any conversion",
			build: map[interface{}]interface{}{
				"key1": "value1",
				"key2": 123,
			},
			params: map[string]interface{}{},
			expected: map[string]interface{}{
				"key1": "value1",
				"key2": 123,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create evaluator context
			evalCtx := criteria.NewEvaluationContext()
			for k, v := range tt.params {
				evalCtx.Set(k, v)
			}
			evaluator, err := criteria.NewEvaluator(context.Background(), evalCtx, pae.log)
			assert.NoError(t, err)

			result, err := pae.buildPayload(context.Background(), tt.build, evaluator, tt.params)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildMapPayload(t *testing.T) {
	pae := testPAE()

	tests := []struct {
		expected    map[string]interface{}
		input       map[string]interface{}
		params      map[string]interface{}
		name        string
		expectError bool
	}{
		{
			name:     "empty map",
			input:    map[string]interface{}{},
			params:   map[string]interface{}{},
			expected: map[string]interface{}{},
		},
		{
			name: "simple key-value pairs",
			input: map[string]interface{}{
				"status":  "active",
				"count":   10,
				"enabled": true,
			},
			params: map[string]interface{}{},
			expected: map[string]interface{}{
				"status":  "active",
				"count":   10,
				"enabled": true,
			},
		},
		{
			name: "template in value",
			input: map[string]interface{}{
				"message": "Hello {{ .name }}",
			},
			params: map[string]interface{}{
				"name": "World",
			},
			expected: map[string]interface{}{
				"message": "Hello World",
			},
		},
		{
			name: "nested map",
			input: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "value",
				},
			},
			params: map[string]interface{}{},
			expected: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "value",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evalCtx := criteria.NewEvaluationContext()
			for k, v := range tt.params {
				evalCtx.Set(k, v)
			}
			evaluator, err := criteria.NewEvaluator(context.Background(), evalCtx, pae.log)
			require.NoError(t, err)
			result, err := pae.buildMapPayload(context.Background(), tt.input, evaluator, tt.params)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestProcessValue(t *testing.T) {
	pae := testPAE()

	tests := []struct {
		expected    interface{}
		value       interface{}
		params      map[string]interface{}
		evalCtxData map[string]interface{}
		name        string
		expectError bool
	}{
		{
			name:     "string without template",
			value:    "plain string",
			params:   map[string]interface{}{},
			expected: "plain string",
		},
		{
			name:     "string with template",
			value:    "Hello {{ .name }}",
			params:   map[string]interface{}{"name": "World"},
			expected: "Hello World",
		},
		{
			name:     "integer passthrough",
			value:    42,
			params:   map[string]interface{}{},
			expected: 42,
		},
		{
			name:     "boolean passthrough",
			value:    true,
			params:   map[string]interface{}{},
			expected: true,
		},
		{
			name:     "float passthrough",
			value:    3.14,
			params:   map[string]interface{}{},
			expected: 3.14,
		},
		{
			name: "expression evaluation",
			value: map[string]interface{}{
				"expression": "1 + 2",
			},
			params:      map[string]interface{}{},
			evalCtxData: map[string]interface{}{},
			expected:    int64(3),
		},
		{
			name: "expression with context variable",
			value: map[string]interface{}{
				"expression": "count * 2",
			},
			params:      map[string]interface{}{},
			evalCtxData: map[string]interface{}{"count": 5},
			expected:    int64(10),
		},
		{
			name:     "slice processing",
			value:    []interface{}{"a", "b", "c"},
			params:   map[string]interface{}{},
			expected: []interface{}{"a", "b", "c"},
		},
		{
			name: "slice with templates",
			value: []interface{}{
				"{{ .prefix }}-1",
				"{{ .prefix }}-2",
			},
			params: map[string]interface{}{"prefix": "item"},
			expected: []interface{}{
				"item-1",
				"item-2",
			},
		},
		{
			name: "map[any]any conversion",
			value: map[interface{}]interface{}{
				"key": "value",
			},
			params: map[string]interface{}{},
			expected: map[string]interface{}{
				"key": "value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evalCtx := criteria.NewEvaluationContext()
			for k, v := range tt.evalCtxData {
				evalCtx.Set(k, v)
			}
			evaluator, err := criteria.NewEvaluator(context.Background(), evalCtx, pae.log)
			require.NoError(t, err)
			result, err := pae.processValue(context.Background(), tt.value, evaluator, tt.params)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPostActionExecutor_ExecuteAll(t *testing.T) {
	tests := []struct {
		postConfig      *configloader.PostConfig
		mockResponse    *hyperfleetapi.Response
		name            string
		expectedResults int
		expectError     bool
	}{
		{
			name:            "nil post config",
			postConfig:      nil,
			expectedResults: 0,
			expectError:     false,
		},
		{
			name: "empty post actions",
			postConfig: &configloader.PostConfig{
				PostActions: []configloader.PostAction{},
			},
			expectedResults: 0,
			expectError:     false,
		},
		{
			name: "single log action",
			postConfig: &configloader.PostConfig{
				PostActions: []configloader.PostAction{
					{
						ActionBase: configloader.ActionBase{
							Name: "log-status",
							Log:  &configloader.LogAction{Message: "Processing complete", Level: "info"},
						},
					},
				},
			},
			expectedResults: 1,
			expectError:     false,
		},
		{
			name: "multiple log actions",
			postConfig: &configloader.PostConfig{
				PostActions: []configloader.PostAction{
					{ActionBase: configloader.ActionBase{
						Name: "log1",
						Log:  &configloader.LogAction{Message: "Step 1", Level: "info"},
					}},
					{ActionBase: configloader.ActionBase{
						Name: "log2",
						Log:  &configloader.LogAction{Message: "Step 2", Level: "info"},
					}},
					{ActionBase: configloader.ActionBase{
						Name: "log3",
						Log:  &configloader.LogAction{Message: "Step 3", Level: "info"},
					}},
				},
			},
			expectedResults: 3,
			expectError:     false,
		},
		{
			name: "with payloads",
			postConfig: &configloader.PostConfig{
				Payloads: []configloader.Payload{
					{
						Name: "statusPayload",
						Build: map[string]interface{}{
							"status": "completed",
						},
					},
				},
				PostActions: []configloader.PostAction{
					{ActionBase: configloader.ActionBase{
						Name: "log1",
						Log:  &configloader.LogAction{Message: "Done", Level: "info"},
					}},
				},
			},
			expectedResults: 1,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := hyperfleetapi.NewMockClient()
			if tt.mockResponse != nil {
				mockClient.DoResponse = tt.mockResponse
			}

			pae := newPostActionExecutor(&ExecutorConfig{
				APIClient: mockClient,
				Logger:    logger.NewTestLogger(),
			})

			evt := event.New()
			evt.SetID("test-event")
			execCtx := NewExecutionContext(context.Background(), map[string]interface{}{}, nil)

			results, err := pae.ExecuteAll(
				context.Background(),
				tt.postConfig,
				execCtx,
			)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, results, tt.expectedResults)
		})
	}
}

func TestExecuteAPICall(t *testing.T) {
	tests := []struct {
		mockError    error
		apiCall      *configloader.APICall
		params       map[string]interface{}
		mockResponse *hyperfleetapi.Response
		name         string
		expectedURL  string
		expectedBody string // optional: for POST/PUT/PATCH, assert last request body (rendered payload)
		expectError  bool
	}{
		{
			name:        "nil api call",
			apiCall:     nil,
			params:      map[string]interface{}{},
			expectError: true,
		},
		{
			name: "simple GET request",
			apiCall: &configloader.APICall{
				Method: "GET",
				URL:    "http://api.example.com/clusters",
			},
			params: map[string]interface{}{},
			mockResponse: &hyperfleetapi.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       []byte(`{"status":"ok"}`),
			},
			expectError: false,
			expectedURL: "http://api.example.com/clusters",
		},
		{
			name: "GET request with URL template",
			apiCall: &configloader.APICall{
				Method: "GET",
				URL:    "http://api.example.com/clusters/{{ .clusterId }}",
			},
			params: map[string]interface{}{
				"clusterId": "cluster-123",
			},
			mockResponse: &hyperfleetapi.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       []byte(`{}`),
			},
			expectError: false,
			expectedURL: "http://api.example.com/clusters/cluster-123",
		},
		{
			name: "POST request with body",
			apiCall: &configloader.APICall{
				Method: "POST",
				URL:    "http://api.example.com/clusters",
				Body:   `{"name": "{{ .name }}"}`,
			},
			params: map[string]interface{}{
				"name": "new-cluster",
			},
			mockResponse: &hyperfleetapi.Response{
				StatusCode: http.StatusCreated,
				Status:     "201 Created",
			},
			expectError:  false,
			expectedURL:  "http://api.example.com/clusters",
			expectedBody: `{"name": "new-cluster"}`,
		},
		{
			name: "PUT request",
			apiCall: &configloader.APICall{
				Method: "PUT",
				URL:    "http://api.example.com/clusters/{{ .id }}",
				Body:   `{"status": "updated"}`,
			},
			params: map[string]interface{}{"id": "123"},
			mockResponse: &hyperfleetapi.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
			},
			expectError:  false,
			expectedURL:  "http://api.example.com/clusters/123",
			expectedBody: `{"status": "updated"}`,
		},
		{
			name: "PATCH request",
			apiCall: &configloader.APICall{
				Method: "PATCH",
				URL:    "http://api.example.com/clusters/123",
				Body:   `{"field": "value"}`,
			},
			params: map[string]interface{}{},
			mockResponse: &hyperfleetapi.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
			},
			expectError:  false,
			expectedURL:  "http://api.example.com/clusters/123",
			expectedBody: `{"field": "value"}`,
		},
		{
			name: "POST with empty body",
			apiCall: &configloader.APICall{
				Method: "POST",
				URL:    "http://api.example.com/clusters",
				Body:   "",
			},
			params: map[string]interface{}{},
			mockResponse: &hyperfleetapi.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
			},
			expectError:  false,
			expectedURL:  "http://api.example.com/clusters",
			expectedBody: "",
		},
		{
			name: "DELETE request",
			apiCall: &configloader.APICall{
				Method: "DELETE",
				URL:    "http://api.example.com/clusters/123",
			},
			params: map[string]interface{}{},
			mockResponse: &hyperfleetapi.Response{
				StatusCode: http.StatusNoContent,
				Status:     "204 No Content",
			},
			expectError: false,
			expectedURL: "http://api.example.com/clusters/123",
		},
		{
			name: "unsupported HTTP method",
			apiCall: &configloader.APICall{
				Method: "INVALID",
				URL:    "http://api.example.com/test",
			},
			params:      map[string]interface{}{},
			expectError: true,
		},
		{
			name: "request with headers",
			apiCall: &configloader.APICall{
				Method: "GET",
				URL:    "http://api.example.com/clusters",
				Headers: []configloader.Header{
					{Name: "Authorization", Value: "Bearer {{ .token }}"},
					{Name: "X-Request-ID", Value: "{{ .requestId }}"},
				},
			},
			params: map[string]interface{}{
				"token":     "secret-token",
				"requestId": "req-123",
			},
			mockResponse: &hyperfleetapi.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
			},
			expectError: false,
			expectedURL: "http://api.example.com/clusters",
		},
		{
			name: "request with timeout",
			apiCall: &configloader.APICall{
				Method:  "GET",
				URL:     "http://api.example.com/slow",
				Timeout: "30s",
			},
			params: map[string]interface{}{},
			mockResponse: &hyperfleetapi.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
			},
			expectError: false,
			expectedURL: "http://api.example.com/slow",
		},
		{
			name: "request with retry config",
			apiCall: &configloader.APICall{
				Method:        "GET",
				URL:           "http://api.example.com/flaky",
				RetryAttempts: 3,
				RetryBackoff:  "exponential",
			},
			params: map[string]interface{}{},
			mockResponse: &hyperfleetapi.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
			},
			expectError: false,
			expectedURL: "http://api.example.com/flaky",
		},
		{
			name: "URL template error",
			apiCall: &configloader.APICall{
				Method: "GET",
				URL:    "http://api.example.com/{{ .missing }}",
			},
			params:      map[string]interface{}{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := hyperfleetapi.NewMockClient()
			if tt.mockResponse != nil {
				mockClient.DoResponse = tt.mockResponse
			}
			if tt.mockError != nil {
				mockClient.DoError = tt.mockError
			}

			execCtx := NewExecutionContext(context.Background(), map[string]interface{}{}, nil)
			execCtx.Params = tt.params

			resp, url, err := ExecuteAPICall(
				context.Background(),
				tt.apiCall,
				execCtx,
				mockClient,
				logger.NewTestLogger(),
			)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Equal(t, tt.expectedURL, url)

			// For body-based methods, verify the rendered payload sent to the client
			isBodyMethod := tt.apiCall != nil &&
				(tt.apiCall.Method == http.MethodPost ||
					tt.apiCall.Method == http.MethodPut ||
					tt.apiCall.Method == http.MethodPatch)
			if isBodyMethod {
				lastReq := mockClient.GetLastRequest()
				require.NotNil(t, lastReq, "expected a request for %s", tt.apiCall.Method)
				assert.Equal(t, tt.expectedBody, string(lastReq.Body), "request body should match rendered payload")
			}
		})
	}
}

func TestPostActionWhenCondition(t *testing.T) {
	tests := []struct {
		when             *configloader.PostActionWhen
		name             string
		wantAPICall      bool
		wantSkipped      bool
		wantErr          bool
		resourcesSkipped bool
	}{
		{
			name:        "no when condition — action always executes",
			when:        nil,
			wantAPICall: true,
		},
		{
			name:        "when expression true — action executes",
			when:        &configloader.PostActionWhen{Expression: "!adapter.resourcesSkipped"},
			wantAPICall: true,
		},
		{
			name:             "when expression false — action skipped",
			when:             &configloader.PostActionWhen{Expression: "!adapter.resourcesSkipped"},
			resourcesSkipped: true,
			wantAPICall:      false,
			wantSkipped:      true,
		},
		{
			name:        "when expression literal false — action skipped",
			when:        &configloader.PostActionWhen{Expression: "false"},
			wantAPICall: false,
			wantSkipped: true,
		},
		{
			name:    "when expression parse error — returns error",
			when:    &configloader.PostActionWhen{Expression: "=== invalid ==="},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := hyperfleetapi.NewMockClient()
			mockClient.DoResponse = &hyperfleetapi.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Body:       []byte(`{}`),
			}

			pae := newPostActionExecutor(&ExecutorConfig{
				APIClient: mockClient,
				Logger:    logger.NewTestLogger(),
			})

			action := configloader.PostAction{
				ActionBase: configloader.ActionBase{
					Name: "testAction",
					APICall: &configloader.APICall{
						Method: "POST",
						URL:    "http://api.example.com/statuses",
					},
				},
				When: tt.when,
			}

			execCtx := NewExecutionContext(context.Background(), map[string]interface{}{}, nil)
			execCtx.Adapter.ResourcesSkipped = tt.resourcesSkipped

			result, err := pae.executePostAction(context.Background(), action, execCtx)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantAPICall, result.APICallMade)
			assert.Equal(t, tt.wantSkipped, result.Skipped)
			if tt.wantSkipped {
				assert.Equal(t, StatusSkipped, result.Status)
			}
		})
	}
}

func TestBuildPostPayloads_WithResourceDiscoveryCELHelpers(t *testing.T) {
	pae := testPAE()
	execCtx := NewExecutionContext(context.Background(), map[string]interface{}{}, nil)

	execCtx.Resources["manifestWork"] = map[string]*unstructured.Unstructured{
		"clusterClaim": {
			Object: map[string]interface{}{
				"status": map[string]interface{}{
					"value": "prod",
				},
			},
		},
	}

	payloads := []configloader.Payload{
		{
			Name: "inspectPayload",
			Build: map[string]interface{}{
				"claimValue": map[string]interface{}{
					"expression": `dig(resources, "manifestWork.clusterClaim.status.value")`,
				},
				"resourceSnapshot": map[string]interface{}{
					"expression": `toJson(resources)`,
				},
			},
		},
	}

	err := pae.buildPostPayloads(context.Background(), payloads, execCtx)
	require.NoError(t, err)

	rawPayload, ok := execCtx.Params["inspectPayload"].(string)
	require.True(t, ok, "payload should be stored as json string in params")

	var built map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(rawPayload), &built))
	assert.Equal(t, "prod", built["claimValue"])
	assert.Contains(t, built["resourceSnapshot"], `"manifestWork"`)
	assert.Contains(t, built["resourceSnapshot"], `"clusterClaim"`)
}
