package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/criteria"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleetapi"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8sclient"
	apierrors "github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/metrics"
)

// newMockAPIClient creates a new mock API client for convenience
func newMockAPIClient() *hyperfleetapi.MockClient {
	return hyperfleetapi.NewMockClient()
}

func mockErrorResponse(statusCode int) (*hyperfleetapi.Response, error) {
	status := fmt.Sprintf("%d %s", statusCode, http.StatusText(statusCode))
	body := []byte(fmt.Sprintf(`{"error":"%d"}`, statusCode))
	resp := &hyperfleetapi.Response{
		StatusCode: statusCode,
		Status:     status,
		Body:       body,
		Attempts:   1,
	}
	apiErr := apierrors.NewAPIError("MOCK", "/mock", statusCode, status, body, 1, 0,
		fmt.Errorf("HTTP %d", statusCode))
	return resp, apiErr
}

// mock404Response returns a 404 response with code HYPERFLEET-NTF-002,
// representing a real resource that was not found.
// The MockClient only returns (nil, error) when an error is set, so using
// mockErrorResponse for 404 tests causes ExecuteAPICall to wrap the error
// in a new APIError with StatusCode=0, which breaks detection. Setting
// only the response lets ValidateAPIResponse create the correct APIError.
func mock404Response() *hyperfleetapi.Response {
	body := `{
		"type": "https://api.hyperfleet.io/errors/resource-not-found",
		"title": "Resource Not Found",
		"status": 404,
		"detail": "Cluster with id='abc123' not found",
		"code": "HYPERFLEET-NTF-002",
		"trace_id": "019ed716-f3cf-7b8e-b400-0796be4722c3"
	}`
	return &hyperfleetapi.Response{
		StatusCode: 404,
		Status:     "404 Not Found",
		Body:       []byte(body),
		Attempts:   1,
	}
}

// mockBrokenEndpoint404Response returns a 404 with code HYPERFLEET-NTF-000,
// representing a misconfigured/broken precondition URL.
func mockBrokenEndpoint404Response() *hyperfleetapi.Response {
	body := `{
		"type": "https://api.hyperfleet.io/errors/endpoint-not-found",
		"title": "Endpoint Not Found",
		"status": 404,
		"detail": "The requested endpoint does not exist",
		"code": "HYPERFLEET-NTF-000",
		"trace_id": ""
	}`
	return &hyperfleetapi.Response{
		StatusCode: 404,
		Status:     "404 Not Found",
		Body:       []byte(body),
		Attempts:   1,
	}
}

func build404TestExecutor(t *testing.T, config *configloader.Config, mockClient *hyperfleetapi.MockClient) *Executor {
	t.Helper()
	exec, err := NewBuilder().
		WithConfig(config).
		WithAPIClient(mockClient).
		WithTransportClient(k8sclient.NewMockK8sClient()).
		WithLogger(logger.NewTestLogger()).
		Build()
	require.NoError(t, err)
	return exec
}

