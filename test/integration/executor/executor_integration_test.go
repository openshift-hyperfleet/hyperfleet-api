package executorintegrationtest

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/executor"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleetapi"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/test/integration/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	healthStatusHealthy                 = "Healthy"
	healthMessageAllOperationsCompleted = "All adapter operations completed successfully"
)

// getK8sEnvForTest returns the K8s environment for integration testing.
// Uses real K8s from testcontainers. Skips tests if testcontainers are unavailable.
func getK8sEnvForTest(t *testing.T) *K8sTestEnv {
	t.Helper()
	// Use shared K8s environment from testcontainers
	if sharedK8sEnv != nil {
		return sharedK8sEnv
	}
	// Integration tests require real testcontainers - skip if unavailable
	t.Skip("Integration tests require testcontainers (set INTEGRATION_ENVTEST_IMAGE)")
	return nil
}

// createTestEvent creates a CloudEvent for testing
func createTestEvent(clusterID string) *event.Event {
	evt := event.New()
	evt.SetID("test-event-" + clusterID)
	evt.SetType("com.redhat.hyperfleet.cluster.provision")
	evt.SetSource("test")
	evt.SetTime(time.Now())

	eventData := map[string]interface{}{
		"id":            clusterID,
		"resource_type": "cluster",
		"generation":    "gen-001",
		"href":          "/api/v1/clusters/" + clusterID,
	}
	eventDataBytes, _ := json.Marshal(eventData)
	_ = evt.SetData(event.ApplicationJSON, eventDataBytes)

	return &evt
}

// createTestConfig creates a unified Config for executor integration tests.
func createTestConfig(apiBaseURL string) *configloader.Config {
	_ = apiBaseURL // Kept for compatibility; base URL comes from env params.
	return &configloader.Config{
		Adapter: configloader.AdapterInfo{
			Name:    "test-adapter",
			Version: "1.0.0",
		},
		Clients: configloader.ClientsConfig{
			HyperfleetAPI: configloader.HyperfleetAPIConfig{
				Timeout:       10 * time.Second,
				RetryAttempts: 1,
				RetryBackoff:  hyperfleetapi.BackoffConstant,
			},
		},
		Params: []configloader.Parameter{
			{Name: "hyperfleetApiBaseUrl", Source: "env.HYPERFLEET_API_BASE_URL", Required: true},
			{Name: "hyperfleetApiVersion", Source: "env.HYPERFLEET_API_VERSION", Default: "v1", Required: false},
			{Name: "clusterID", Source: "event.id", Required: true},
		},
		Preconditions: []configloader.Precondition{
			{
				ActionBase: configloader.ActionBase{
					Name: "clusterStatus",
					APICall: &configloader.APICall{
						Method:  "GET",
						URL:     "{{ .hyperfleetApiBaseUrl }}/api/{{ .hyperfleetApiVersion }}/clusters/{{ .clusterID }}",
						Timeout: "5s",
					},
				},
				Capture: []configloader.CaptureField{
					{Name: "clusterName", FieldExpressionDef: configloader.FieldExpressionDef{Field: "name"}},
					{
						Name: "reconciledConditionStatus",
						FieldExpressionDef: configloader.FieldExpressionDef{
							Expression: `status.conditions.filter(c, c.type == "Reconciled").size() > 0
  ? status.conditions.filter(c, c.type == "Reconciled")[0].status
  : "False"`,
						},
					},
					{Name: "region", FieldExpressionDef: configloader.FieldExpressionDef{Field: "spec.region"}},
					{Name: "cloudProvider", FieldExpressionDef: configloader.FieldExpressionDef{Field: "spec.provider"}},
					{Name: "vpcId", FieldExpressionDef: configloader.FieldExpressionDef{Field: "spec.vpc_id"}},
				},
				Conditions: []configloader.Condition{
					{Field: "reconciledConditionStatus", Operator: "equals", Value: "True"},
					{Field: "cloudProvider", Operator: "in", Value: []interface{}{"aws", "gcp", "azure"}},
				},
			},
		},
		Resources: []configloader.Resource{},
		Post: &configloader.PostConfig{
			Payloads: []configloader.Payload{
				{
					Name: "clusterStatusPayload",
					Build: map[string]interface{}{
						"conditions": map[string]interface{}{
							"health": map[string]interface{}{
								"status": map[string]interface{}{
									"expression": `adapter.executionStatus == "success" && !adapter.resourcesSkipped`,
								},
								"reason": map[string]interface{}{
									"expression": `adapter.resourcesSkipped ? "PreconditionNotMet" : ` +
										`(adapter.errorReason != "" ? adapter.errorReason : "Healthy")`,
								},
								"message": map[string]interface{}{
									"expression": `adapter.skipReason != "" ? adapter.skipReason : ` +
										`(adapter.errorMessage != "" ? adapter.errorMessage : "All adapter operations completed successfully")`,
								},
							},
						},
						"clusterID": map[string]interface{}{
							"value": "{{ .clusterID }}",
						},
						"clusterName": map[string]interface{}{
							"expression": `clusterName != "" ? clusterName : "unknown"`,
						},
					},
				},
			},
			PostActions: []configloader.PostAction{
				{
					ActionBase: configloader.ActionBase{
						Name: "reportClusterStatus",
						APICall: &configloader.APICall{
							Method:  "PUT",
							URL:     "{{ .hyperfleetApiBaseUrl }}/api/{{ .hyperfleetApiVersion }}/clusters/{{ .clusterID }}/statuses",
							Body:    "{{ .clusterStatusPayload }}",
							Timeout: "5s",
						},
					},
				},
			},
		},
	}
}

