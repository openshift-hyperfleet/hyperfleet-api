package criteria

import (
	"context"
	"testing"
	"time"

	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCELEvaluator(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("status", "Ready")
	ctx.Set("replicas", 3)

	evaluator, err := newCELEvaluator(ctx)
	require.NoError(t, err)
	require.NotNil(t, evaluator)
}

func TestCELEvaluatorEvaluate(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("status", "Ready")
	ctx.Set("replicas", 3)
	ctx.Set("provider", "aws")
	ctx.Set("enabled", true)

	evaluator, err := newCELEvaluator(ctx)
	require.NoError(t, err)

	tests := []struct {
		wantValue  interface{}
		name       string
		expression string
		wantMatch  bool
		wantErr    bool
	}{
		{
			name:       "string equality true",
			expression: `status == "Ready"`,
			wantMatch:  true,
			wantValue:  true,
		},
		{
			name:       "string equality false",
			expression: `status == "Failed"`,
			wantMatch:  false,
			wantValue:  false,
		},
		{
			name:       "numeric comparison greater",
			expression: `replicas > 2`,
			wantMatch:  true,
			wantValue:  true,
		},
		{
			name:       "numeric comparison less",
			expression: `replicas < 5`,
			wantMatch:  true,
			wantValue:  true,
		},
		{
			name:       "boolean variable",
			expression: `enabled`,
			wantMatch:  true,
			wantValue:  true,
		},
		{
			name:       "compound and",
			expression: `status == "Ready" && replicas > 0`,
			wantMatch:  true,
			wantValue:  true,
		},
		{
			name:       "compound or",
			expression: `status == "Failed" || replicas > 0`,
			wantMatch:  true,
			wantValue:  true,
		},
		{
			name:       "string in list",
			expression: `provider in ["aws", "gcp", "azure"]`,
			wantMatch:  true,
			wantValue:  true,
		},
		{
			name:       "empty expression",
			expression: ``,
			wantMatch:  true,
			wantValue:  true,
		},
		{
			name:       "invalid syntax",
			expression: `status ===== "Ready"`,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.EvaluateSafe(tt.expression)
			if tt.wantErr {
				// Parse errors are returned as error, eval errors in result
				if err != nil {
					assert.Error(t, err)
					return
				}
				// Evaluation error captured in result
				assert.True(t, result.HasError())
				return
			}
			require.NoError(t, err)
			assert.False(t, result.HasError())
			assert.Equal(t, tt.wantMatch, result.Matched)
			assert.Equal(t, tt.wantValue, result.Value)
			assert.Equal(t, tt.expression, result.Expression)
		})
	}
}

func TestCELEvaluatorWithNestedData(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("cluster", map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Reconciled", "status": "True"},
			},
		},
		"spec": map[string]interface{}{
			"replicas": 3,
		},
	})

	evaluator, err := newCELEvaluator(ctx)
	require.NoError(t, err)

	// Test nested field access
	result, err := evaluator.EvaluateSafe(
		`cluster.status.conditions.exists(c, c.type == "Reconciled" && c.status == "True")`)
	require.NoError(t, err)
	assert.False(t, result.HasError())
	assert.True(t, result.Matched)

	// Test nested numeric comparison
	result, err = evaluator.EvaluateSafe(`cluster.spec.replicas > 1`)
	require.NoError(t, err)
	assert.False(t, result.HasError())
	assert.True(t, result.Matched)
}