// TestNewExecutor tests the NewExecutor function
func TestNewExecutor(t *testing.T) {
	tests := []struct {
		config      *ExecutorConfig
		name        string
		expectError bool
	}{
		{
			name:        "nil config",
			config:      nil,
			expectError: true,
		},
		{
			name: "missing adapter config",
			config: &ExecutorConfig{
				APIClient: newMockAPIClient(),
				Logger:    logger.NewTestLogger(),
			},
			expectError: true,
		},
		{
			name: "missing API client",
			config: &ExecutorConfig{
				Config: &configloader.Config{},
				Logger: logger.NewTestLogger(),
			},
			expectError: true,
		},
		{
			name: "missing logger",
			config: &ExecutorConfig{
				Config:    &configloader.Config{},
				APIClient: newMockAPIClient(),
			},
			expectError: true,
		},
		{
			name: "valid config",
			config: &ExecutorConfig{
				Config:          &configloader.Config{},
				APIClient:       newMockAPIClient(),
				TransportClient: k8sclient.NewMockK8sClient(),
				Logger:          logger.NewTestLogger(),
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewExecutor(tt.config)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExecutorBuilder(t *testing.T) {
	config := &configloader.Config{
		Adapter: configloader.AdapterInfo{
			Name:    "test-adapter",
			Version: "1.0.0",
		},
	}

	exec, err := NewBuilder().
		WithConfig(config).
		WithAPIClient(newMockAPIClient()).
		WithTransportClient(k8sclient.NewMockK8sClient()).
		WithLogger(logger.NewTestLogger()).
		Build()

	require.NoError(t, err)
	require.NotNil(t, exec)
}

func TestExecutionContext(t *testing.T) {
	ctx := context.Background()
	eventData := map[string]interface{}{
		"id": "test-cluster",
	}

	execCtx := NewExecutionContext(ctx, eventData, nil)

	assert.Equal(t, "test-cluster", execCtx.EventData["id"])
	assert.Empty(t, execCtx.Params)
	assert.Empty(t, execCtx.Resources)
	assert.Equal(t, string(StatusSuccess), execCtx.Adapter.ExecutionStatus)
}

func TestExecutionContext_SetError(t *testing.T) {
	ctx := context.Background()
	execCtx := NewExecutionContext(ctx, map[string]interface{}{}, nil)
	execCtx.SetError("TestReason", "Test message")

	assert.Equal(t, string(StatusFailed), execCtx.Adapter.ExecutionStatus)
	assert.Equal(t, "TestReason", execCtx.Adapter.ErrorReason)
	assert.Equal(t, "Test message", execCtx.Adapter.ErrorMessage)
}

func TestExecutionContext_EvaluationTracking(t *testing.T) {
	ctx := context.Background()
	execCtx := NewExecutionContext(ctx, map[string]interface{}{}, nil)

	// Verify evaluations are empty initially
	assert.Empty(t, execCtx.Evaluations, "expected empty evaluations initially")

	// Add a CEL evaluation
	execCtx.AddCELEvaluation(PhasePreconditions, "check-status", "status == 'active'", true)

	require.Len(t, execCtx.Evaluations, 1, "evaluation")

	eval := execCtx.Evaluations[0]
	assert.Equal(t, PhasePreconditions, eval.Phase)
	assert.Equal(t, "check-status", eval.Name)
	assert.Equal(t, EvaluationTypeCEL, eval.EvaluationType)
	assert.Equal(t, "status == 'active'", eval.Expression)
	assert.True(t, eval.Matched)

	// Add a conditions evaluation with field results (using criteria.EvaluationResult)
	fieldResults := map[string]criteria.EvaluationResult{
		"status.phase": {
			Field:         "status.phase",
			Operator:      criteria.OperatorEquals,
			ExpectedValue: "Running",
			FieldValue:    "Running",
			Matched:       true,
		},
		"replicas": {
			Field:         "replicas",
			Operator:      criteria.OperatorGreaterThan,
			ExpectedValue: 0,
			FieldValue:    3,
			Matched:       true,
		},
	}
	execCtx.AddConditionsEvaluation(PhasePreconditions, "check-replicas", true, fieldResults)

	require.Len(t, execCtx.Evaluations, 2, "evaluations")

	condEval := execCtx.Evaluations[1]
	assert.Equal(t, EvaluationTypeConditions, condEval.EvaluationType)
	assert.Len(t, condEval.FieldResults, 2)

	// Verify lookup by field name works
	assert.Contains(t, condEval.FieldResults, "status.phase")
	assert.Equal(t, "Running", condEval.FieldResults["status.phase"].FieldValue)

	assert.Contains(t, condEval.FieldResults, "replicas")
	assert.Equal(t, 3, condEval.FieldResults["replicas"].FieldValue)
}

func TestExecutionContext_GetEvaluationsByPhase(t *testing.T) {
	ctx := context.Background()
	execCtx := NewExecutionContext(ctx, map[string]interface{}{}, nil)

	// Add evaluations in different phases
	execCtx.AddCELEvaluation(PhasePreconditions, "precond-1", "true", true)
	execCtx.AddCELEvaluation(PhasePreconditions, "precond-2", "false", false)
	execCtx.AddCELEvaluation(PhasePostActions, "post-1", "true", true)

	// Get preconditions evaluations
	precondEvals := execCtx.GetEvaluationsByPhase(PhasePreconditions)
	require.Len(t, precondEvals, 2, "precondition evaluations")

	// Get post actions evaluations
	postEvals := execCtx.GetEvaluationsByPhase(PhasePostActions)
	require.Len(t, postEvals, 1, "post action evaluation")

	// Get resources evaluations (none)
	resourceEvals := execCtx.GetEvaluationsByPhase(PhaseResources)
	require.Len(t, resourceEvals, 0, "resource evaluations")
}

func TestExecutionContext_GetFailedEvaluations(t *testing.T) {
	ctx := context.Background()
	execCtx := NewExecutionContext(ctx, map[string]interface{}{}, nil)

	// Add mixed evaluations
	execCtx.AddCELEvaluation(PhasePreconditions, "passed-1", "true", true)
	execCtx.AddCELEvaluation(PhasePreconditions, "failed-1", "false", false)
	execCtx.AddCELEvaluation(PhasePreconditions, "passed-2", "true", true)
	execCtx.AddCELEvaluation(PhasePostActions, "failed-2", "false", false)

	failedEvals := execCtx.GetFailedEvaluations()
	require.Len(t, failedEvals, 2, "failed evaluations")

	// Verify the failed ones are correct
	names := make(map[string]bool)
	for _, eval := range failedEvals {
		names[eval.Name] = true
	}
	assert.True(t, names["failed-1"], "failed-1")
	assert.True(t, names["failed-2"], "failed-2")
}

func TestExecutorError(t *testing.T) {
	err := NewExecutorError(PhasePreconditions, "test-step", "test message", nil)

	expected := "[preconditions] test-step: test message"
	if err.Error() != expected {
		t.Errorf("expected '%s', got '%s'", expected, err.Error())
	}

	// With wrapped error
	wrappedErr := NewExecutorError(PhaseResources, "create", "failed to create", context.Canceled)
	assert.Equal(t, context.Canceled, wrappedErr.Unwrap())
}

func TestExecute_ParamExtraction(t *testing.T) {
	// Set up environment variable for test
	t.Setenv("TEST_VAR", "test-value")

	config := &configloader.Config{
		Adapter: configloader.AdapterInfo{
			Name:    "test-adapter",
			Version: "1.0.0",
		},
		Params: []configloader.Parameter{
			{
				Name:     "testParam",
				Source:   "env.TEST_VAR",
				Required: true,
			},
			{
				Name:     "eventParam",
				Source:   "event.id",
				Required: true,
			},
		},
	}

	exec, err := NewBuilder().
		WithConfig(config).
		WithAPIClient(newMockAPIClient()).
		WithTransportClient(k8sclient.NewMockK8sClient()).
		WithLogger(logger.NewTestLogger()).
		Build()
	if err != nil {
		t.Fatalf("unexpected error creating executor: %v", err)
	}

	// Create event data
	eventData := map[string]interface{}{
		"id": "cluster-456",
	}

	// Execute with event ID in context
	ctx := logger.WithEventID(context.Background(), "test-event-123")
	result := exec.Execute(ctx, eventData)

	// Check result

	// Check extracted params
	if result.Params["testParam"] != "test-value" {
		t.Errorf("expected testParam to be 'test-value', got '%v'", result.Params["testParam"])
	}

	if result.Params["eventParam"] != "cluster-456" {
		t.Errorf("expected eventParam to be 'cluster-456', got '%v'", result.Params["eventParam"])
	}
}

func TestParamExtractor(t *testing.T) {
	t.Setenv("TEST_ENV", "env-value")

	evt := event.New()
	eventData := map[string]interface{}{
		"id": "test-cluster",
		"nested": map[string]interface{}{
			"value": "nested-value",
		},
	}
	_ = evt.SetData(event.ApplicationJSON, eventData)

	tests := []struct {
		expectValue interface{}
		name        string
		expectKey   string
		params      []configloader.Parameter
		expectError bool
	}{
		{
			name: "extract from env",
			params: []configloader.Parameter{
				{Name: "envVar", Source: "env.TEST_ENV"},
			},
			expectKey:   "envVar",
			expectValue: "env-value",
		},
		{
			name: "extract from event",
			params: []configloader.Parameter{
				{Name: "clusterId", Source: "event.id"},
			},
			expectKey:   "clusterId",
			expectValue: "test-cluster",
		},
		{
			name: "extract nested from event",
			params: []configloader.Parameter{
				{Name: "nestedVal", Source: "event.nested.value"},
			},
			expectKey:   "nestedVal",
			expectValue: "nested-value",
		},
		{
			name: "use default for missing optional",
			params: []configloader.Parameter{
				{Name: "optional", Source: "env.MISSING", Default: "default-val"},
			},
			expectKey:   "optional",
			expectValue: "default-val",
		},
		{
			name: "fail on missing required",
			params: []configloader.Parameter{
				{Name: "required", Source: "env.MISSING", Required: true},
			},
			expectError: true,
		},
		{
			name: "extract from config",
			params: []configloader.Parameter{
				{Name: "adapterName", Source: "config.adapter.name"},
			},
			expectKey:   "adapterName",
			expectValue: "test",
		},
		{
			name: "extract nested from config",
			params: []configloader.Parameter{
				{Name: "adapterVersion", Source: "config.adapter.version"},
			},
			expectKey:   "adapterVersion",
			expectValue: "1.0.0",
		},
		{
			name: "use default for missing optional config field",
			params: []configloader.Parameter{
				{Name: "optional", Source: "config.nonexistent", Default: "fallback"},
			},
			expectKey:   "optional",
			expectValue: "fallback",
		},
		{
			name: "fail on missing required config field",
			params: []configloader.Parameter{
				{Name: "required", Source: "config.nonexistent", Required: true},
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create fresh context for each test
			execCtx := NewExecutionContext(context.Background(), eventData, nil)

			// Create config with test params
			config := &configloader.Config{
				Adapter: configloader.AdapterInfo{
					Name:    "test",
					Version: "1.0.0",
				},
				Params: tt.params,
			}

			// Extract params using pure function
			configMap, err := configToMap(config)
			require.NoError(t, err)
			err = extractConfigParams(config, execCtx, configMap)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tt.expectKey != "" {
				if execCtx.Params[tt.expectKey] != tt.expectValue {
					t.Errorf("expected %s=%v, got %v", tt.expectKey, tt.expectValue, execCtx.Params[tt.expectKey])
				}
			}
		})
	}
}

// TestSequentialExecution_Preconditions tests that preconditions stop on first failure
func TestSequentialExecution_Preconditions(t *testing.T) {
	tests := []struct {
		name             string
		expectedLastName string
		preconditions    []configloader.Precondition
		expectedResults  int // number of results before stopping
		expectError      bool
		expectNotMet     bool
	}{
		{
			name: "all pass - all executed",
			preconditions: []configloader.Precondition{
				{ActionBase: configloader.ActionBase{Name: "precond1"}, Expression: "true"},
				{ActionBase: configloader.ActionBase{Name: "precond2"}, Expression: "true"},
				{ActionBase: configloader.ActionBase{Name: "precond3"}, Expression: "true"},
			},
			expectedResults:  3,
			expectError:      false,
			expectNotMet:     false,
			expectedLastName: "precond3",
		},
		{
			name: "first fails - stops immediately",
			preconditions: []configloader.Precondition{
				{ActionBase: configloader.ActionBase{Name: "precond1"}, Expression: "false"},
				{ActionBase: configloader.ActionBase{Name: "precond2"}, Expression: "true"},
				{ActionBase: configloader.ActionBase{Name: "precond3"}, Expression: "true"},
			},
			expectedResults:  1,
			expectError:      false,
			expectNotMet:     true,
			expectedLastName: "precond1",
		},
		{
			name: "second fails - first executes, stops at second",
			preconditions: []configloader.Precondition{
				{ActionBase: configloader.ActionBase{Name: "precond1"}, Expression: "true"},
				{ActionBase: configloader.ActionBase{Name: "precond2"}, Expression: "false"},
				{ActionBase: configloader.ActionBase{Name: "precond3"}, Expression: "true"},
			},
			expectedResults:  2,
			expectError:      false,
			expectNotMet:     true,
			expectedLastName: "precond2",
		},
		{
			name: "third fails - first two execute, stops at third",
			preconditions: []configloader.Precondition{
				{ActionBase: configloader.ActionBase{Name: "precond1"}, Expression: "true"},
				{ActionBase: configloader.ActionBase{Name: "precond2"}, Expression: "true"},
				{ActionBase: configloader.ActionBase{Name: "precond3"}, Expression: "false"},
			},
			expectedResults:  3,
			expectError:      false,
			expectNotMet:     true,
			expectedLastName: "precond3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &configloader.Config{
				Adapter: configloader.AdapterInfo{
					Name:    "test-adapter",
					Version: "1.0.0",
				},
				Preconditions: tt.preconditions,
			}

			exec, err := NewBuilder().
				WithConfig(config).
				WithAPIClient(newMockAPIClient()).
				WithTransportClient(k8sclient.NewMockK8sClient()).
				WithLogger(logger.NewTestLogger()).
				Build()
			if err != nil {
				t.Fatalf("unexpected error creating executor: %v", err)
			}

			ctx := logger.WithEventID(context.Background(), "test-event-seq")
			result := exec.Execute(ctx, map[string]interface{}{})

			// Verify number of precondition results
			assert.Equal(t, tt.expectedResults, len(result.PreconditionResults),
				"unexpected precondition result count")

			// Verify last executed precondition name
			if len(result.PreconditionResults) > 0 {
				lastResult := result.PreconditionResults[len(result.PreconditionResults)-1]
				if lastResult.Name != tt.expectedLastName {
					t.Errorf("expected last precondition to be '%s', got '%s'",
						tt.expectedLastName, lastResult.Name)
				}
			}

			// Verify error/not met status
			if tt.expectNotMet {
				// Precondition not met is a successful execution, just with resources skipped
				assert.Equal(t, StatusSuccess, result.Status, "expected status Success (precondition not met is valid outcome)")
				assert.True(t, result.ResourcesSkipped, "ResourcesSkipped")
				assert.NotEmpty(t, result.SkipReason, "expected SkipReason to be set")
			}

			if !tt.expectNotMet && !tt.expectError {
				assert.Equal(t, StatusSuccess, result.Status, "expected status Success")
			}
		})
	}
}

// TestPrecondition_CustomCELFunctions tests that custom CEL functions
// (like now()) are available in precondition expressions
func TestPrecondition_CustomCELFunctions(t *testing.T) {
	tests := []struct {
		name        string
		expression  string
		shouldMatch bool
	}{
		{
			name:        "now() returns valid timestamp",
			expression:  `timestamp(now()).getFullYear() >= 2024`,
			shouldMatch: true,
		},
		{
			name:        "now() can be used in time comparisons",
			expression:  `(timestamp(now()) - timestamp("2020-01-01T00:00:00Z")).getSeconds() > 0`,
			shouldMatch: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &configloader.Config{
				Adapter: configloader.AdapterInfo{
					Name:    "test-adapter",
					Version: "1.0.0",
				},
				Preconditions: []configloader.Precondition{
					{ActionBase: configloader.ActionBase{Name: "test-custom-function"}, Expression: tt.expression},
				},
			}

			exec, err := NewBuilder().
				WithConfig(config).
				WithAPIClient(newMockAPIClient()).
				WithTransportClient(k8sclient.NewMockK8sClient()).
				WithLogger(logger.NewTestLogger()).
				Build()
			require.NoError(t, err, "failed to create executor")

			ctx := logger.WithEventID(context.Background(), "test-custom-cel")
			result := exec.Execute(ctx, map[string]interface{}{})

			// Verify precondition executed
			require.Len(t, result.PreconditionResults, 1, "expected one precondition result")
			precondResult := result.PreconditionResults[0]

			// Verify the expression evaluated correctly
			assert.Equal(t, tt.shouldMatch, precondResult.Matched, "unexpected match result")
			assert.Equal(t, StatusSuccess, precondResult.Status, "expected precondition status to be success")

			if tt.shouldMatch {
				// If precondition matched, resources should have been attempted
				assert.Equal(t, StatusSuccess, result.Status, "expected overall status success")
				assert.False(t, result.ResourcesSkipped, "resources should not be skipped")
			}
		})
	}
}

// TestSequentialExecution_Resources tests that resources stop on first failure
func TestSequentialExecution_Resources(t *testing.T) {
	// Note: This test uses dry-run mode and focuses on the sequential logic
	// without requiring a real K8s cluster. Resource sequential execution is better
	// tested in integration tests with real K8s API.

	tests := []struct {
		name            string
		resources       []configloader.Resource
		expectedResults int
		expectFailure   bool
	}{
		{
			name: "single resource with valid manifest",
			resources: []configloader.Resource{
				{
					Name: "resource1",
					Manifest: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name": "test-cm",
						},
					},
				},
			},
			expectedResults: 1,
			expectFailure:   false,
		},
		{
			name: "first resource has no manifest - stops immediately",
			resources: []configloader.Resource{
				{Name: "resource1"}, // No manifest at all
				{
					Name: "resource2",
					Manifest: map[string]interface{}{
						"apiVersion": "v1",
						"kind":       "ConfigMap",
						"metadata": map[string]interface{}{
							"name": "test-cm2",
						},
					},
				},
			},
			expectedResults: 1, // Stops at first failure
			expectFailure:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &configloader.Config{
				Adapter: configloader.AdapterInfo{
					Name:    "test-adapter",
					Version: "1.0.0",
				},
				Resources: tt.resources,
			}

			exec, err := NewBuilder().
				WithConfig(config).
				WithAPIClient(newMockAPIClient()).
				WithTransportClient(k8sclient.NewMockK8sClient()).
				WithLogger(logger.NewTestLogger()).
				Build()
			if err != nil {
				t.Fatalf("unexpected error creating executor: %v", err)
			}

			ctx := logger.WithEventID(context.Background(), "test-event-resources")
			result := exec.Execute(ctx, map[string]interface{}{})

			// Verify sequential stop-on-failure: number of results should match expected
			assert.Equal(t, tt.expectedResults, len(result.ResourceResults),
				"sequential execution should stop at failure")

			// Verify failure status
			if tt.expectFailure {
				if result.Status == StatusSuccess {
					t.Error("expected execution to fail but got success")
				}
			}
		})
	}
}