func TestExecutor_FullFlow_Success(t *testing.T) {
	// Setup mock API server
	mockAPI := testutil.NewMockAPIServer(t)
	defer mockAPI.Close()

	// Set environment variables
	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	// Get K8s environment from testcontainers
	k8sEnv := getK8sEnvForTest(t)

	// Create config and executor
	config := createTestConfig(mockAPI.URL())
	apiClient, err := hyperfleetapi.NewClient(testLog(),
		hyperfleetapi.WithTimeout(10*time.Second),
		hyperfleetapi.WithRetryAttempts(1),
	)
	require.NoError(t, err, "failed to create API client")

	exec, err := executor.NewBuilder().
		WithConfig(config).
		WithAPIClient(apiClient).
		WithLogger(k8sEnv.Log).
		WithTransportClient(k8sEnv.Client).
		Build()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Create test event
	evt := createTestEvent("cluster-123")

	// Execute
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result := exec.Execute(ctx, evt)

	// Verify result
	require.Equal(t, executor.StatusSuccess, result.Status, "Expected success status; errors=%v", result.Errors)

	// Verify params were extracted
	if result.Params["clusterID"] != "cluster-123" {
		t.Errorf("Expected clusterID 'cluster-123', got '%v'", result.Params["clusterID"])
	}

	// Verify preconditions passed
	if len(result.PreconditionResults) != 1 {
		t.Errorf("Expected 1 precondition result, got %d", len(result.PreconditionResults))
	} else {
		precondResult := result.PreconditionResults[0]
		if !precondResult.Matched {
			t.Errorf("Expected precondition to match, but it didn't")
		}
		if precondResult.CapturedFields["clusterName"] != "test-cluster" {
			t.Errorf("Expected captured clusterName 'test-cluster', got '%v'", precondResult.CapturedFields["clusterName"])
		}
	}

	// Verify post actions executed
	if len(result.PostActionResults) != 1 {
		t.Errorf("Expected 1 post action result, got %d", len(result.PostActionResults))
	} else {
		postResult := result.PostActionResults[0]
		if postResult.Status != executor.StatusSuccess {
			t.Errorf("Expected post action success, got %s: %v", postResult.Status, postResult.Error)
		}
		if !postResult.APICallMade {
			t.Error("Expected API call to be made in post action")
		}
	}

	// Verify API calls were made
	requests := mockAPI.GetRequests()
	if len(requests) < 2 {
		t.Errorf("Expected at least 2 API requests (precondition + post action), got %d", len(requests))
	}

	// Verify status was posted with correct template expression values
	statusResponses := mockAPI.GetStatusResponses()
	if len(statusResponses) != 1 {
		t.Errorf("Expected 1 status response, got %d", len(statusResponses))
	} else {
		status := statusResponses[0]
		t.Logf("Status payload received: %+v", status)

		// Check that status contains expected fields with correct values from template
		if conditions, ok := status["conditions"].(map[string]interface{}); ok {
			if health, ok := conditions["health"].(map[string]interface{}); ok {
				// Status should be true (adapter.executionStatus == "success")
				if health["status"] != true {
					t.Errorf("Expected health.status to be true, got %v", health["status"])
				}

				// Reason should be "Healthy" (default since no adapter.errorReason)
				if reason, ok := health["reason"].(string); ok {
					if reason != healthStatusHealthy {
						t.Errorf("Expected health.reason to be '%s', got '%s'", healthStatusHealthy, reason)
					}
				} else {
					t.Error("Expected health.reason to be a string")
				}

				// Message should be default success message (no adapter.errorMessage)
				if message, ok := health["message"].(string); ok {
					if message != healthMessageAllOperationsCompleted {
						t.Errorf("Expected health.message to be default success message, got '%s'", message)
					}
				} else {
					t.Error("Expected health.message to be a string")
				}
			} else {
				t.Error("Expected health condition in status")
			}
		} else {
			t.Error("Expected conditions in status payload")
		}
	}

	t.Logf("Execution completed successfully")
}

func TestExecutor_PreconditionNotMet(t *testing.T) {
	// Setup mock API server that returns a cluster in "Terminating" phase
	mockAPI := testutil.NewMockAPIServer(t)
	defer mockAPI.Close()

	// Set cluster to a phase that doesn't match conditions
	mockAPI.SetClusterResponse(map[string]interface{}{
		"id":   "test-cluster-id",
		"name": "test-cluster",
		"kind": "Cluster",
		"spec": map[string]interface{}{
			"region":   "us-east-1",
			"provider": "aws",
			"vpc_id":   "vpc-12345",
		},
		"status": map[string]interface{}{
			"phase": "Terminating", // This won't match the condition
		},
	})

	// Set environment variables
	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	// Get K8s environment from testcontainers
	k8sEnv := getK8sEnvForTest(t)

	// Create config and executor
	config := createTestConfig(mockAPI.URL())
	apiClient, err := hyperfleetapi.NewClient(k8sEnv.Log)
	assert.NoError(t, err)
	exec, err := executor.NewBuilder().
		WithConfig(config).
		WithAPIClient(apiClient).
		WithLogger(k8sEnv.Log).
		WithTransportClient(k8sEnv.Client).
		Build()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Execute
	evt := createTestEvent("cluster-456")
	ctx := context.Background()
	result := exec.Execute(ctx, evt)

	// Verify result - should be success with resources skipped (precondition not met is valid outcome)
	if result.Status != executor.StatusSuccess {
		t.Errorf("Expected success status (precondition not met is valid), got %s", result.Status)
	}
	if !result.ResourcesSkipped {
		t.Error("Expected ResourcesSkipped to be true")
	}

	// Verify precondition was not matched
	if len(result.PreconditionResults) != 1 {
		t.Errorf("Expected 1 precondition result, got %d", len(result.PreconditionResults))
	} else if result.PreconditionResults[0].Matched {
		t.Error("Expected precondition to NOT match")
	}

	// Post actions should still execute (to report the error)
	if len(result.PostActionResults) != 1 {
		t.Errorf("Expected 1 post action result (error reporting), got %d", len(result.PostActionResults))
	}

	// Verify the status payload reflects skipped execution (not an error)
	statusResponses := mockAPI.GetStatusResponses()
	if len(statusResponses) == 1 {
		status := statusResponses[0]
		t.Logf("Status payload: %+v", status)

		if conditions, ok := status["conditions"].(map[string]interface{}); ok {
			if health, ok := conditions["health"].(map[string]interface{}); ok {
				// Health status should be false because adapter.executionStatus != "success"
				if health["status"] != false {
					t.Errorf("Expected health.status to be false for precondition not met, got %v", health["status"])
				}

				// Reason should contain error (from adapter.errorReason, not "Healthy")
				if reason, ok := health["reason"].(string); ok {
					if reason == healthStatusHealthy {
						t.Errorf("Expected health.reason to indicate precondition not met, got '%s'", healthStatusHealthy)
					}
					t.Logf("Health reason: %s", reason)
				}

				// Message should contain error explanation (from adapter.errorMessage)
				if message, ok := health["message"].(string); ok {
					if message == healthMessageAllOperationsCompleted {
						t.Error("Expected health.message to explain precondition not met, got default success message")
					}
					t.Logf("Health message: %s", message)
				}
			}
		}
	}

	// Verify execution context shows adapter is healthy (executionStatus = "success", resources skipped)
	if result.ExecutionContext != nil {
		if result.ExecutionContext.Adapter.ExecutionStatus != string(executor.StatusSuccess) {
			t.Errorf("Expected adapter.executionStatus to be 'success', got '%s'",
				result.ExecutionContext.Adapter.ExecutionStatus)
		}
		// No executionError should be set - precondition not matching is not an error
		if result.ExecutionContext.Adapter.ExecutionError != nil {
			t.Errorf("Expected no executionError for precondition not met, got %+v",
				result.ExecutionContext.Adapter.ExecutionError)
		}
	}

	t.Logf("Execution completed with expected precondition skip")
}