func TestCELEvaluatorEvaluateSafe(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("cluster", map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Reconciled", "status": "True"},
			},
		},
	})
	ctx.Set("nullValue", nil)

	evaluator, err := newCELEvaluator(ctx)
	require.NoError(t, err)

	t.Run("successful evaluation", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(
			`cluster.status.conditions.exists(c, c.type == "Reconciled" && c.status == "True")`)
		require.NoError(t, err, "EvaluateSafe should not return error for valid expression")
		assert.False(t, result.HasError())
		assert.True(t, result.Matched)
		assert.Nil(t, result.Error)
	})

	t.Run("missing field returns error in result (safe)", func(t *testing.T) {
		// Evaluation errors (missing fields) are captured in result, NOT returned as error
		result, err := evaluator.EvaluateSafe(`cluster.nonexistent.field == "test"`)
		require.NoError(t, err, "EvaluateSafe should not return error for evaluation errors")
		assert.True(t, result.HasError())
		assert.False(t, result.Matched)
		assert.NotNil(t, result.Error)
		assert.Contains(t, result.Error.Error(), "no such key")
	})

	t.Run("access field on null returns error in result (safe)", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`nullValue.field == "test"`)
		require.NoError(t, err, "EvaluateSafe should not return error for null access")
		assert.True(t, result.HasError())
		assert.False(t, result.Matched)
		assert.NotNil(t, result.Error)
	})

	t.Run("has() on missing intermediate key returns error in result", func(t *testing.T) {
		// Without preprocessing, has(cluster.nonexistent.field) errors
		// because cluster.nonexistent doesn't exist
		result, err := evaluator.EvaluateSafe(`has(cluster.nonexistent.field)`)
		require.NoError(t, err)
		assert.True(t, result.HasError())
		assert.False(t, result.Matched)
		assert.Contains(t, result.Error.Error(), "no such key")
	})

	t.Run("has() on existing intermediate key returns false for missing leaf", func(t *testing.T) {
		// has(cluster.status.missing) - cluster.status exists, but missing doesn't
		result, err := evaluator.EvaluateSafe(`has(cluster.status.missing)`)
		require.NoError(t, err)
		assert.True(t, !result.HasError())
		assert.False(t, result.Matched) // false because field doesn't exist
		assert.Nil(t, result.Error)
	})

	t.Run("empty expression returns true", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe("")
		require.NoError(t, err)
		assert.True(t, !result.HasError())
		assert.True(t, result.Matched)
	})

	t.Run("error result can be used for conditional logic", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`cluster.missing.path == "value"`)
		require.NoError(t, err, "Evaluation errors should be captured, not returned")

		// You can use the result for conditional logic
		var finalValue interface{}
		var reason string

		if result.HasError() {
			finalValue = nil
			reason = result.Error.Error()
		} else {
			finalValue = result.Value
			reason = ""
		}

		assert.Nil(t, finalValue)
		assert.NotEmpty(t, reason)
	})

	t.Run("parse error returns actual error (not safe)", func(t *testing.T) {
		// Parse errors should be returned as actual errors - they indicate bugs
		result, err := evaluator.EvaluateSafe(`invalid syntax ===`)
		assert.Error(t, err, "Parse errors should be returned as errors")
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "parse error")
	})
}

func TestCELEvaluatorEvaluateBool(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("status", "Ready")

	evaluator, err := newCELEvaluator(ctx)
	require.NoError(t, err)

	// True result
	matched, err := evaluator.EvaluateBool(`status == "Ready"`)
	require.NoError(t, err)
	assert.True(t, matched)

	// False result
	matched, err = evaluator.EvaluateBool(`status == "Failed"`)
	require.NoError(t, err)
	assert.False(t, matched)
}

func TestCELEvaluatorEvaluateString(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("status", "Ready")
	ctx.Set("name", "test-cluster")

	evaluator, err := newCELEvaluator(ctx)
	require.NoError(t, err)

	// String result
	result, err := evaluator.EvaluateString(`name`)
	require.NoError(t, err)
	assert.Equal(t, "test-cluster", result)

	// String concatenation
	result, err = evaluator.EvaluateString(`name + "-suffix"`)
	require.NoError(t, err)
	assert.Equal(t, "test-cluster-suffix", result)
}

func TestEvaluatorCELIntegration(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("status", "Ready")
	ctx.Set("replicas", 3)
	ctx.Set("provider", "aws")

	evaluator, err := NewEvaluator(context.Background(), ctx, logger.NewTestLogger())
	require.NoError(t, err)
	// Test EvaluateCEL
	result, err := evaluator.EvaluateCEL(`status == "Ready" && replicas > 1`)
	require.NoError(t, err)
	assert.True(t, result.Matched)
}