// TestSequentialExecution_PostActions tests that post actions stop on first failure
func TestSequentialExecution_PostActions(t *testing.T) {
	tests := []struct {
		mockError       error
		mockResponse    *hyperfleetapi.Response
		name            string
		postActions     []configloader.PostAction
		expectedResults int
		expectError     bool
	}{
		{
			name: "all log actions succeed",
			postActions: []configloader.PostAction{
				{ActionBase: configloader.ActionBase{Name: "log1", Log: &configloader.LogAction{Message: "msg1"}}},
				{ActionBase: configloader.ActionBase{Name: "log2", Log: &configloader.LogAction{Message: "msg2"}}},
				{ActionBase: configloader.ActionBase{Name: "log3", Log: &configloader.LogAction{Message: "msg3"}}},
			},
			expectedResults: 3,
			expectError:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			postConfig := &configloader.PostConfig{
				PostActions: tt.postActions,
			}

			config := &configloader.Config{
				Adapter: configloader.AdapterInfo{
					Name:    "test-adapter",
					Version: "1.0.0",
				},
				Post: postConfig,
			}

			mockClient := newMockAPIClient()
			mockClient.GetResponse = tt.mockResponse
			mockClient.GetError = tt.mockError
			mockClient.PostResponse = tt.mockResponse
			mockClient.PostError = tt.mockError

			exec, err := NewBuilder().
				WithConfig(config).
				WithAPIClient(mockClient).
				WithTransportClient(k8sclient.NewMockK8sClient()).
				WithLogger(logger.NewTestLogger()).
				Build()
			if err != nil {
				t.Fatalf("unexpected error creating executor: %v", err)
			}

			ctx := logger.WithEventID(context.Background(), "test-event-post")
			result := exec.Execute(ctx, map[string]interface{}{})

			// Verify number of post action results
			assert.Equal(t, tt.expectedResults, len(result.PostActionResults),
				"unexpected post action result count")

			// Verify error expectation
			if tt.expectError {
				assert.NotEmpty(t, result.Errors, "expected errors, got none")
				assert.NotNil(t, result.Errors[PhasePostActions], "expected post_actions error, got %#v", result.Errors)
			} else {
				assert.Empty(t, result.Errors, "expected no errors, got %#v", result.Errors)
			}
		})
	}
}