func TestExecutor_PreconditionAPIFailure(t *testing.T) {
	// Setup mock API server that fails precondition API call
	mockAPI := testutil.NewMockAPIServer(t)
	defer mockAPI.Close()
	mockAPI.SetFailPrecondition(true) // API will return 404

	// Set environment variables
	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	// Get K8s environment from testcontainers
	k8sEnv := getK8sEnvForTest(t)

	// Create config and executor
	config := createTestConfig(mockAPI.URL())
	apiClient, err := hyperfleetapi.NewClient(testLog(),
		hyperfleetapi.WithRetryAttempts(1),
	)
	assert.NoError(t, err)

	exec, err := executor.NewBuilder().
		WithConfig(config).
		WithAPIClient(apiClient).
		WithLogger(k8sEnv.Log).
		WithTransportClient(k8sEnv.Client).
		Build()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Execute
	evt := createTestEvent("cluster-notfound")
	ctx := context.Background()
	result := exec.Execute(ctx, evt)

	// Verify result - should be failed (API error)
	if result.Status != executor.StatusFailed {
		t.Errorf("Expected failed status, got %s", result.Status)
	}

	require.NotEmpty(t, result.Errors, "expected errors to be set")

	// Verify resources were not processed due to precondition failure
	if len(result.ResourceResults) > 0 {
		t.Errorf("Expected no resources to be processed on API error, got %d", len(result.ResourceResults))
	}

	// Verify post actions still executed to report the error
	if len(result.PostActionResults) != 1 {
		t.Errorf("Expected 1 post action result (error reporting), got %d", len(result.PostActionResults))
	}

	// Verify status payload contains adapter error fields
	statusResponses := mockAPI.GetStatusResponses()
	if len(statusResponses) != 1 {
		t.Errorf("Expected 1 status response, got %d", len(statusResponses))
	} else {
		status := statusResponses[0]
		t.Logf("Error status payload: %+v", status)

		// Verify health condition with adapter.xxx fields
		if conditions, ok := status["conditions"].(map[string]interface{}); ok {
			if health, ok := conditions["health"].(map[string]interface{}); ok {
				// Status should be false because adapter.executionStatus != "success"
				if health["status"] != false {
					t.Errorf("Expected health.status to be false for API error, got %v", health["status"])
				}

				// Reason should contain error reason (not "Healthy")
				if reason, ok := health["reason"].(string); ok {
					if reason == healthStatusHealthy {
						t.Errorf("Expected health.reason to contain error, got '%s'", healthStatusHealthy)
					}
					t.Logf("Health reason: %s", reason)
				} else {
					t.Error("Expected health.reason to be a string")
				}

				// Message should contain error message (not default success message)
				if message, ok := health["message"].(string); ok {
					if message == healthMessageAllOperationsCompleted {
						t.Error("Expected health.message to contain error, got default success message")
					}
					t.Logf("Health message: %s", message)
				} else {
					t.Error("Expected health.message to be a string")
				}
			} else {
				t.Error("Expected health condition in status")
			}
		} else {
			t.Error("Expected conditions in status payload")
		}
	}

	t.Logf("Execution failed as expected: errors=%v", result.Errors)
}

func TestExecutor_CELExpressionEvaluation(t *testing.T) {
	// Setup mock API server
	mockAPI := testutil.NewMockAPIServer(t)
	defer mockAPI.Close()

	// Set environment variables
	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	// Create config with CEL expression precondition
	config := createTestConfig(mockAPI.URL())
	config.Preconditions = []configloader.Precondition{
		{
			ActionBase: configloader.ActionBase{
				Name: "clusterStatus",
				APICall: &configloader.APICall{
					Method:  "GET",
					URL:     "{{ .hyperfleetApiBaseUrl }}/api/{{ .hyperfleetApiVersion }}/clusters/{{ .clusterID }}",
					Timeout: "5s",
				},
			},
			Capture: []configloader.CaptureField{
				{Name: "clusterName", FieldExpressionDef: configloader.FieldExpressionDef{Field: "name"}},
				{
					Name: "reconciledConditionStatus",
					FieldExpressionDef: configloader.FieldExpressionDef{
						Expression: `status.conditions.filter(c, c.type == "Reconciled").size() > 0 ? ` +
							`status.conditions.filter(c, c.type == "Reconciled")[0].status : "False"`,
					},
				},
				{Name: "nodeCount", FieldExpressionDef: configloader.FieldExpressionDef{Field: "spec.node_count"}},
			},
			// Use CEL expression instead of structured conditions
			Expression: `reconciledConditionStatus == "True" && nodeCount >= 3`,
		},
	}

	apiClient, err := hyperfleetapi.NewClient(testLog())

	assert.NoError(t, err)

	exec, err := executor.NewBuilder().
		WithConfig(config).
		WithAPIClient(apiClient).
		WithLogger(getK8sEnvForTest(t).Log).
		WithTransportClient(getK8sEnvForTest(t).Client).
		Build()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Execute
	evt := createTestEvent("cluster-cel-test")
	ctx := context.Background()
	result := exec.Execute(ctx, evt)

	// Verify CEL evaluation passed
	require.Equal(t, executor.StatusSuccess, result.Status, "Expected success status; errors=%v", result.Errors)

	if len(result.PreconditionResults) != 1 {
		t.Fatalf("Expected 1 precondition result, got %d", len(result.PreconditionResults))
	}

	precondResult := result.PreconditionResults[0]
	if !precondResult.Matched {
		t.Error("Expected CEL expression to evaluate to true")
	}

	// Verify CEL result was recorded
	if precondResult.CELResult == nil {
		t.Error("Expected CEL result to be recorded")
	} else {
		t.Logf("CEL expression result: matched=%v, value=%v", precondResult.CELResult.Matched, precondResult.CELResult.Value)
	}
}