func TestCELEvaluatorCustomFunctions(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("resources", map[string]interface{}{
		"managedCluster": map[string]interface{}{
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{"type": "Reconciled", "status": "True"},
				},
			},
		},
		"manifestWork": map[string]interface{}{
			"clusterClaim": map[string]interface{}{
				"status": map[string]interface{}{
					"value": "prod",
				},
			},
		},
	})

	evaluator, err := newCELEvaluator(ctx)
	require.NoError(t, err)

	t.Run("toJson serializes structures", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`toJson(resources)`)
		require.NoError(t, err)
		require.False(t, result.HasError())

		jsonText, ok := result.Value.(string)
		require.True(t, ok)
		assert.Contains(t, jsonText, `"managedCluster"`)
		assert.Contains(t, jsonText, `"manifestWork"`)
	})

	t.Run("dig safely reads nested fields", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`dig(resources, "managedCluster.status.conditions")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.NotNil(t, result.Value)
		assert.Equal(t, []interface{}{map[string]interface{}{"type": "Reconciled", "status": "True"}}, result.Value)
	})

	t.Run("dig returns null for missing path", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`dig(resources, "managedCluster.status.missing") == null`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, true, result.Value)
		assert.True(t, result.Matched)
	})

	t.Run("now returns RFC3339 timestamp", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`now()`)
		require.NoError(t, err)
		require.False(t, result.HasError())

		timestamp, ok := result.Value.(string)
		require.True(t, ok, "now() should return a string")
		assert.NotEmpty(t, timestamp)

		// Verify it's a valid RFC3339 timestamp by parsing it
		_, parseErr := time.Parse(time.RFC3339, timestamp)
		assert.NoError(t, parseErr, "now() should return a valid RFC3339 timestamp")
	})

	t.Run("now can be used with timestamp() for time calculations", func(t *testing.T) {
		// Test that now() can be converted to timestamp type and used in calculations
		result, err := evaluator.EvaluateSafe(`timestamp(now()).getFullYear() >= 2024`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, true, result.Value)
		assert.True(t, result.Matched)
	})
}

func TestCELEvaluatorDomainFunctions(t *testing.T) {
	recentTime := time.Now().Add(-30 * time.Second).Format(time.RFC3339)
	oldTime := time.Now().Add(-10 * time.Minute).Format(time.RFC3339)

	ctx := NewEvaluationContext()
	ctx.Set("conditions", []interface{}{
		map[string]interface{}{
			"type":                 "Reconciled",
			"status":               "True",
			"last_transition_time": recentTime,
		},
		map[string]interface{}{
			"type":                 "Available",
			"status":               "False",
			"last_transition_time": oldTime,
		},
	})
	ctx.Set("emptyConditions", []interface{}{})
	ctx.Set("statusFeedback", map[string]interface{}{
		"values": []interface{}{
			map[string]interface{}{
				"name":       "phase",
				"fieldValue": map[string]interface{}{"string": "Active"},
			},
			map[string]interface{}{
				"name":       "replicas",
				"fieldValue": map[string]interface{}{"string": "3"},
			},
		},
	})
	ctx.Set("emptyFeedback", map[string]interface{}{})

	evaluator, err := newCELEvaluator(ctx)
	require.NoError(t, err)

	t.Run("conditionStatus returns status for existing condition", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`conditionStatus(conditions, "Reconciled")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, "True", result.Value)
	})

	t.Run("conditionStatus returns Unknown for missing condition", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`conditionStatus(conditions, "MissingType")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, "Unknown", result.Value)
	})

	t.Run("conditionStatus returns Unknown for empty list", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`conditionStatus(emptyConditions, "Reconciled")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, "Unknown", result.Value)
	})

	t.Run("conditionStatus returns False for Available condition", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`conditionStatus(conditions, "Available")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, "False", result.Value)
	})

	t.Run("conditionAge returns positive seconds for existing condition", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`conditionAge(conditions, "Reconciled")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		age, ok := result.Value.(int64)
		require.True(t, ok)
		assert.True(t, age >= 29 && age <= 60, "expected age ~30s, got %d", age)
	})

	t.Run("conditionAge returns -1 for missing condition", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`conditionAge(conditions, "MissingType")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, int64(-1), result.Value)
	})

	t.Run("conditionAge returns -1 for empty list", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`conditionAge(emptyConditions, "Reconciled")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, int64(-1), result.Value)
	})

	t.Run("stableFor returns true when condition is True and age exceeds threshold", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`stableFor(conditions, "Reconciled", 10)`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, true, result.Value)
	})

	t.Run("stableFor returns false when age below threshold", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`stableFor(conditions, "Reconciled", 9999)`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, false, result.Value)
	})

	t.Run("stableFor returns false when condition is False", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`stableFor(conditions, "Available", 1)`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, false, result.Value)
	})

	t.Run("stableFor returns false for missing condition", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`stableFor(conditions, "MissingType", 1)`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, false, result.Value)
	})

	t.Run("statusFeedbackValue returns value for existing name", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`statusFeedbackValue(statusFeedback, "phase")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, "Active", result.Value)
	})

	t.Run("statusFeedbackValue returns empty for missing name", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`statusFeedbackValue(statusFeedback, "missing")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, "", result.Value)
	})

	t.Run("statusFeedbackValue returns empty for empty feedback", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`statusFeedbackValue(emptyFeedback, "phase")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, "", result.Value)
	})

	t.Run("triState returns True when first arg true", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`triState(true, false)`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, "True", result.Value)
	})

	t.Run("triState returns False when second arg true", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`triState(false, true)`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, "False", result.Value)
	})

	t.Run("triState returns Unknown when both false", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`triState(false, false)`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, "Unknown", result.Value)
	})

	t.Run("triState returns True when both true (trueCond takes priority)", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`triState(true, true)`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, "True", result.Value)
	})

	t.Run("conditionStatus with CEL expressions as arguments", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(
			`conditionStatus(conditions, "Reconciled") == "True"`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, true, result.Value)
	})

	t.Run("triState with CEL expressions replacing nested ternaries", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(
			`triState(conditionStatus(conditions, "Reconciled") == "True", ` +
				`conditionStatus(conditions, "Reconciled") == "False")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, "True", result.Value)
	})
}