// TestSequentialExecution_SkipReasonCapture tests that SkipReason captures which precondition wasn't met
func TestSequentialExecution_SkipReasonCapture(t *testing.T) {
	tests := []struct {
		name           string
		expectedStatus ExecutionStatus
		preconditions  []configloader.Precondition
		expectSkipped  bool
	}{
		{
			name: "first precondition not met",
			preconditions: []configloader.Precondition{
				{ActionBase: configloader.ActionBase{Name: "check1"}, Expression: "false"},
				{ActionBase: configloader.ActionBase{Name: "check2"}, Expression: "true"},
				{ActionBase: configloader.ActionBase{Name: "check3"}, Expression: "true"},
			},
			expectedStatus: StatusSuccess, // Successful execution, just resources skipped
			expectSkipped:  true,
		},
		{
			name: "second precondition not met",
			preconditions: []configloader.Precondition{
				{ActionBase: configloader.ActionBase{Name: "check1"}, Expression: "true"},
				{ActionBase: configloader.ActionBase{Name: "check2"}, Expression: "false"},
				{ActionBase: configloader.ActionBase{Name: "check3"}, Expression: "true"},
			},
			expectedStatus: StatusSuccess, // Successful execution, just resources skipped
			expectSkipped:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &configloader.Config{
				Adapter: configloader.AdapterInfo{
					Name:    "test-adapter",
					Version: "1.0.0",
				},
				Preconditions: tt.preconditions,
			}

			exec, err := NewBuilder().
				WithConfig(config).
				WithAPIClient(newMockAPIClient()).
				WithTransportClient(k8sclient.NewMockK8sClient()).
				WithLogger(logger.NewTestLogger()).
				Build()
			if err != nil {
				t.Fatalf("unexpected error creating executor: %v", err)
			}

			ctx := logger.WithEventID(context.Background(), "test-event-skip")
			result := exec.Execute(ctx, map[string]interface{}{})

			// Verify execution status is success (adapter executed successfully)
			if result.Status != tt.expectedStatus {
				t.Errorf("expected status %s, got %s", tt.expectedStatus, result.Status)
			}

			// Verify resources were skipped
			if tt.expectSkipped {
				assert.True(t, result.ResourcesSkipped, "ResourcesSkipped")
				assert.NotEmpty(t, result.SkipReason, "expected SkipReason to be set")
				// Verify execution context captures skip information
				if result.ExecutionContext != nil {
					assert.True(t, result.ExecutionContext.Adapter.ResourcesSkipped, "adapter.ResourcesSkipped")
				}
			}
		})
	}
}

// TestCreateHandler_MetricsRecording verifies that WithMetrics records Prometheus metrics
func TestCreateHandler_MetricsRecording(t *testing.T) {
	tests := []struct {
		name           string
		preconditions  []configloader.Precondition
		expectedStatus string // "success", "skipped", or "failed"
		expectedErrors []string
	}{
		{
			name:           "success records success metric",
			preconditions:  []configloader.Precondition{},
			expectedStatus: "success",
		},
		{
			name: "skipped records skipped metric",
			preconditions: []configloader.Precondition{
				{ActionBase: configloader.ActionBase{Name: "check"}, Expression: "false"},
			},
			expectedStatus: "skipped",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			recorder := metrics.NewRecorder("test-adapter", "v0.1.0", "test", registry)

			config := &configloader.Config{
				Adapter:       configloader.AdapterInfo{Name: "test-adapter", Version: "v0.1.0"},
				Preconditions: tt.preconditions,
			}

			exec, err := NewBuilder().
				WithConfig(config).
				WithAPIClient(newMockAPIClient()).
				WithTransportClient(k8sclient.NewMockK8sClient()).
				WithLogger(logger.NewTestLogger()).
				Build()
			require.NoError(t, err)

			handler := AlwaysAck(WithMetrics(exec.CreateHandler(), recorder, logger.NewTestLogger()), logger.NewTestLogger())

			evt := event.New()
			evt.SetID("test-event-1")
			evt.SetType("com.hyperfleet.test")
			evt.SetSource("test")
			eventData := map[string]interface{}{"id": "cluster-1"}
			eventBytes, _ := json.Marshal(eventData)
			_ = evt.SetData(event.ApplicationJSON, eventBytes)

			err = handler(context.Background(), &evt)
			require.NoError(t, err, "handler should always return nil")

			// Verify events_processed_total
			families, err := registry.Gather()
			require.NoError(t, err)

			eventsCount := getCounterValue(t, families, "hyperfleet_adapter_events_processed_total", "status", tt.expectedStatus)
			assert.Equal(t, float64(1), eventsCount, "expected 1 event with status %s", tt.expectedStatus)

			// Verify duration was recorded
			durationFamily := findFamily(families, "hyperfleet_adapter_event_processing_duration_seconds")
			require.NotNil(t, durationFamily, "duration metric should exist")
			histogram := durationFamily.GetMetric()[0].GetHistogram()
			assert.Equal(t, uint64(1), histogram.GetSampleCount(), "expected 1 duration sample")
		})
	}
}