func TestExecutor_MultipleMessages(t *testing.T) {
	// Test that multiple messages can be processed with isolated contexts
	mockAPI := testutil.NewMockAPIServer(t)
	defer mockAPI.Close()

	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	config := createTestConfig(mockAPI.URL())
	apiClient, err := hyperfleetapi.NewClient(testLog())
	assert.NoError(t, err)
	exec, err := executor.NewBuilder().
		WithConfig(config).
		WithAPIClient(apiClient).
		WithLogger(getK8sEnvForTest(t).Log).
		WithTransportClient(getK8sEnvForTest(t).Client).
		Build()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Process multiple messages
	clusterIDs := []string{"cluster-a", "cluster-b", "cluster-c"}
	results := make([]*executor.ExecutionResult, len(clusterIDs))

	for i, clusterID := range clusterIDs {
		evt := createTestEvent(clusterID)
		results[i] = exec.Execute(context.Background(), evt)
	}

	// Verify all succeeded with isolated params
	for i, result := range results {
		if result.Status != executor.StatusSuccess {
			require.Empty(t, result.Errors, "Message %d failed: errors=%v", i, result.Errors)
			continue
		}

		// Verify each message had its own clusterID
		expectedClusterID := clusterIDs[i]
		if result.Params["clusterID"] != expectedClusterID {
			t.Errorf("Message %d: expected clusterID '%s', got '%v'", i, expectedClusterID, result.Params["clusterID"])
		}
	}

	// Verify we got separate status posts for each message
	statusResponses := mockAPI.GetStatusResponses()
	if len(statusResponses) != len(clusterIDs) {
		t.Errorf("Expected %d status responses, got %d", len(clusterIDs), len(statusResponses))
	}

	t.Logf("Successfully processed %d messages with isolated contexts", len(clusterIDs))
}

func TestExecutor_Handler_Integration(t *testing.T) {
	// Test the CreateHandler function that would be used with broker
	mockAPI := testutil.NewMockAPIServer(t)
	defer mockAPI.Close()

	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	config := createTestConfig(mockAPI.URL())
	apiClient, err := hyperfleetapi.NewClient(testLog())
	assert.NoError(t, err)
	exec, err := executor.NewBuilder().
		WithConfig(config).
		WithAPIClient(apiClient).
		WithLogger(getK8sEnvForTest(t).Log).
		WithTransportClient(getK8sEnvForTest(t).Client).
		Build()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Get the handler function
	handler := executor.AlwaysAck(exec.CreateHandler())

	// Simulate broker calling the handler
	evt := createTestEvent("cluster-handler-test")
	ctx := context.Background()

	err = handler(ctx, evt)
	// Handler should return nil on success
	if err != nil {
		t.Errorf("Handler returned error: %v", err)
	}

	// Verify API calls were made
	requests := mockAPI.GetRequests()
	if len(requests) < 2 {
		t.Errorf("Expected at least 2 API requests, got %d", len(requests))
	}

	t.Log("Handler integration test passed")
}

func TestExecutor_Handler_PreconditionNotMet_ReturnsNil(t *testing.T) {
	// When preconditions aren't met, handler should return nil (not an error)
	// because it's expected behavior, not a system failure
	mockAPI := testutil.NewMockAPIServer(t)
	defer mockAPI.Close()

	mockAPI.SetClusterResponse(map[string]interface{}{
		"id":     "test-id",
		"name":   "test",
		"kind":   "Cluster",
		"spec":   map[string]interface{}{"region": "us-east-1", "provider": "aws"},
		"status": map[string]interface{}{"phase": "Terminating"}, // Won't match
	})

	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	config := createTestConfig(mockAPI.URL())
	apiClient, err := hyperfleetapi.NewClient(testLog())
	assert.NoError(t, err)
	exec, err := executor.NewBuilder().
		WithConfig(config).
		WithAPIClient(apiClient).
		WithLogger(getK8sEnvForTest(t).Log).
		WithTransportClient(getK8sEnvForTest(t).Client).
		Build()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	handler := executor.AlwaysAck(exec.CreateHandler())
	evt := createTestEvent("cluster-skip")

	// Handler should return nil even when precondition not met
	err = handler(context.Background(), evt)
	if err != nil {
		t.Errorf("Handler should return nil for precondition not met, got: %v", err)
	}

	t.Log("Handler correctly returns nil for skipped execution")
}

func TestExecutor_ContextCancellation(t *testing.T) {
	// Test that context cancellation is respected
	mockAPI := testutil.NewMockAPIServer(t)
	defer mockAPI.Close()

	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	config := createTestConfig(mockAPI.URL())
	apiClient, err := hyperfleetapi.NewClient(testLog())
	assert.NoError(t, err)
	exec, err := executor.NewBuilder().
		WithConfig(config).
		WithAPIClient(apiClient).
		WithLogger(getK8sEnvForTest(t).Log).
		WithTransportClient(getK8sEnvForTest(t).Client).
		Build()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Create already canceled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	evt := createTestEvent("cluster-canceled")
	result := exec.Execute(ctx, evt)

	// Should fail due to context cancellation
	// Note: The exact behavior depends on where cancellation is checked
	require.NotEmpty(t, result.Errors, "Expected error for canceled context")
	t.Logf("Result with canceled context: status=%s, errors=%v", result.Status, result.Errors)
}

func TestExecutor_MissingRequiredParam(t *testing.T) {
	// Test that missing required params cause failure
	mockAPI := testutil.NewMockAPIServer(t)
	defer mockAPI.Close()

	// Set env var initially so executor can be created
	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	config := createTestConfig(mockAPI.URL())
	apiClient, err := hyperfleetapi.NewClient(testLog())
	assert.NoError(t, err)

	exec, err := executor.NewBuilder().
		WithConfig(config).
		WithAPIClient(apiClient).
		WithLogger(getK8sEnvForTest(t).Log).
		WithTransportClient(getK8sEnvForTest(t).Client).
		Build()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Unset the env var after executor creation
	if unsetErr := os.Unsetenv("HYPERFLEET_API_BASE_URL"); unsetErr != nil {
		t.Fatalf("Failed to unset env var: %v", unsetErr)
	}

	evt := createTestEvent("cluster-missing-param")
	result := exec.Execute(context.Background(), evt)

	// Should fail during param extraction
	if result.Status != executor.StatusFailed {
		t.Errorf("Expected failed status for missing required param, got %s", result.Status)
	}

	if result.CurrentPhase != executor.PhaseParamExtraction {
		t.Errorf("Expected failure in param_extraction phase, got %s", result.CurrentPhase)
	}

	// Post-actions DO NOT execute for param extraction failures
	// We skip all phases to avoid processing invalid events
	if len(result.PostActionResults) != 0 {
		t.Errorf("Expected 0 post-actions for param extraction failure, got %d", len(result.PostActionResults))
	}

	// Preconditions should not execute
	if len(result.PreconditionResults) != 0 {
		t.Errorf("Expected 0 preconditions for param extraction failure, got %d", len(result.PreconditionResults))
	}

	// Resources should not execute
	if len(result.ResourceResults) != 0 {
		t.Errorf("Expected 0 resources for param extraction failure, got %d", len(result.ResourceResults))
	}

	// Test handler behavior: should ACK (not NACK) invalid events
	handler := executor.AlwaysAck(exec.CreateHandler())
	err = handler(context.Background(), evt)
	if err != nil {
		t.Errorf("Handler should ACK (return nil) for param extraction failures, got error: %v", err)
	}

	require.NotEmpty(t, result.Errors, "Expected errors for missing required param")
	t.Logf("Correctly failed with missing required param and ACKed (not NACKed): errors=%v", result.Errors)
}