func TestCELEvaluatorDomainFunctions_MalformedInputs(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("badTimeConditions", []interface{}{
		map[string]interface{}{
			"type":                 "Ready",
			"status":               "True",
			"last_transition_time": "not-a-timestamp",
		},
	})
	ctx.Set("noTimeConditions", []interface{}{
		map[string]interface{}{
			"type":   "Ready",
			"status": "True",
		},
	})
	ctx.Set("malformedFeedback", map[string]interface{}{
		"values": "not-a-list",
	})
	ctx.Set("feedbackBadEntry", map[string]interface{}{
		"values": []interface{}{"not-a-map"},
	})
	ctx.Set("feedbackNoFieldValue", map[string]interface{}{
		"values": []interface{}{
			map[string]interface{}{"name": "phase"},
		},
	})

	evaluator, err := newCELEvaluator(ctx)
	require.NoError(t, err)

	t.Run("conditionAge returns -1 for invalid timestamp", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`conditionAge(badTimeConditions, "Ready")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, int64(-1), result.Value)
	})

	t.Run("conditionAge returns -1 when last_transition_time is missing", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`conditionAge(noTimeConditions, "Ready")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, int64(-1), result.Value)
	})

	t.Run("stableFor returns false for invalid timestamp", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`stableFor(badTimeConditions, "Ready", 1)`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, false, result.Value)
	})

	t.Run("stableFor returns false when last_transition_time is missing", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`stableFor(noTimeConditions, "Ready", 1)`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, false, result.Value)
	})

	t.Run("statusFeedbackValue returns empty for non-list values", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`statusFeedbackValue(malformedFeedback, "phase")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, "", result.Value)
	})

	t.Run("statusFeedbackValue returns empty for non-map entry", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`statusFeedbackValue(feedbackBadEntry, "phase")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, "", result.Value)
	})

	t.Run("statusFeedbackValue returns empty when fieldValue is missing", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`statusFeedbackValue(feedbackNoFieldValue, "phase")`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, "", result.Value)
	})
}

func TestCELEvaluatorExtStrings(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("channelGroup", "candidate")
	ctx.Set("version", "4.22.0-ec.4")

	evaluator, err := newCELEvaluator(ctx)
	require.NoError(t, err)

	// The version resolution adapter derives the Cincinnati channel name from channelGroup
	// and the major.minor portion of version using CEL split(). This mirrors the Go logic
	// in deriveChannel(): channelGroup + "-" + major + "." + minor
	// e.g. channelGroup="candidate", version="4.22.0-ec.4" → "candidate-4.22"
	t.Run("split derives cincinnati channel name from channelGroup and version", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`channelGroup + "-" + version.split(".")[0] + "." + version.split(".")[1]`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		assert.Equal(t, "candidate-4.22", result.Value)
	})
}

