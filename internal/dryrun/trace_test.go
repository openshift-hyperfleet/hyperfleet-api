package dryrun

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/executor"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTestTrace(status executor.ExecutionStatus, verbose bool) *ExecutionTrace {
	apiClient, _ := NewDryrunAPIClient(nil)
	transport := NewDryrunTransportClient()
	return &ExecutionTrace{
		EventID:   "test-event-id",
		EventType: "test.event.type",
		Result: &executor.ExecutionResult{
			Status: status,
			Params: map[string]interface{}{"key": "value"},
			Errors: make(map[executor.ExecutionPhase]error),
		},
		APIClient: apiClient,
		Transport: transport,
		Verbose:   verbose,
	}
}

func TestFormatText_Success(t *testing.T) {
	t.Run("successful execution trace contains all phases and SUCCESS result", func(t *testing.T) {
		trace := makeTestTrace(executor.StatusSuccess, false)
		trace.Result.PreconditionResults = []executor.PreconditionResult{
			{
				Name:    "check-exists",
				Status:  executor.StatusSuccess,
				Matched: true,
			},
		}
		trace.Result.ResourceResults = []executor.ResourceResult{
			{
				Name:         "my-resource",
				Kind:         "ConfigMap",
				Namespace:    "default",
				ResourceName: "my-configmap",
				Status:       executor.StatusSuccess,
				Operation:    manifest.OperationCreate,
			},
		}

		output := trace.FormatText()

		assert.Contains(t, output, "Dry-Run Execution Trace")
		assert.Contains(t, output, "Phase 1: Parameter Extraction")
		assert.Contains(t, output, "Phase 2: Preconditions")
		assert.Contains(t, output, "Phase 3: Resources")
		assert.Contains(t, output, "Phase 4: Post Actions")
		assert.Contains(t, output, "Result: SUCCESS")
	})
}

func TestFormatText_Failed(t *testing.T) {
	t.Run("failed execution trace shows FAILED result and error in resource result", func(t *testing.T) {
		trace := makeTestTrace(executor.StatusFailed, false)
		trace.Result.Errors[executor.PhaseResources] = fmt.Errorf("resource apply failed: connection refused")
		trace.Result.ResourceResults = []executor.ResourceResult{
			{
				Name:         "failing-resource",
				Kind:         "Deployment",
				Namespace:    "default",
				ResourceName: "my-deploy",
				Status:       executor.StatusFailed,
				Operation:    manifest.OperationCreate,
				Error:        fmt.Errorf("resource apply failed: connection refused"),
			},
		}

		output := trace.FormatText()

		assert.Contains(t, output, "Result: FAILED")
		assert.Contains(t, output, "resource apply failed: connection refused")
	})
}

func TestFormatText_ResourcesSkipped(t *testing.T) {
	t.Run("skipped resources phase shows SKIPPED", func(t *testing.T) {
		trace := makeTestTrace(executor.StatusSuccess, false)
		trace.Result.ResourcesSkipped = true
		trace.Result.SkipReason = "preconditions not met"

		output := trace.FormatText()

		assert.Contains(t, output, "Phase 3: Resources")
		assert.Contains(t, output, "SKIPPED")
		assert.Contains(t, output, "preconditions not met")
	})

	t.Run("resource not found shows RESOURCE NOT FOUND instead of NOT MET", func(t *testing.T) {
		trace := makeTestTrace(executor.StatusSuccess, false)
		trace.Result.ResourcesSkipped = true
		trace.Result.SkipReason = executor.ResourceNotFoundReason

		output := trace.FormatText()

		assert.Contains(t, output, "(RESOURCE NOT FOUND)")
		assert.NotContains(t, output, "(NOT MET)")
	})

	t.Run("post-action resource not found shows RESOURCE NOT FOUND in phase 4", func(t *testing.T) {
		trace := makeTestTrace(executor.StatusSuccess, false)
		trace.Result.ResourcesSkipped = true
		trace.Result.SkipReason = executor.ResourceNotFoundReason
		trace.Result.PostActionResults = []executor.PostActionResult{
			{Name: "reportStatus", Status: executor.StatusSkipped, Skipped: true, SkipReason: executor.ResourceNotFoundReason},
		}

		output := trace.FormatText()

		assert.Contains(t, output, "Phase 4: Post Actions")
		assert.Contains(t, output, "(RESOURCE NOT FOUND)")
		assert.Contains(t, output, "SKIPPED")
	})
}

func TestFormatText_VerboseShowsBodies(t *testing.T) {
	t.Run("verbose mode includes request and response bodies", func(t *testing.T) {
		trace := makeTestTrace(executor.StatusSuccess, true)

		trace.APIClient.Requests = append(trace.APIClient.Requests, RequestRecord{
			Method:     "GET",
			URL:        "http://mock-api/test",
			StatusCode: 200,
			Body:       []byte(`{"request":"data"}`),
			Response:   []byte(`{"response":"data"}`),
		})
		trace.Result.PreconditionResults = []executor.PreconditionResult{
			{
				Name:        "api-check",
				Status:      executor.StatusSuccess,
				Matched:     true,
				APICallMade: true,
			},
		}

		output := trace.FormatText()

		assert.Contains(t, output, "[verbose]")
	})
}