// TestExecutor_InvalidEventJSON tests handling of malformed event data
func TestExecutor_InvalidEventJSON(t *testing.T) {
	mockAPI := testutil.NewMockAPIServer(t)
	defer mockAPI.Close()

	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	config := createTestConfig(mockAPI.URL())
	apiClient, err := hyperfleetapi.NewClient(testLog())
	assert.NoError(t, err)

	exec, err := executor.NewBuilder().
		WithConfig(config).
		WithAPIClient(apiClient).
		WithLogger(getK8sEnvForTest(t).Log).
		WithTransportClient(getK8sEnvForTest(t).Client).
		Build()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Create event with invalid JSON data
	evt := event.New()
	evt.SetID("invalid-event-123")
	evt.SetType("com.redhat.hyperfleet.cluster.provision")
	evt.SetSource("test")

	// Set malformed JSON data that can't be parsed
	invalidJSON := []byte("this is not valid JSON {{{")
	_ = evt.SetData(event.ApplicationJSON, invalidJSON)

	result := exec.Execute(context.Background(), &evt)

	// Should fail during param extraction (JSON parsing)
	assert.Equal(t, executor.StatusFailed, result.Status, "Should fail with invalid JSON")
	assert.Equal(t, executor.PhaseParamExtraction, result.CurrentPhase, "Should fail in param extraction phase")
	require.NotEmpty(t, result.Errors, "Should have error set")
	t.Logf("Invalid JSON errors: %v", result.Errors)

	// All phases should be skipped for invalid events
	assert.Empty(t, result.PostActionResults, "Post-actions should not execute for invalid event")
	assert.Empty(t, result.PreconditionResults, "Preconditions should not execute for invalid event")
	assert.Empty(t, result.ResourceResults, "Resources should not execute for invalid event")

	// Test handler behavior: should ACK (not NACK) invalid events
	handler := executor.AlwaysAck(exec.CreateHandler())
	err = handler(context.Background(), &evt)
	assert.Nil(t, err, "Handler should ACK (return nil) for invalid events, not NACK")

	t.Log("Expected behavior: Invalid event is ACKed (not NACKed), all phases skipped")
}

// TestExecutor_MissingEventFields tests handling of events missing required fields
func TestExecutor_MissingEventFields(t *testing.T) {
	mockAPI := testutil.NewMockAPIServer(t)
	defer mockAPI.Close()

	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	config := createTestConfig(mockAPI.URL())
	apiClient, err := hyperfleetapi.NewClient(testLog())
	assert.NoError(t, err)
	exec, err := executor.NewBuilder().
		WithConfig(config).
		WithAPIClient(apiClient).
		WithLogger(getK8sEnvForTest(t).Log).
		WithTransportClient(getK8sEnvForTest(t).Client).
		Build()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Create event missing required field (id)
	evt := event.New()
	evt.SetID("missing-field-event")
	evt.SetType("com.redhat.hyperfleet.cluster.provision")
	evt.SetSource("test")

	eventData := map[string]interface{}{
		// Missing id (required)
	}
	eventDataBytes, _ := json.Marshal(eventData)
	_ = evt.SetData(event.ApplicationJSON, eventDataBytes)

	result := exec.Execute(context.Background(), &evt)

	// Should fail during param extraction (missing required param from event)
	assert.Equal(t, executor.StatusFailed, result.Status, "Should fail with missing required field")
	assert.Equal(t, executor.PhaseParamExtraction, result.CurrentPhase, "Should fail in param extraction")
	require.NotEmpty(t, result.Errors)
	// Expect failure in param extraction phase
	errPhase := result.Errors[executor.PhaseParamExtraction]
	require.Error(t, errPhase)
	assert.Contains(t, errPhase.Error(), "clusterID", "Error should mention missing clusterID")
	t.Logf("Missing field error: %v", errPhase)

	// All phases should be skipped for events with missing required fields
	assert.Empty(t, result.PostActionResults, "Post-actions should not execute for missing required field")
	assert.Empty(t, result.PreconditionResults, "Preconditions should not execute for missing required field")
	assert.Empty(t, result.ResourceResults, "Resources should not execute for missing required field")

	// Test handler behavior: should ACK (not NACK) events with missing required fields
	handler := executor.AlwaysAck(exec.CreateHandler())
	errPhase = handler(context.Background(), &evt)
	assert.Nil(t, errPhase, "Handler should ACK (return nil) for missing required fields, not NACK")

	t.Log("Expected behavior: Event with missing required field is ACKed (not NACKed), all phases skipped")
}