// TestCreateHandler_MetricsRecording_Failed verifies error metrics are recorded on failure
func TestCreateHandler_MetricsRecording_Failed(t *testing.T) {
	registry := prometheus.NewRegistry()
	recorder := metrics.NewRecorder("test-adapter", "v0.1.0", "test", registry)

	config := &configloader.Config{
		Adapter: configloader.AdapterInfo{Name: "test-adapter", Version: "v0.1.0"},
		Params: []configloader.Parameter{
			{Name: "required", Source: "env.MISSING_VAR", Required: true},
		},
	}

	exec, err := NewBuilder().
		WithConfig(config).
		WithAPIClient(newMockAPIClient()).
		WithTransportClient(k8sclient.NewMockK8sClient()).
		WithLogger(logger.NewTestLogger()).
		Build()
	require.NoError(t, err)

	handler := AlwaysAck(WithMetrics(exec.CreateHandler(), recorder, logger.NewTestLogger()), logger.NewTestLogger())

	evt := event.New()
	evt.SetID("test-event-fail")
	evt.SetType("com.hyperfleet.test")
	evt.SetSource("test")
	eventData := map[string]interface{}{"id": "cluster-1"}
	eventBytes, _ := json.Marshal(eventData)
	_ = evt.SetData(event.ApplicationJSON, eventBytes)

	err = handler(context.Background(), &evt)
	require.NoError(t, err, "handler should always return nil even on failure")

	families, err := registry.Gather()
	require.NoError(t, err)

	// Verify failed event was recorded
	failedCount := getCounterValue(t, families, "hyperfleet_adapter_events_processed_total", "status", "failed")
	assert.Equal(t, float64(1), failedCount, "expected 1 failed event")

	// Verify error was recorded with phase label
	errorCount := getCounterValue(t, families, "hyperfleet_adapter_errors_total", "error_type", "param_extraction")
	assert.Equal(t, float64(1), errorCount, "expected 1 param_extraction error")
}

// TestCreateHandler_NilMetricsRecorder verifies handler works without a metrics recorder
func TestCreateHandler_NilMetricsRecorder(t *testing.T) {
	config := &configloader.Config{
		Adapter: configloader.AdapterInfo{Name: "test-adapter", Version: "v0.1.0"},
	}

	exec, err := NewBuilder().
		WithConfig(config).
		WithAPIClient(newMockAPIClient()).
		WithTransportClient(k8sclient.NewMockK8sClient()).
		WithLogger(logger.NewTestLogger()).
		Build()
	require.NoError(t, err)

	handler := AlwaysAck(WithMetrics(exec.CreateHandler(), nil, logger.NewTestLogger()), logger.NewTestLogger())

	evt := event.New()
	evt.SetID("test-event-nil")
	evt.SetType("com.hyperfleet.test")
	evt.SetSource("test")
	_ = evt.SetData(event.ApplicationJSON, []byte(`{"id":"cluster-1"}`))

	assert.NotPanics(t, func() {
		_ = handler(context.Background(), &evt)
	}, "handler with nil MetricsRecorder should not panic")
}

// TestWithMetrics_RecordsMetrics verifies WithMetrics records the correct metric status
// and passes the result through
func TestWithMetrics_RecordsMetrics(t *testing.T) {
	tests := []struct {
		result          *ExecutionResult
		name            string
		expectedStatus  string
		expectNoMetrics bool
	}{
		{
			name:           "success",
			result:         &ExecutionResult{Status: StatusSuccess},
			expectedStatus: "success",
		},
		{
			name:           "skipped",
			result:         &ExecutionResult{Status: StatusSuccess, ResourcesSkipped: true},
			expectedStatus: "skipped",
		},
		{
			name: "failed",
			result: &ExecutionResult{
				Status: StatusFailed,
				Errors: map[ExecutionPhase]error{PhaseParamExtraction: fmt.Errorf("error")},
			},
			expectedStatus: "failed",
		},
		{
			name:            "nil result",
			result:          nil,
			expectNoMetrics: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registry := prometheus.NewRegistry()
			recorder := metrics.NewRecorder("test-adapter", "v0.1.0", "test", registry)

			inner := HandlerFunc(func(_ context.Context, _ *event.Event) (*ExecutionResult, error) {
				return tt.result, nil
			})
			handler := WithMetrics(inner, recorder, logger.NewTestLogger())

			evt := event.New()
			evt.SetID("test-metrics-" + tt.name)
			evt.SetType("com.hyperfleet.test")
			evt.SetSource("test")

			got, err := handler(context.Background(), &evt)
			require.NoError(t, err)
			assert.Equal(t, tt.result, got, "result must be passed through unchanged")

			families, err := registry.Gather()
			require.NoError(t, err)

			if tt.expectNoMetrics {
				assert.Nil(t, findFamily(families, "hyperfleet_adapter_events_processed_total"),
					"no status counter should be recorded for nil result")
				durationFamily := findFamily(families, "hyperfleet_adapter_event_processing_duration_seconds")
				require.NotNil(t, durationFamily, "duration must be recorded even for nil result")
				assert.Equal(t, uint64(1), durationFamily.GetMetric()[0].GetHistogram().GetSampleCount())
				return
			}

			count := getCounterValue(t, families, "hyperfleet_adapter_events_processed_total", "status", tt.expectedStatus)
			assert.Equal(t, float64(1), count)

			durationFamily := findFamily(families, "hyperfleet_adapter_event_processing_duration_seconds")
			require.NotNil(t, durationFamily)
			assert.Equal(t, uint64(1), durationFamily.GetMetric()[0].GetHistogram().GetSampleCount())
		})
	}
}

// TestWithMetrics_HandlerPanicPropagates verifies a panic in handler is not swallowed by WithMetrics
func TestWithMetrics_HandlerPanicPropagates(t *testing.T) {
	inner := HandlerFunc(func(_ context.Context, _ *event.Event) (*ExecutionResult, error) {
		panic("handler panic")
	})

	registry := prometheus.NewRegistry()
	recorder := metrics.NewRecorder("test-adapter", "v0.1.0", "test", registry)
	handler := WithMetrics(inner, recorder, logger.NewTestLogger())

	evt := event.New()
	evt.SetID("test-handler-panic")
	evt.SetType("com.hyperfleet.test")
	evt.SetSource("test")

	assert.Panics(t, func() {
		_, _ = handler(context.Background(), &evt)
	}, "panic in inner handler must propagate through WithMetrics")
}