func TestFormatText_NonVerboseOmitsBodies(t *testing.T) {
	t.Run("non-verbose mode omits request and response bodies", func(t *testing.T) {
		trace := makeTestTrace(executor.StatusSuccess, false)

		trace.APIClient.Requests = append(trace.APIClient.Requests, RequestRecord{
			Method:     "GET",
			URL:        "http://mock-api/test",
			StatusCode: 200,
			Body:       []byte(`{"request":"data"}`),
			Response:   []byte(`{"response":"data"}`),
		})
		trace.Result.PreconditionResults = []executor.PreconditionResult{
			{
				Name:        "api-check",
				Status:      executor.StatusSuccess,
				Matched:     true,
				APICallMade: true,
			},
		}

		output := trace.FormatText()

		assert.NotContains(t, output, "[verbose]")
		assert.NotContains(t, output, `{"request":"data"}`)
		assert.NotContains(t, output, `{"response":"data"}`)
	})
}

func TestFormatJSON_Structure(t *testing.T) {
	t.Run("JSON output has correct event and status fields", func(t *testing.T) {
		trace := makeTestTrace(executor.StatusSuccess, false)
		trace.Result.PreconditionResults = []executor.PreconditionResult{
			{
				Name:    "check-exists",
				Status:  executor.StatusSuccess,
				Matched: true,
			},
		}

		data, err := trace.FormatJSON()
		require.NoError(t, err)

		var result TraceJSON
		err = json.Unmarshal(data, &result)
		require.NoError(t, err)

		assert.Equal(t, "test-event-id", result.Event.ID)
		assert.Equal(t, "test.event.type", result.Event.Type)
		assert.Equal(t, string(executor.StatusSuccess), result.Status)
		assert.Len(t, result.Preconditions, 1)
		assert.Equal(t, "check-exists", result.Preconditions[0].Name)
	})
}

func TestFormatJSON_VerboseIncludesBodies(t *testing.T) {
	t.Run("verbose JSON includes request and response bodies", func(t *testing.T) {
		trace := makeTestTrace(executor.StatusSuccess, true)

		trace.APIClient.Requests = append(trace.APIClient.Requests, RequestRecord{
			Method:     "POST",
			URL:        "http://mock-api/resource",
			StatusCode: 201,
			Body:       []byte(`{"name":"test"}`),
			Response:   []byte(`{"id":"123"}`),
		})

		data, err := trace.FormatJSON()
		require.NoError(t, err)

		var result TraceJSON
		err = json.Unmarshal(data, &result)
		require.NoError(t, err)

		require.Len(t, result.APIRequests, 1)
		assert.NotEmpty(t, result.APIRequests[0].Request)
		assert.NotEmpty(t, result.APIRequests[0].Response)
	})
}

func TestFormatJSON_NonVerboseOmitsBodies(t *testing.T) {
	t.Run("non-verbose JSON omits request and response bodies", func(t *testing.T) {
		trace := makeTestTrace(executor.StatusSuccess, false)

		trace.APIClient.Requests = append(trace.APIClient.Requests, RequestRecord{
			Method:     "POST",
			URL:        "http://mock-api/resource",
			StatusCode: 201,
			Body:       []byte(`{"name":"test"}`),
			Response:   []byte(`{"id":"123"}`),
		})

		data, err := trace.FormatJSON()
		require.NoError(t, err)

		var result TraceJSON
		err = json.Unmarshal(data, &result)
		require.NoError(t, err)

		require.Len(t, result.APIRequests, 1)
		assert.Empty(t, result.APIRequests[0].Request)
		assert.Empty(t, result.APIRequests[0].Response)
	})
}

func TestPrettyJSON(t *testing.T) {
	t.Run("valid JSON is indented", func(t *testing.T) {
		input := []byte(`{"key":"value","nested":{"a":1}}`)
		result := prettyJSON(input)

		assert.Contains(t, result, "\n")
		assert.Contains(t, result, "  ")

		// Verify it is valid JSON
		var parsed interface{}
		err := json.Unmarshal([]byte(result), &parsed)
		assert.NoError(t, err)
	})

	t.Run("invalid JSON is returned as-is", func(t *testing.T) {
		input := []byte(`not valid json {{{`)
		result := prettyJSON(input)

		assert.Equal(t, string(input), result)
	})
}

func TestFormatValue(t *testing.T) {
	t.Run("string value is quoted", func(t *testing.T) {
		result := formatValue("hello")

		assert.True(t, strings.HasPrefix(result, `"`))
		assert.True(t, strings.HasSuffix(result, `"`))
		assert.Contains(t, result, "hello")
	})

	t.Run("non-string value uses JSON representation", func(t *testing.T) {
		input := map[string]interface{}{"a": 1, "b": "two"}
		result := formatValue(input)

		// Should be valid JSON
		var parsed map[string]interface{}
		err := json.Unmarshal([]byte(result), &parsed)
		assert.NoError(t, err)
		assert.Equal(t, float64(1), parsed["a"])
		assert.Equal(t, "two", parsed["b"])
	})

	t.Run("integer value uses JSON representation", func(t *testing.T) {
		result := formatValue(42)
		assert.Equal(t, "42", result)
	})
}