// TestExecutor_LogAction tests log actions in preconditions and post-actions
func TestExecutor_LogAction(t *testing.T) {
	mockAPI := testutil.NewMockAPIServer(t)
	defer mockAPI.Close()

	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	// Create a logger that captures log messages for assertions
	log, logCapture := logger.NewCaptureLogger()

	// Create config with log actions in preconditions and post-actions
	config := &configloader.Config{
		Adapter: configloader.AdapterInfo{
			Name:    "log-test-adapter",
			Version: "1.0.0",
		},
		Clients: configloader.ClientsConfig{
			HyperfleetAPI: configloader.HyperfleetAPIConfig{
				Timeout: 10 * time.Second, RetryAttempts: 1,
			},
		},
		Params: []configloader.Parameter{
			{Name: "hyperfleetApiBaseUrl", Source: "env.HYPERFLEET_API_BASE_URL", Required: true},
			{Name: "hyperfleetApiVersion", Default: "v1"},
			{Name: "clusterID", Source: "event.id", Required: true},
		},
		Preconditions: []configloader.Precondition{
			{
				// Log action only - no API call or conditions
				ActionBase: configloader.ActionBase{
					Name: "logStart",
					Log: &configloader.LogAction{
						Message: "Starting processing for cluster {{ .clusterID }}",
						Level:   "info",
					},
				},
			},
			{
				// Log action before API call
				ActionBase: configloader.ActionBase{
					Name: "logBeforeAPICall",
					Log: &configloader.LogAction{
						Message: "About to check cluster status for {{ .clusterID }}",
						Level:   "debug",
					},
				},
			},
			{
				ActionBase: configloader.ActionBase{
					Name: "checkCluster",
					APICall: &configloader.APICall{
						Method: "GET",
						URL:    "{{ .hyperfleetApiBaseUrl }}/api/{{ .hyperfleetApiVersion }}/clusters/{{ .clusterID }}",
					},
				},
				Capture: []configloader.CaptureField{
					{
						Name: "reconciledConditionStatus",
						FieldExpressionDef: configloader.FieldExpressionDef{
							Expression: `status.conditions.filter(c, c.type == "Reconciled").size() > 0 ? ` +
								`status.conditions.filter(c, c.type == "Reconciled")[0].status : "False"`,
						},
					},
				},
				Conditions: []configloader.Condition{
					{Field: "reconciledConditionStatus", Operator: "equals", Value: "True"},
				},
			},
		},
		Post: &configloader.PostConfig{
			PostActions: []configloader.PostAction{
				{
					// Log action in post-actions
					ActionBase: configloader.ActionBase{
						Name: "logCompletion",
						Log: &configloader.LogAction{
							Message: "Completed processing cluster {{ .clusterID }} with resource {{ .resourceId }}",
							Level:   "info",
						},
					},
				},
				{
					// Log with warning level
					ActionBase: configloader.ActionBase{
						Name: "logWarning",
						Log: &configloader.LogAction{
							Message: "This is a warning for cluster {{ .clusterID }}",
							Level:   "warning",
						},
					},
				},
			},
		},
	}

	apiClient, err := hyperfleetapi.NewClient(testLog())
	assert.NoError(t, err)
	exec, err := executor.NewBuilder().
		WithConfig(config).
		WithAPIClient(apiClient).
		WithLogger(log).
		WithTransportClient(getK8sEnvForTest(t).Client).
		Build()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Execute
	evt := createTestEvent("log-test-clusterx")
	result := exec.Execute(context.Background(), evt)

	// Should succeed
	if result.Status != executor.StatusSuccess {
		t.Fatalf("Expected success, got %s: errors=%v", result.Status, result.Errors)
	}

	// Verify log messages were captured
	capturedLogs := logCapture.Messages()
	t.Logf("Captured logs:\n%s", capturedLogs)

	// Check for expected log messages (with [config] prefix)
	expectedLogs := []string{
		"[config] Starting processing for cluster log-test-cluster",
		"[config] About to check cluster status for log-test-cluster",
		"[config] Completed processing cluster log-test-cluster with resource log-test-resource",
		"[config] This is a warning for cluster log-test-cluster",
	}

	for _, expected := range expectedLogs {
		if !logCapture.Contains(expected) {
			t.Errorf("Expected log message not found: %s", expected)
		}
	}

	// Verify preconditions executed (including log-only ones)
	if len(result.PreconditionResults) != 3 {
		t.Errorf("Expected 3 precondition results, got %d", len(result.PreconditionResults))
	}

	// Verify post actions executed
	if len(result.PostActionResults) != 2 {
		t.Errorf("Expected 2 post action results, got %d", len(result.PostActionResults))
	}

	t.Logf("Log action test completed successfully")
}

// TestExecutor_PostActionAPIFailure tests handling of post action API failures (4xx/5xx responses)
func TestExecutor_PostActionAPIFailure(t *testing.T) {
	// Setup mock API server that fails post action API calls
	mockAPI := testutil.NewMockAPIServer(t)
	defer mockAPI.Close()

	// Preconditions will succeed, but post action API call will fail with 500
	mockAPI.SetFailPostAction(true)

	// Set environment variables
	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	// Create config and executor
	config := createTestConfig(mockAPI.URL())
	apiClient, err := hyperfleetapi.NewClient(testLog(),
		hyperfleetapi.WithRetryAttempts(1),
	)
	assert.NoError(t, err)
	exec, err := executor.NewBuilder().
		WithConfig(config).
		WithAPIClient(apiClient).
		WithLogger(getK8sEnvForTest(t).Log).
		WithTransportClient(getK8sEnvForTest(t).Client).
		Build()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Execute
	evt := createTestEvent("cluster-post-fail")
	ctx := context.Background()
	result := exec.Execute(ctx, evt)

	// Verify result - should be failed due to post action API error
	assert.Equal(t, executor.StatusFailed, result.Status, "Expected failed status for post action API error")
	require.NotEmpty(t, result.Errors, "Expected error to be set")
	t.Logf("Post action API failure errors: %v", result.Errors)

	// Verify preconditions passed successfully
	assert.Equal(t, 1, len(result.PreconditionResults), "Expected 1 precondition result")
	if len(result.PreconditionResults) > 0 {
		assert.True(t, result.PreconditionResults[0].Matched, "Expected precondition to match")
	}

	// Verify post action was attempted and failed
	assert.Equal(t, 1, len(result.PostActionResults), "Expected 1 post action result")
	if len(result.PostActionResults) > 0 {
		postResult := result.PostActionResults[0]
		assert.Equal(t, executor.StatusFailed, postResult.Status, "Expected post action to fail")
		assert.NotNil(t, postResult.Error, "Expected post action error to be set")
		assert.True(t, postResult.APICallMade, "Expected API call to be made")
		assert.Equal(t, http.StatusInternalServerError, postResult.HTTPStatus, "Expected 500 status code")

		// Verify error contains status code and response body
		errStr := postResult.Error.Error()
		assert.Contains(t, errStr, "500", "Error should contain status code")
		assert.Contains(t, errStr, "Internal Server Error", "Error should contain status text")
		// The response body should be included in the error
		t.Logf("Post action error message: %s", errStr)
	}

	// Verify ExecutionError was populated in execution context
	assert.NotNil(t, result.ExecutionContext, "Expected execution context to be set")
	if result.ExecutionContext != nil {
		assert.NotNil(t, result.ExecutionContext.Adapter.ExecutionError, "Expected ExecutionError to be populated")
		if result.ExecutionContext.Adapter.ExecutionError != nil {
			execErr := result.ExecutionContext.Adapter.ExecutionError
			assert.Equal(t, "post_actions", execErr.Phase, "Expected error in post_actions phase")
			assert.Equal(t, "reportClusterStatus", execErr.Step, "Expected error in reportClusterStatus step")
			assert.Contains(t, execErr.Message, "500", "Expected error message to contain 500 status code")
			t.Logf("ExecutionError: phase=%s, step=%s, message=%s",
				execErr.Phase, execErr.Step, execErr.Message)
		}
	}

	// Verify the phase is post_actions
	assert.Equal(t, executor.PhasePostActions, result.CurrentPhase, "Expected failure in post_actions phase")

	// Verify precondition API was called, but status PUT failed
	requests := mockAPI.GetRequests()
	assert.GreaterOrEqual(t, len(requests), 2, "Expected at least 2 API calls (GET cluster + PUT status)")

	// Find the status POST request
	var statusPutFound bool
	for _, req := range requests {
		if req.Method == http.MethodPut && strings.Contains(req.Path, "/statuses") {
			statusPutFound = true
			t.Logf("Status PUT was attempted: %s %s", req.Method, req.Path)
		}
	}
	assert.True(t, statusPutFound, "Expected status PUT to be attempted")

	// No status should be successfully stored since PUT failed
	statusResponses := mockAPI.GetStatusResponses()
	assert.Empty(t, statusResponses, "Expected no successful status responses due to API failure")

	t.Logf("Post action API failure test completed successfully")
}