// TestWithMetrics_MetricsPanicRecovered verifies that a panic inside metrics recording
// is recovered
func TestWithMetrics_MetricsPanicRecovered(t *testing.T) {
	inner := HandlerFunc(func(_ context.Context, _ *event.Event) (*ExecutionResult, error) {
		return &ExecutionResult{Status: StatusSuccess}, nil
	})

	// new(metrics.Recorder) bypasses the nil receiver guard but has nil internal
	// fields, causing panic inside recordMetrics
	panicRecorder := new(metrics.Recorder)
	handler := WithMetrics(inner, panicRecorder, logger.NewTestLogger())

	evt := event.New()
	evt.SetID("test-metrics-panic")
	evt.SetType("com.hyperfleet.test")
	evt.SetSource("test")

	var got *ExecutionResult
	assert.NotPanics(t, func() {
		got, _ = handler(context.Background(), &evt)
	}, "panic in metrics recording must be recovered by WithMetrics")
	assert.NotNil(t, got, "result must still be returned after metrics panic")
}

// TestAlwaysAck_AlwaysReturnsNil verifies AlwaysAck always returns nil
func TestAlwaysAck_AlwaysReturnsNil(t *testing.T) {
	tests := []struct {
		result *ExecutionResult
		err    error
		name   string
	}{
		{
			name:   "success result",
			result: &ExecutionResult{Status: StatusSuccess},
		},
		{
			name:   "failed result",
			result: &ExecutionResult{Status: StatusFailed},
		},
		{
			name: "error from inner handler",
			err:  fmt.Errorf("inner handler error"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := HandlerFunc(func(_ context.Context, _ *event.Event) (*ExecutionResult, error) {
				return tt.result, tt.err
			})

			handler := AlwaysAck(inner, logger.NewTestLogger())

			evt := event.New()
			evt.SetID("test-ack")
			evt.SetType("com.hyperfleet.test")
			evt.SetSource("test")

			err := handler(context.Background(), &evt)
			assert.Nil(t, err, "AlwaysAck must always return nil")
		})
	}
}

// TestPreconditionAPIFailure_ExecutionStatusRemainsFailed verifies that when a precondition
// API call fails, adapter.executionStatus stays "failed" and is not overwritten to "success".
// This is a regression test for a bug where SetSkipped() was called after SetError(),
// resetting executionStatus and causing Health CEL expressions to evaluate incorrectly.
func TestPreconditionAPIFailure_ExecutionStatusRemainsFailed(t *testing.T) {
	// Configure mock to return an error on GET (simulating precondition API failure)
	mockClient := newMockAPIClient()
	mockClient.GetError = fmt.Errorf("connection refused")
	mockClient.GetResponse = nil

	config := &configloader.Config{
		Adapter: configloader.AdapterInfo{
			Name:    "test-adapter",
			Version: "1.0.0",
		},
		Clients: configloader.ClientsConfig{
			HyperfleetAPI: configloader.HyperfleetAPIConfig{
				BaseURL: "http://mock-api:8000",
				Version: "v1",
			},
		},
		Params: []configloader.Parameter{
			{Name: "clusterId", Source: "event.id", Required: true},
		},
		Preconditions: []configloader.Precondition{
			{
				ActionBase: configloader.ActionBase{
					Name: "clusterStatus",
					APICall: &configloader.APICall{
						Method:  "GET",
						URL:     "/clusters/{{ .clusterId }}",
						Timeout: "2s",
					},
				},
			},
		},
	}

	exec, err := NewBuilder().
		WithConfig(config).
		WithAPIClient(mockClient).
		WithTransportClient(k8sclient.NewMockK8sClient()).
		WithLogger(logger.NewTestLogger()).
		Build()
	require.NoError(t, err)

	ctx := logger.WithEventID(context.Background(), "test-precond-fail")
	result := exec.Execute(ctx, map[string]interface{}{"id": "cluster-123"})

	// Verify overall result status is failed
	assert.Equal(t, StatusFailed, result.Status, "expected overall status to be failed")
	assert.True(t, result.ResourcesSkipped, "resources should be skipped on precondition failure")

	// Critical assertion: verify adapter.executionStatus is "failed", not "success"
	require.NotNil(t, result.ExecutionContext, "execution context should be present")
	assert.Equal(t, string(StatusFailed), result.ExecutionContext.Adapter.ExecutionStatus,
		"adapter.executionStatus must remain 'failed' after precondition API failure")

	// Verify error information is preserved
	assert.Equal(t, "PreconditionFailed", result.ExecutionContext.Adapter.ErrorReason,
		"adapter.errorReason should be 'PreconditionFailed'")
	assert.NotEmpty(t, result.ExecutionContext.Adapter.ErrorMessage,
		"adapter.errorMessage should contain the error details")

	// Verify skip metadata is also set (for CEL expressions that check resourcesSkipped)
	assert.True(t, result.ExecutionContext.Adapter.ResourcesSkipped,
		"adapter.resourcesSkipped should be true")
	assert.NotEmpty(t, result.ExecutionContext.Adapter.SkipReason,
		"adapter.skipReason should be set")
}

// TestPreconditionCapture_NamedMapVariable verifies Option 1: the full API response is
// exposed as a named map variable in the capture CEL context under the precondition name,
// enabling safe optional-field access via dig() and has().
func TestPreconditionCapture_NamedMapVariable(t *testing.T) {
	responseWithField := `{"name":"cluster-1","deleted_time":"2026-04-14T10:00:00Z"}`
	responseWithoutField := `{"name":"cluster-1"}`

	tests := []struct {
		name         string
		responseBody string
		wantValue    interface{}
		captureExpr  string
		wantCaptured bool
	}{
		{
			name:         "dig() returns value when field present",
			responseBody: responseWithField,
			captureExpr:  `dig(fetchCluster, "deleted_time") != null`,
			wantValue:    true,
			wantCaptured: true,
		},
		{
			name:         "dig() returns false when field absent - no error",
			responseBody: responseWithoutField,
			captureExpr:  `dig(fetchCluster, "deleted_time") != null`,
			wantValue:    false,
			wantCaptured: true,
		},
		{
			name:         "has() returns true when field present",
			responseBody: responseWithField,
			captureExpr:  `has(fetchCluster.deleted_time)`,
			wantValue:    true,
			wantCaptured: true,
		},
		{
			name:         "has() returns false when field absent - no error",
			responseBody: responseWithoutField,
			captureExpr:  `has(fetchCluster.deleted_time)`,
			wantValue:    false,
			wantCaptured: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockAPIClient()
			mockClient.GetResponse = &hyperfleetapi.Response{
				StatusCode: 200,
				Status:     "200 OK",
				Body:       []byte(tt.responseBody),
			}

			config := &configloader.Config{
				Adapter: configloader.AdapterInfo{Name: "test-adapter", Version: "1.0.0"},
				Clients: configloader.ClientsConfig{
					HyperfleetAPI: configloader.HyperfleetAPIConfig{
						BaseURL: "http://mock-api:8000",
						Version: "v1",
					},
				},
				Preconditions: []configloader.Precondition{
					{
						ActionBase: configloader.ActionBase{
							Name: "fetchCluster",
							APICall: &configloader.APICall{
								Method:  "GET",
								URL:     "/clusters/test",
								Timeout: "2s",
							},
						},
						Capture: []configloader.CaptureField{
							{
								Name:               "is_deleting",
								FieldExpressionDef: configloader.FieldExpressionDef{Expression: tt.captureExpr},
							},
						},
					},
				},
			}

			exec, err := NewBuilder().
				WithConfig(config).
				WithAPIClient(mockClient).
				WithTransportClient(k8sclient.NewMockK8sClient()).
				WithLogger(logger.NewTestLogger()).
				Build()
			require.NoError(t, err)

			ctx := logger.WithEventID(context.Background(), "test-named-map")
			result := exec.Execute(ctx, map[string]interface{}{})

			require.Equal(t, StatusSuccess, result.Status)
			require.Len(t, result.PreconditionResults, 1)
			captured := result.PreconditionResults[0].CapturedFields
			if tt.wantCaptured {
				assert.Equal(t, tt.wantValue, captured["is_deleting"],
					"captured is_deleting should be %v", tt.wantValue)
			}
		})
	}
}