func TestCELEvaluatorExtLists(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("tags", []interface{}{"production", "tier-1", "us-east", "tier-1"})
	ctx.Set("nodePools", []interface{}{
		map[string]interface{}{"name": "worker-pool", "replicas": int64(3)},
		map[string]interface{}{"name": "infra-pool", "replicas": int64(2)},
	})
	ctx.Set("nested", []interface{}{
		[]interface{}{"a", "b"},
		[]interface{}{"c", "d"},
	})

	evaluator, err := newCELEvaluator(ctx)
	require.NoError(t, err)

	t.Run("distinct removes duplicates", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`tags.distinct()`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		vals, ok := result.Value.([]ref.Val)
		require.True(t, ok)
		assert.Len(t, vals, 3)
	})

	t.Run("sort orders strings alphabetically", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`tags.distinct().sort()`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		vals, ok := result.Value.([]ref.Val)
		require.True(t, ok)
		assert.Equal(t, "production", string(vals[0].(types.String)))
		assert.Equal(t, "tier-1", string(vals[1].(types.String)))
		assert.Equal(t, "us-east", string(vals[2].(types.String)))
	})

	t.Run("slice returns sub-list", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`tags.slice(0, 2)`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		vals, ok := result.Value.([]ref.Val)
		require.True(t, ok)
		assert.Len(t, vals, 2)
	})

	t.Run("flatten collapses nested lists", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`nested.flatten()`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		vals, ok := result.Value.([]ref.Val)
		require.True(t, ok)
		assert.Len(t, vals, 4)
	})

	t.Run("sortBy orders objects by field", func(t *testing.T) {
		result, err := evaluator.EvaluateSafe(`nodePools.sortBy(p, p.name).map(p, p.name)`)
		require.NoError(t, err)
		require.False(t, result.HasError())
		vals, ok := result.Value.([]ref.Val)
		require.True(t, ok)
		require.Len(t, vals, 2)
		assert.Equal(t, "infra-pool", string(vals[0].(types.String)))
		assert.Equal(t, "worker-pool", string(vals[1].(types.String)))
	})
}

// TestEvaluateSafeErrorHandling tests how EvaluateSafe handles various error scenarios
// and how callers can use the result to make decisions at a higher level
func TestEvaluateSafeErrorHandling(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("data", map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"value": "found",
			},
		},
	})

	evaluator, err := newCELEvaluator(ctx)
	require.NoError(t, err)

	tests := []struct {
		name        string
		expression  string
		wantReason  string // substring to match in Error
		wantSuccess bool
		wantMatched bool
	}{
		{
			name:        "existing nested field",
			expression:  `data.level1.level2.value == "found"`,
			wantSuccess: true,
			wantMatched: true,
		},
		{
			name:        "missing leaf field",
			expression:  `data.level1.level2.missing == "test"`,
			wantSuccess: false,
			wantReason:  "no such key",
		},
		{
			name:        "missing intermediate field",
			expression:  `data.level1.nonexistent.value == "test"`,
			wantSuccess: false,
			wantReason:  "no such key",
		},
		{
			name:        "has() on existing path",
			expression:  `has(data.level1.level2.value)`,
			wantSuccess: true,
			wantMatched: true,
		},
		{
			name:        "has() on missing leaf",
			expression:  `has(data.level1.level2.missing)`,
			wantSuccess: true,
			wantMatched: false,
		},
		{
			name:        "has() on missing intermediate",
			expression:  `has(data.level1.nonexistent.value)`,
			wantSuccess: false,
			wantReason:  "no such key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.EvaluateSafe(tt.expression)
			require.NoError(t, err, "EvaluateSafe should not return parse/program errors for valid expressions")

			if tt.wantSuccess {
				assert.True(t, !result.HasError(), "expected success but got error: %v", result.Error)
				assert.Equal(t, tt.wantMatched, result.Matched)
			} else {
				assert.True(t, result.HasError(), "expected error but got success")
				assert.Contains(t, result.Error.Error(), tt.wantReason)
			}
		})
	}
}