// TestExecutor_ExecutionError_CELAccess tests that adapter.executionError is accessible via CEL expressions
func TestExecutor_ExecutionError_CELAccess(t *testing.T) {
	// Setup mock API server that fails precondition to trigger an error
	mockAPI := testutil.NewMockAPIServer(t)
	defer mockAPI.Close()
	mockAPI.SetFailPrecondition(true) // Will return 404 for cluster lookup

	// Set environment variables
	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	// Create config with CEL expressions that access adapter.executionError
	config := &configloader.Config{
		Adapter: configloader.AdapterInfo{
			Name:    "executionError-cel-test",
			Version: "1.0.0",
		},
		Clients: configloader.ClientsConfig{
			HyperfleetAPI: configloader.HyperfleetAPIConfig{
				Timeout: 10 * time.Second, RetryAttempts: 1, RetryBackoff: hyperfleetapi.BackoffConstant,
			},
		},
		Params: []configloader.Parameter{
			{Name: "hyperfleetApiBaseUrl", Source: "env.HYPERFLEET_API_BASE_URL", Required: true},
			{Name: "hyperfleetApiVersion", Default: "v1"},
			{Name: "clusterID", Source: "event.id", Required: true},
		},
		Preconditions: []configloader.Precondition{
			{
				ActionBase: configloader.ActionBase{
					Name: "clusterStatus",
					APICall: &configloader.APICall{
						Method:  "GET",
						URL:     "{{ .hyperfleetApiBaseUrl }}/api/{{ .hyperfleetApiVersion }}/clusters/{{ .clusterID }}",
						Timeout: "5s",
					},
				},
				Capture: []configloader.CaptureField{
					{
						Name: "reconciledConditionStatus",
						FieldExpressionDef: configloader.FieldExpressionDef{
							Expression: `status.conditions.filter(c, c.type == "Reconciled").size() > 0 ? ` +
								`status.conditions.filter(c, c.type == "Reconciled")[0].status : "False"`,
						},
					},
				},
				Conditions: []configloader.Condition{
					{Field: "reconciledConditionStatus", Operator: "equals", Value: "True"},
				},
			},
		},
		Resources: []configloader.Resource{},
		Post: &configloader.PostConfig{
			Payloads: []configloader.Payload{
				{
					Name: "errorReportPayload",
					Build: map[string]interface{}{
						// Test accessing adapter.executionError fields via CEL
						"hasError": map[string]interface{}{
							"expression": "has(adapter.executionError) && adapter.executionError != null",
						},
						"errorPhase": map[string]interface{}{
							"expression": "has(adapter.executionError) && adapter.executionError != null ? " +
								`adapter.executionError.phase : "no_error"`,
						},
						"errorStep": map[string]interface{}{
							"expression": "has(adapter.executionError) && adapter.executionError != null ? " +
								`adapter.executionError.step : "no_step"`,
						},
						"errorMessage": map[string]interface{}{
							"expression": "has(adapter.executionError) && adapter.executionError != null ? " +
								`adapter.executionError.message : "no_message"`,
						},
						// Also test that other adapter fields still work
						"executionStatus": map[string]interface{}{
							"expression": "adapter.executionStatus",
						},
						"errorReason": map[string]interface{}{
							"expression": "adapter.errorReason",
						},
						"clusterID": map[string]interface{}{
							"value": "{{ .clusterID }}",
						},
					},
				},
			},
			PostActions: []configloader.PostAction{
				{
					ActionBase: configloader.ActionBase{
						Name: "reportError",
						APICall: &configloader.APICall{
							Method:  "PUT",
							URL:     "{{ .hyperfleetApiBaseUrl }}/api/{{ .hyperfleetApiVersion }}/clusters/{{ .clusterID }}/error-report",
							Body:    "{{ .errorReportPayload }}",
							Timeout: "5s",
						},
					},
				},
			},
		},
	}

	apiClient, err := hyperfleetapi.NewClient(testLog(), hyperfleetapi.WithRetryAttempts(1))
	assert.NoError(t, err)
	exec, err := executor.NewBuilder().
		WithConfig(config).
		WithAPIClient(apiClient).
		WithLogger(getK8sEnvForTest(t).Log).
		WithTransportClient(getK8sEnvForTest(t).Client).
		Build()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Execute - should fail due to precondition API error
	evt := createTestEvent("cluster-cel-error-test")
	ctx := context.Background()
	result := exec.Execute(ctx, evt)

	// Verify execution failed (due to precondition failure)
	assert.Equal(t, executor.StatusFailed, result.Status, "Expected failed status")
	require.NotEmpty(t, result.Errors, "Expected error to be set")

	// Verify post action was attempted (to report the error)
	assert.Equal(t, 1, len(result.PostActionResults), "Expected 1 post action result")
	if len(result.PostActionResults) > 0 {
		postResult := result.PostActionResults[0]
		// The post action itself may fail (mock server returns 404), but the API call should have been made
		assert.True(t, postResult.APICallMade, "Expected API call to be made")
		// Note: Post action status may be failed if mock API returns 404 for error-report endpoint
	}

	// Verify the error report payload was built correctly with CEL expressions accessing executionError
	requests := mockAPI.GetRequests()
	var errorReportRequest *testutil.MockRequest
	for i := range requests {
		if requests[i].Method == http.MethodPut && strings.Contains(requests[i].Path, "/error-report") {
			errorReportRequest = &requests[i]
			break
		}
	}

	assert.NotNil(t, errorReportRequest, "Expected error report API call to be made")
	if errorReportRequest != nil {
		t.Logf("Error report body: %s", errorReportRequest.Body)

		// Parse the request body
		var reportPayload map[string]interface{}
		err := json.Unmarshal([]byte(errorReportRequest.Body), &reportPayload)
		assert.NoError(t, err, "Should be able to parse error report payload")

		if err == nil {
			// Verify CEL expressions successfully accessed adapter.executionError
			assert.Equal(t, true, reportPayload["hasError"], "hasError should be true")
			assert.Equal(t, "preconditions", reportPayload["errorPhase"], "errorPhase should be 'preconditions'")
			assert.Equal(t, "clusterStatus", reportPayload["errorStep"], "errorStep should be 'clusterStatus'")
			assert.NotEqual(t, "no_message", reportPayload["errorMessage"], "errorMessage should contain actual error")

			// Verify other adapter fields still accessible
			assert.Equal(t, "failed", reportPayload["executionStatus"], "executionStatus should be 'failed'")
			assert.NotEmpty(t, reportPayload["errorReason"], "errorReason should be populated")
			assert.Equal(t, "cluster-cel-error-test", reportPayload["clusterID"], "clusterID should match")

			t.Logf("CEL expressions successfully accessed executionError:")
			t.Logf("  hasError: %v", reportPayload["hasError"])
			t.Logf("  errorPhase: %v", reportPayload["errorPhase"])
			t.Logf("  errorStep: %v", reportPayload["errorStep"])
			t.Logf("  errorMessage: %v", reportPayload["errorMessage"])
			t.Logf("  executionStatus: %v", reportPayload["executionStatus"])
			t.Logf("  errorReason: %v", reportPayload["errorReason"])
		}
	}

	t.Logf("ExecutionError CEL access test completed successfully")
}