// TestPreconditionCapture_FieldDefault verifies Option 2: when a field: capture is absent
// from the API response, the configured Default is used and no WARN is logged.
// Expression captures are unaffected by Default.
func TestPreconditionCapture_FieldDefault(t *testing.T) {
	responseWithField := `{"name":"cluster-1","status_code":"active"}`
	responseWithoutField := `{"name":"cluster-1"}`

	tests := []struct {
		name         string
		responseBody string
		wantValue    interface{}
		capture      configloader.CaptureField
		wantCaptured bool
	}{
		{
			name:         "field present - default not used",
			responseBody: responseWithField,
			capture: configloader.CaptureField{
				Name:               "statusCode",
				Default:            "unknown",
				FieldExpressionDef: configloader.FieldExpressionDef{Field: "status_code"},
			},
			wantValue:    "active",
			wantCaptured: true,
		},
		{
			name:         "field absent with default - uses default, no WARN",
			responseBody: responseWithoutField,
			capture: configloader.CaptureField{
				Name:               "statusCode",
				Default:            "unknown",
				FieldExpressionDef: configloader.FieldExpressionDef{Field: "status_code"},
			},
			wantValue:    "unknown",
			wantCaptured: true,
		},
		{
			name:         "field absent without default - value is nil",
			responseBody: responseWithoutField,
			capture: configloader.CaptureField{
				Name:               "statusCode",
				FieldExpressionDef: configloader.FieldExpressionDef{Field: "status_code"},
			},
			wantValue:    nil,
			wantCaptured: true,
		},
		{
			name:         "bool default false when field absent",
			responseBody: responseWithoutField,
			capture: configloader.CaptureField{
				Name:               "is_deleting",
				Default:            false,
				FieldExpressionDef: configloader.FieldExpressionDef{Field: "deleted_time"},
			},
			wantValue:    false,
			wantCaptured: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockClient := newMockAPIClient()
			mockClient.GetResponse = &hyperfleetapi.Response{
				StatusCode: 200,
				Status:     "200 OK",
				Body:       []byte(tt.responseBody),
			}

			config := &configloader.Config{
				Adapter: configloader.AdapterInfo{Name: "test-adapter", Version: "1.0.0"},
				Clients: configloader.ClientsConfig{
					HyperfleetAPI: configloader.HyperfleetAPIConfig{
						BaseURL: "http://mock-api:8000",
						Version: "v1",
					},
				},
				Preconditions: []configloader.Precondition{
					{
						ActionBase: configloader.ActionBase{
							Name: "fetchCluster",
							APICall: &configloader.APICall{
								Method:  "GET",
								URL:     "/clusters/test",
								Timeout: "2s",
							},
						},
						Capture: []configloader.CaptureField{tt.capture},
					},
				},
			}

			exec, err := NewBuilder().
				WithConfig(config).
				WithAPIClient(mockClient).
				WithTransportClient(k8sclient.NewMockK8sClient()).
				WithLogger(logger.NewTestLogger()).
				Build()
			require.NoError(t, err)

			ctx := logger.WithEventID(context.Background(), "test-field-default")
			result := exec.Execute(ctx, map[string]interface{}{})

			require.Equal(t, StatusSuccess, result.Status)
			require.Len(t, result.PreconditionResults, 1)
			captured := result.PreconditionResults[0].CapturedFields
			assert.Equal(t, tt.wantValue, captured[tt.capture.Name],
				"captured %s should be %v", tt.capture.Name, tt.wantValue)
		})
	}
}

// helper functions for metrics assertions

func findFamily(families []*dto.MetricFamily, name string) *dto.MetricFamily {
	for _, f := range families {
		if f.GetName() == name {
			return f
		}
	}
	return nil
}

// TestPrecondition404_GracefulStop verifies that when a precondition API call returns 404
// (resource no longer exists), the executor stops gracefully: status remains "success",
// resources are skipped, and no error state is set.
func new404PreconditionConfig() *configloader.Config {
	return &configloader.Config{
		Adapter: configloader.AdapterInfo{
			Name:    "test-adapter",
			Version: "1.0.0",
		},
		Clients: configloader.ClientsConfig{
			HyperfleetAPI: configloader.HyperfleetAPIConfig{
				BaseURL: "http://mock-api:8000",
				Version: "v1",
			},
		},
		Params: []configloader.Parameter{
			{Name: "clusterId", Source: "event.id", Required: true},
		},
		Preconditions: []configloader.Precondition{
			{
				ActionBase: configloader.ActionBase{
					Name: "clusterStatus",
					APICall: &configloader.APICall{
						Method:  "GET",
						URL:     "/clusters/{{ .clusterId }}",
						Timeout: "2s",
					},
				},
			},
		},
	}
}

func new404PostActionConfig() *configloader.Config {
	cfg := new404PreconditionConfig()
	cfg.Post = &configloader.PostConfig{
		PostActions: []configloader.PostAction{
			{
				ActionBase: configloader.ActionBase{
					Name: "reportStatus",
					APICall: &configloader.APICall{
						Method:  "PUT",
						URL:     "/clusters/{{ .clusterId }}/statuses",
						Body:    `{"status":"done"}`,
						Timeout: "2s",
					},
				},
			},
		},
	}
	return cfg
}

func TestPrecondition404_GracefulStop(t *testing.T) {
	mockClient := newMockAPIClient()
	mockClient.GetResponse = mock404Response()

	config := new404PreconditionConfig()
	exec := build404TestExecutor(t, config, mockClient)

	ctx := logger.WithEventID(context.Background(), "test-precond-404")
	result := exec.Execute(ctx, map[string]interface{}{"id": "cluster-gone"})

	assert.Equal(t, StatusSuccess, result.Status,
		"404 on resource should not mark execution as failed")

	assert.True(t, result.ResourcesSkipped, "resources should be skipped")
	assert.Equal(t, ResourceNotFoundReason, result.SkipReason,
		"skip reason should be ResourceNotFoundReason")

	assert.Empty(t, result.Errors, "no errors should be recorded for a 404")
	assert.Equal(t, "", result.ExecutionContext.Adapter.ErrorReason,
		"adapter.errorReason should be empty for a 404")
	assert.Equal(t, "", result.ExecutionContext.Adapter.ErrorMessage,
		"adapter.errorMessage should be empty for a 404")
	assert.Nil(t, result.ExecutionContext.Adapter.ExecutionError,
		"adapter.executionError should be nil for a 404")
}