// TestExecutor_PayloadBuildFailure tests that payload build failures are logged as errors and block post actions
func TestExecutor_PayloadBuildFailure(t *testing.T) {
	// Setup mock API server
	mockAPI := testutil.NewMockAPIServer(t)
	defer mockAPI.Close()

	// Set environment variables
	t.Setenv("HYPERFLEET_API_BASE_URL", mockAPI.URL())
	t.Setenv("HYPERFLEET_API_VERSION", "v1")

	// Create config with invalid CEL expression in payload build (will cause build failure)
	config := &configloader.Config{
		Adapter: configloader.AdapterInfo{
			Name:    "payload-build-fail-test",
			Version: "1.0.0",
		},
		Clients: configloader.ClientsConfig{
			HyperfleetAPI: configloader.HyperfleetAPIConfig{
				Timeout: 10 * time.Second, RetryAttempts: 1,
			},
		},
		Params: []configloader.Parameter{
			{Name: "hyperfleetApiBaseUrl", Source: "env.HYPERFLEET_API_BASE_URL", Required: true},
			{Name: "hyperfleetApiVersion", Default: "v1"},
			{Name: "clusterID", Source: "event.id", Required: true},
		},
		Preconditions: []configloader.Precondition{
			{
				ActionBase: configloader.ActionBase{Name: "simpleCheck"},
				Conditions: []configloader.Condition{
					{Field: "clusterID", Operator: "equals", Value: "test-cluster"},
				},
			},
		},
		Resources: []configloader.Resource{},
		Post: &configloader.PostConfig{
			Payloads: []configloader.Payload{
				{
					Name: "badPayload",
					Build: map[string]interface{}{
						// Use template that references non-existent parameter
						"field": map[string]interface{}{
							"value": "{{ .nonExistentParam }}",
						},
					},
				},
			},
			PostActions: []configloader.PostAction{
				{
					ActionBase: configloader.ActionBase{
						Name: "shouldNotExecute",
						APICall: &configloader.APICall{
							Method:  "PUT",
							URL:     "{{ .hyperfleetApiBaseUrl }}/api/{{ .hyperfleetApiVersion }}/clusters/{{ .clusterID }}/statuses",
							Body:    "{{ .badPayload }}",
							Timeout: "5s",
						},
					},
				},
			},
		},
	}

	apiClient, err := hyperfleetapi.NewClient(testLog())
	assert.NoError(t, err)
	// Use capture logger to verify error logging
	log, logCapture := logger.NewCaptureLogger()

	exec, err := executor.NewBuilder().
		WithConfig(config).
		WithAPIClient(apiClient).
		WithLogger(log).
		WithTransportClient(getK8sEnvForTest(t).Client).
		Build()
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Execute
	evt := createTestEvent("test-cluster")
	ctx := context.Background()
	result := exec.Execute(ctx, evt)

	// Verify execution failed in post_actions phase (payload build)
	assert.Equal(t, executor.StatusFailed, result.Status, "Expected failed status")
	assert.Equal(t, executor.PhasePostActions, result.CurrentPhase, "Expected failure in post_actions phase")
	require.NotEmpty(t, result.Errors, "Expected error to be set")

	// Verify preconditions passed
	assert.Equal(t, 1, len(result.PreconditionResults), "Expected 1 precondition result")
	if len(result.PreconditionResults) > 0 {
		assert.True(t, result.PreconditionResults[0].Matched, "Expected precondition to pass")
	}

	// Verify NO post actions were executed (blocked by payload build failure)
	assert.Equal(t, 0, len(result.PostActionResults), "Expected 0 post action results (blocked by payload build failure)")

	// Verify ExecutionError was set
	assert.NotNil(t, result.ExecutionContext, "Expected execution context")
	if result.ExecutionContext != nil {
		assert.NotNil(t, result.ExecutionContext.Adapter.ExecutionError, "Expected ExecutionError to be set")
		if result.ExecutionContext.Adapter.ExecutionError != nil {
			assert.Equal(t, "post_actions", result.ExecutionContext.Adapter.ExecutionError.Phase)
			assert.Equal(t, "build_payloads", result.ExecutionContext.Adapter.ExecutionError.Step)
			t.Logf("ExecutionError: %+v", result.ExecutionContext.Adapter.ExecutionError)
		}
	}

	// Verify error was logged (should contain "failed to build")
	// slog uses "level=ERROR" format
	capturedLogs := logCapture.Messages()
	t.Logf("Captured logs:\n%s", capturedLogs)
	foundErrorLog := logCapture.Contains("level=ERROR") && logCapture.Contains("failed to build")
	assert.True(t, foundErrorLog, "Expected to find error log for payload build failure")

	// Verify NO API call was made to the post action endpoint (blocked)
	requests := mockAPI.GetRequests()
	for _, req := range requests {
		if req.Method == http.MethodPut && strings.Contains(req.Path, "/statuses") {
			t.Errorf("Post action API call should NOT have been made (blocked by payload build failure)")
		}
	}

	t.Logf("Payload build failure test completed: post actions properly blocked, error logged")
}