// TestPostAction404_GracefulHandling verifies that when a post-action API call returns 404
// (resource no longer exists), the executor does not mark the execution as failed.
func TestPostAction404_GracefulHandling(t *testing.T) {
	mockClient := newMockAPIClient()
	mockClient.GetResponse = &hyperfleetapi.Response{
		StatusCode: 200,
		Body:       []byte(`{"id":"cluster-456","status":{"conditions":[{"type":"Reconciled","status":"False"}]}}`),
	}
	mockClient.PutResponse = mock404Response()

	config := new404PostActionConfig()
	exec := build404TestExecutor(t, config, mockClient)

	ctx := logger.WithEventID(context.Background(), "test-postaction-404")
	result := exec.Execute(ctx, map[string]interface{}{"id": "cluster-gone"})

	// Status should be success; 404 in post-actions is gracefully handled
	assert.Equal(t, StatusSuccess, result.Status,
		"404 in post-actions should result in success, not failure")

	// No post-action errors should be recorded
	assert.Nil(t, result.Errors[PhasePostActions],
		"no post_actions error should be recorded for a 404")

	// Verify adapter metadata is consistent
	require.NotNil(t, result.ExecutionContext)
	assert.Equal(t, string(StatusSuccess), result.ExecutionContext.Adapter.ExecutionStatus,
		"adapter.executionStatus should be success for a post-action 404")
	assert.True(t, result.ExecutionContext.Adapter.ResourcesSkipped,
		"adapter.resourcesSkipped should be true")
	assert.Equal(t, ResourceNotFoundReason, result.SkipReason,
		"skip reason should be ResourceNotFound")
	assert.Nil(t, result.ExecutionContext.Adapter.ExecutionError,
		"adapter.executionError should be nil for a post-action 404")
}

// TestPrecondition404_SkipsPostActions verifies that when a precondition returns 404,
// post-actions are never attempted (no PUT/POST requests after the 404 GET).
func TestPrecondition404_SkipsPostActions(t *testing.T) {
	mockClient := newMockAPIClient()
	mockClient.GetResponse = mock404Response()

	config := new404PostActionConfig()
	exec := build404TestExecutor(t, config, mockClient)

	ctx := logger.WithEventID(context.Background(), "test-precond-404-skips-post")
	_ = exec.Execute(ctx, map[string]interface{}{"id": "cluster-gone"})

	// Only the precondition GET should have been made; no PUT for post-actions
	var getCalls, putCalls int
	for _, req := range mockClient.Requests {
		switch req.Method {
		case "GET":
			getCalls++
		case "PUT":
			putCalls++
		}
	}
	assert.Equal(t, 1, getCalls,
		"expected exactly 1 GET call (precondition)")
	assert.Equal(t, 0, putCalls,
		"expected 0 PUT calls (post-actions should not run after precondition 404)")
}

// TestPreconditionFail_PostAction404 verifies that when preconditions fail with a non-404 error
// and post-actions also return 404, the execution status remains failed (not masked by ResourceNotFound)
// and the original error context is preserved.
func TestPreconditionFail_PostAction404(t *testing.T) {
	mockClient := newMockAPIClient()
	mockClient.GetResponse, mockClient.GetError = mockErrorResponse(500)
	mockClient.PutResponse = mock404Response()

	config := new404PostActionConfig()
	exec := build404TestExecutor(t, config, mockClient)

	ctx := logger.WithEventID(context.Background(), "test-precond-fail-post-404")
	result := exec.Execute(ctx, map[string]interface{}{"id": "cluster-gone"})

	// Status stays failed (precondition error is the primary failure)
	assert.Equal(t, StatusFailed, result.Status,
		"status should remain failed from precondition error")

	// SkipReason gets overwritten to ResourceNotFound (resource no longer exists, overrides PreconditionFailed)
	assert.Equal(t, ResourceNotFoundReason, result.SkipReason,
		"skip reason should be ResourceNotFound when post-action 404 overwrites it")
	assert.True(t, result.ResourcesSkipped, "resources should be skipped")

	// The precondition error should still be recorded
	assert.NotNil(t, result.Errors[PhasePreconditions],
		"precondition error should still be recorded")

	// No post-action error recorded (404 is handled gracefully)
	assert.Nil(t, result.Errors[PhasePostActions],
		"post-action 404 should not add an error")
}

// TestPreconditionBrokenURL404_ReportsError verifies that when a
// precondition API call returns 404 due to a misconfigured URL (error
// code HYPERFLEET-NTF-000), the adapter treats it as an error rather than a graceful stop.
func TestPreconditionBrokenURL404_ReportsError(t *testing.T) {
	mockClient := newMockAPIClient()
	mockClient.GetResponse = mockBrokenEndpoint404Response()

	config := new404PreconditionConfig()
	exec := build404TestExecutor(t, config, mockClient)

	ctx := logger.WithEventID(context.Background(), "test-broken-url-404")
	result := exec.Execute(ctx, map[string]interface{}{"id": "cls-123"})

	assert.Equal(t, StatusFailed, result.Status,
		"broken URL 404 should mark execution as failed")

	assert.True(t, result.ResourcesSkipped,
		"resources should be skipped on precondition failure")

	assert.NotEmpty(t, result.Errors,
		"errors should be recorded for a broken URL 404")
}

// TestPostActionBrokenURL404_ReportsError verifies that when a post-action
// API call returns 404 due to a misconfigured URL (error code
// HYPERFLEET-NTF-000), the adapter treats it as an error instead of a graceful stop.
func TestPostActionBrokenURL404_ReportsError(t *testing.T) {
	mockClient := newMockAPIClient()
	mockClient.GetResponse = &hyperfleetapi.Response{
		StatusCode: 200,
		Body: []byte(
			`{"id":"cluster-456","status":{"conditions":` +
				`[{"type":"Reconciled","status":"False"}]}}`,
		),
	}
	mockClient.PutResponse = mockBrokenEndpoint404Response()

	config := new404PostActionConfig()
	exec := build404TestExecutor(t, config, mockClient)

	ctx := logger.WithEventID(context.Background(), "test-post-broken-url-404")
	result := exec.Execute(ctx, map[string]interface{}{"id": "cls-123"})

	assert.Equal(t, StatusFailed, result.Status,
		"broken URL 404 in post-actions should mark execution as failed")

	assert.NotNil(t, result.Errors[PhasePostActions],
		"post-action error should be recorded for a broken URL 404")
}

func getCounterValue(t *testing.T, families []*dto.MetricFamily, metricName, labelName, labelValue string) float64 {
	t.Helper()
	family := findFamily(families, metricName)
	if family == nil {
		t.Fatalf("metric %s not found", metricName)
	}
	for _, m := range family.GetMetric() {
		for _, l := range m.GetLabel() {
			if l.GetName() == labelName && l.GetValue() == labelValue {
				return m.GetCounter().GetValue()
			}
		}
	}
	return 0
}
