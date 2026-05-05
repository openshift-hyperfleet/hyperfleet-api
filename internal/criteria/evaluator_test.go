package criteria

import (
	"context"
	"testing"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewEvaluationContext(t *testing.T) {
	ctx := NewEvaluationContext()
	assert.NotNil(t, ctx)
	assert.NotNil(t, ctx.Data())
}

func TestEvaluationContextSetGet(t *testing.T) {
	ctx := NewEvaluationContext()

	ctx.Set("key1", "value1")
	ctx.Set("key2", 42)
	ctx.Set("key3", map[string]string{"nested": "value"})

	val, ok := ctx.Get("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", val)

	val, ok = ctx.Get("key2")
	assert.True(t, ok)
	assert.Equal(t, 42, val)

	val, ok = ctx.Get("nonexistent")
	assert.False(t, ok)
	assert.Nil(t, val)
}

func TestEvaluationContextGetField(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("cluster", map[string]interface{}{
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Reconciled", "status": "True"},
				map[string]interface{}{"type": "Available", "status": "True"},
			},
		},
		"spec": map[string]interface{}{
			"provider": "aws",
		},
	})

	tests := []struct {
		name    string
		want    interface{}
		path    string
		wantNil bool // field not found returns nil value
	}{
		{
			name: "simple nested field",
			path: "cluster.status.conditions",
			want: []interface{}{
				map[string]interface{}{"type": "Reconciled", "status": "True"},
				map[string]interface{}{"type": "Available", "status": "True"},
			},
		},
		{
			name: "deeply nested field",
			path: "cluster.spec.provider",
			want: "aws",
		},
		{
			name:    "nonexistent field",
			path:    "cluster.status.nonexistent",
			wantNil: true,
		},
		{
			name:    "nonexistent top level",
			path:    "nonexistent.field",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ctx.GetField(tt.path)
			assert.NoError(t, err) // parse errors only
			if tt.wantNil {
				assert.Nil(t, result.Value)
			} else {
				assert.Equal(t, tt.want, result.Value)
			}
		})
	}
}

func TestEvaluationContextMerge(t *testing.T) {
	ctx1 := NewEvaluationContext()
	ctx1.Set("key1", "value1")
	ctx1.Set("key2", "value2")

	ctx2 := NewEvaluationContext()
	ctx2.Set("key2", "overwritten")
	ctx2.Set("key3", "value3")

	ctx1.Merge(ctx2)

	val, _ := ctx1.Get("key1")
	assert.Equal(t, "value1", val)

	val, _ = ctx1.Get("key2")
	assert.Equal(t, "overwritten", val)

	val, _ = ctx1.Get("key3")
	assert.Equal(t, "value3", val)
}

// TestEvaluationContextMergeConcurrent verifies that concurrent cross-merges
// don't cause deadlock. Previously, if goroutine A called ctx1.Merge(ctx2) while
// goroutine B called ctx2.Merge(ctx1), a deadlock could occur due to lock ordering.
// The fix snapshots other's data before acquiring the write lock.
func TestEvaluationContextMergeConcurrent(t *testing.T) {
	ctx1 := NewEvaluationContext()
	ctx2 := NewEvaluationContext()

	// Initialize with different data
	ctx1.Set("from1", "value1")
	ctx2.Set("from2", "value2")

	done := make(chan bool, 2)

	// Goroutine A: ctx1.Merge(ctx2)
	go func() {
		for i := 0; i < 100; i++ {
			ctx1.Merge(ctx2)
		}
		done <- true
	}()

	// Goroutine B: ctx2.Merge(ctx1) - would deadlock with old implementation
	go func() {
		for i := 0; i < 100; i++ {
			ctx2.Merge(ctx1)
		}
		done <- true
	}()

	// Wait for both goroutines (with timeout via test timeout)
	<-done
	<-done

	// Both contexts should have both keys
	val1, ok1 := ctx1.Get("from2")
	assert.True(t, ok1, "ctx1 should have from2 after merge")
	assert.Equal(t, "value2", val1)

	val2, ok2 := ctx2.Get("from1")
	assert.True(t, ok2, "ctx2 should have from1 after merge")
	assert.Equal(t, "value1", val2)
}

func TestEvaluateEquals(t *testing.T) {
	tests := []struct {
		field     interface{}
		value     interface{}
		name      string
		wantError bool
		want      bool
	}{
		{
			name:  "equal strings",
			field: "test",
			value: "test",
			want:  true,
		},
		{
			name:  "not equal strings",
			field: "test1",
			value: "test2",
			want:  false,
		},
		{
			name:  "equal numbers",
			field: 42,
			value: 42,
			want:  true,
		},
		{
			name:  "not equal numbers",
			field: 42,
			value: 43,
			want:  false,
		},
		{
			name:  "both nil",
			field: nil,
			value: nil,
			want:  true,
		},
		{
			name:  "field nil",
			field: nil,
			value: "test",
			want:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateEquals(tt.field, tt.value)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestEvaluateIn(t *testing.T) {
	tests := []struct {
		field     interface{}
		list      interface{}
		name      string
		wantError bool
		want      bool
	}{
		{
			name:  "value in string list",
			field: "aws",
			list:  []interface{}{"aws", "gcp", "azure"},
			want:  true,
		},
		{
			name:  "value not in string list",
			field: "unknown",
			list:  []interface{}{"aws", "gcp", "azure"},
			want:  false,
		},
		{
			name:  "number in number list",
			field: 2,
			list:  []interface{}{1, 2, 3},
			want:  true,
		},
		{
			name:      "not a list",
			field:     "test",
			list:      "not a list",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateIn(tt.field, tt.list)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestEvaluateContains(t *testing.T) {
	tests := []struct {
		field     interface{}
		needle    interface{}
		name      string
		wantError bool
		want      bool
	}{
		{
			name:   "string contains substring",
			field:  "hello world",
			needle: "world",
			want:   true,
		},
		{
			name:   "string does not contain substring",
			field:  "hello world",
			needle: "xyz",
			want:   false,
		},
		{
			name:   "array contains value",
			field:  []interface{}{"a", "b", "c"},
			needle: "b",
			want:   true,
		},
		{
			name:   "array does not contain value",
			field:  []interface{}{"a", "b", "c"},
			needle: "d",
			want:   false,
		},
		{
			name:      "invalid field type",
			field:     42,
			needle:    "test",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateContains(tt.field, tt.needle)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestEvaluateGreaterThan(t *testing.T) {
	tests := []struct {
		field     interface{}
		threshold interface{}
		name      string
		wantError bool
		want      bool
	}{
		{
			name:      "int greater than",
			field:     10,
			threshold: 5,
			want:      true,
		},
		{
			name:      "int not greater than",
			field:     5,
			threshold: 10,
			want:      false,
		},
		{
			name:      "float greater than",
			field:     10.5,
			threshold: 10.0,
			want:      true,
		},
		{
			name:      "equal values",
			field:     10,
			threshold: 10,
			want:      false,
		},
		{
			name:      "non-numeric field",
			field:     "test",
			threshold: 10,
			wantError: true,
		},
		{
			name:      "zero field greater than negative",
			field:     0,
			threshold: -1,
			want:      true,
		},
		{
			name:      "zero field not greater than zero",
			field:     0,
			threshold: 0,
			want:      false,
		},
		{
			name:      "zero field not greater than positive",
			field:     0,
			threshold: 1,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateGreaterThan(tt.field, tt.threshold)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestEvaluateLessThan(t *testing.T) {
	tests := []struct {
		field     interface{}
		threshold interface{}
		name      string
		wantError bool
		want      bool
	}{
		{
			name:      "int less than",
			field:     5,
			threshold: 10,
			want:      true,
		},
		{
			name:      "int not less than",
			field:     10,
			threshold: 5,
			want:      false,
		},
		{
			name:      "float less than",
			field:     9.5,
			threshold: 10.0,
			want:      true,
		},
		{
			name:      "equal values",
			field:     10,
			threshold: 10,
			want:      false,
		},
		{
			name:      "non-numeric field",
			field:     "test",
			threshold: 10,
			wantError: true,
		},
		{
			name:      "zero field less than positive",
			field:     0,
			threshold: 1,
			want:      true,
		},
		{
			name:      "zero field not less than zero",
			field:     0,
			threshold: 0,
			want:      false,
		},
		{
			name:      "zero field not less than negative",
			field:     0,
			threshold: -1,
			want:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluateLessThan(tt.field, tt.threshold)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestEvaluateExists(t *testing.T) {
	tests := []struct {
		field interface{}
		name  string
		want  bool
	}{
		{
			name:  "non-nil string",
			field: "test",
			want:  true,
		},
		{
			name:  "empty string",
			field: "",
			want:  false,
		},
		{
			name:  "nil value",
			field: nil,
			want:  false,
		},
		{
			name:  "non-empty slice",
			field: []string{"a", "b"},
			want:  true,
		},
		{
			name:  "empty slice",
			field: []string{},
			want:  false,
		},
		{
			name:  "non-empty map",
			field: map[string]string{"key": "value"},
			want:  true,
		},
		{
			name:  "empty map",
			field: map[string]string{},
			want:  false,
		},
		{
			name:  "number",
			field: 42,
			want:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evaluateExists(tt.field)
			assert.Equal(t, tt.want, result)
		})
	}
}

func TestEvaluatorEvaluateCondition(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("clusterPhase", "Ready")
	ctx.Set("nodeCount", 5)
	ctx.Set("provider", "aws")
	ctx.Set("status", map[string]interface{}{
		"phase": "Active",
	})

	evaluator, err := NewEvaluator(context.Background(), ctx, logger.NewTestLogger())
	require.NoError(t, err)

	tests := []struct {
		name      string
		field     string
		value     interface{}
		operator  Operator
		wantError bool
		want      bool
	}{
		{
			name:     "equals operator",
			field:    "clusterPhase",
			operator: OperatorEquals,
			value:    "Ready",
			want:     true,
		},
		{
			name:     "not equals operator",
			field:    "clusterPhase",
			operator: OperatorNotEquals,
			value:    "Terminating",
			want:     true,
		},
		{
			name:     "in operator",
			field:    "provider",
			operator: OperatorIn,
			value:    []interface{}{"aws", "gcp", "azure"},
			want:     true,
		},
		{
			name:     "greater than operator",
			field:    "nodeCount",
			operator: OperatorGreaterThan,
			value:    3,
			want:     true,
		},
		{
			name:     "less than operator",
			field:    "nodeCount",
			operator: OperatorLessThan,
			value:    10,
			want:     true,
		},
		{
			name:     "exists operator",
			field:    "provider",
			operator: OperatorExists,
			value:    nil,
			want:     true,
		},
		{
			name:     "nested field",
			field:    "status.phase",
			operator: OperatorEquals,
			value:    "Active",
			want:     true,
		},
		{
			name:     "nonexistent field - no error, just not matched",
			field:    "nonexistent",
			operator: OperatorEquals,
			value:    "test",
			want:     false, // nil != "test", so not matched
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.EvaluateCondition(tt.field, tt.operator, tt.value)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result.Matched)
			}
		})
	}
}

func TestEvaluatorEvaluateConditions(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("clusterPhase", "Ready")
	ctx.Set("cloudProvider", "aws")
	ctx.Set("vpcId", "vpc-12345")

	evaluator, err := NewEvaluator(context.Background(), ctx, logger.NewTestLogger())
	require.NoError(t, err)

	tests := []struct {
		name       string
		conditions []ConditionDef
		wantError  bool
		want       bool
	}{
		{
			name: "all conditions pass",
			conditions: []ConditionDef{
				{Field: "clusterPhase", Operator: "in", Value: []interface{}{"Provisioning", "Ready"}},
				{Field: "cloudProvider", Operator: "in", Value: []interface{}{"aws", "gcp", "azure"}},
				{Field: "vpcId", Operator: "exists", Value: nil},
			},
			want: true,
		},
		{
			name: "one condition fails",
			conditions: []ConditionDef{
				{Field: "clusterPhase", Operator: "equals", Value: "Ready"},
				{Field: "cloudProvider", Operator: "equals", Value: "gcp"}, // This fails
			},
			want: false,
		},
		{
			name:       "empty conditions",
			conditions: []ConditionDef{},
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := evaluator.EvaluateConditions(tt.conditions)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result.Matched)
			}
		})
	}
}

func TestExtractField(t *testing.T) {
	data := map[string]interface{}{
		"level1": map[string]interface{}{
			"level2": map[string]interface{}{
				"level3": "value",
			},
			"array": []interface{}{1, 2, 3},
		},
		"simple": "test",
	}

	tests := []struct {
		name      string
		want      interface{}
		path      string
		wantError bool
	}{
		{
			name: "simple field",
			path: "simple",
			want: "test",
		},
		{
			name: "nested field level 2",
			path: "level1.level2",
			want: map[string]interface{}{"level3": "value"},
		},
		{
			name: "nested field level 3",
			path: "level1.level2.level3",
			want: "value",
		},
		{
			name: "array field",
			path: "level1.array",
			want: []interface{}{1, 2, 3},
		},
		{
			name:      "nonexistent field",
			path:      "level1.nonexistent",
			wantError: true,
		},
		{
			name:      "nonexistent top level",
			path:      "nonexistent",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractField(data, tt.path)
			if tt.wantError {
				// Error can be returned as err or captured in result.Error
				hasError := err != nil || (result != nil && result.Error != nil)
				assert.True(t, hasError, "expected error")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.NoError(t, result.Error)
				assert.Equal(t, tt.want, result.Value)
			}
		})
	}
}

func TestExtractFieldJSONPath(t *testing.T) {
	data := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{
				"adapter": "landing-zone-adapter",
				"data": map[string]interface{}{
					"namespace": map[string]interface{}{
						"status": "active",
					},
				},
			},
			map[string]interface{}{
				"adapter": "other-adapter",
				"data": map[string]interface{}{
					"namespace": map[string]interface{}{
						"status": "inactive",
					},
				},
			},
			map[string]interface{}{
				"adapter": "landing-zone-adapter",
				"data": map[string]interface{}{
					"namespace": map[string]interface{}{
						"status": "pending",
					},
				},
			},
		},
		"adapter": map[string]interface{}{
			"name": "test-resource",
		},
	}

	tests := []struct {
		name      string
		want      interface{}
		path      string
		wantError bool
	}{
		{
			name: "JSONPath get all adapters",
			path: "{.items[*].adapter}",
			want: []interface{}{"landing-zone-adapter", "other-adapter", "landing-zone-adapter"},
		},
		{
			name: "JSONPath get first item adapter",
			path: "{.items[0].adapter}",
			want: "landing-zone-adapter",
		},
		{
			name: "JSONPath filter by adapter",
			path: "{.items[?(@.adapter=='landing-zone-adapter')].data.namespace.status}",
			want: []interface{}{"active", "pending"},
		},
		{
			name: "JSONPath filter single result",
			path: "{.items[?(@.adapter=='other-adapter')].data.namespace.status}",
			want: "inactive",
		},
		{
			name: "JSONPath without braces - auto-wrapped",
			path: ".items[0].adapter",
			want: "landing-zone-adapter",
		},
		{
			name: "JSONPath wildcard detected with dot prefix",
			path: ".items[*].adapter",
			want: []interface{}{"landing-zone-adapter", "other-adapter", "landing-zone-adapter"},
		},
		{
			name: "Simple path still works",
			path: "adapter.name",
			want: "test-resource",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ExtractField(data, tt.path)
			if tt.wantError {
				// Error can be returned as err or captured in result.Error
				hasError := err != nil || (result != nil && result.Error != nil)
				assert.True(t, hasError, "expected error")
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
				assert.NoError(t, result.Error)
				assert.Equal(t, tt.want, result.Value)
			}
		})
	}
}

func TestExtractFieldFunction(t *testing.T) {
	// Test the convenience ExtractField function
	data := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{"name": "a", "status": "ready"},
			map[string]interface{}{"name": "b", "status": "pending"},
		},
	}

	// Simple extraction
	result, err := ExtractField(data, "items")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotNil(t, result.Value)

	// JSONPath extraction
	result, err = ExtractField(data, "{.items[?(@.status=='ready')].name}")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "a", result.Value)

	// All items with JSONPath
	result, err = ExtractField(data, "{.items[*].name}")
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, []interface{}{"a", "b"}, result.Value)
}

func TestToFloat64(t *testing.T) {
	tests := []struct {
		value     interface{}
		name      string
		want      float64
		wantError bool
	}{
		{name: "float64", value: float64(3.14), want: 3.14},
		{name: "float32", value: float32(3.14), want: float64(float32(3.14))},
		{name: "int", value: int(42), want: 42.0},
		{name: "int8", value: int8(42), want: 42.0},
		{name: "int16", value: int16(42), want: 42.0},
		{name: "int32", value: int32(42), want: 42.0},
		{name: "int64", value: int64(42), want: 42.0},
		{name: "uint", value: uint(42), want: 42.0},
		{name: "uint8", value: uint8(42), want: 42.0},
		{name: "uint16", value: uint16(42), want: 42.0},
		{name: "uint32", value: uint32(42), want: 42.0},
		{name: "uint64", value: uint64(42), want: 42.0},
		{name: "string", value: "test", wantError: true},
		{name: "bool", value: true, wantError: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := toFloat64(tt.value)
			if tt.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestEvaluationError(t *testing.T) {
	err := &EvaluationError{
		Field:   "testField",
		Message: "test error",
	}
	assert.Contains(t, err.Error(), "testField")
	assert.Contains(t, err.Error(), "test error")

	err2 := &EvaluationError{
		Field:   "testField",
		Message: "test error",
		Err:     assert.AnError,
	}
	assert.Contains(t, err2.Error(), "testField")
	assert.Contains(t, err2.Error(), "test error")
	assert.Equal(t, assert.AnError, err2.Unwrap())
}

func TestNewEvaluatorErrorsWithNilParams(t *testing.T) {
	t.Run("errors with nil ctx", func(t *testing.T) {
		//nolint:staticcheck // intentionally testing nil ctx
		_, err := NewEvaluator(nil, NewEvaluationContext(), logger.NewTestLogger())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "ctx is required")
	})
	t.Run("errors with nil evalCtx", func(t *testing.T) {
		_, err := NewEvaluator(context.Background(), nil, logger.NewTestLogger())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "evalCtx is required")
	})
	t.Run("errors with nil log", func(t *testing.T) {
		_, err := NewEvaluator(context.Background(), NewEvaluationContext(), nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "log is required")
	})
}

func TestExtractValue(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("cluster", map[string]interface{}{
		"name": "test-cluster",
		"status": map[string]interface{}{
			"conditions": []interface{}{
				map[string]interface{}{"type": "Reconciled", "status": "True"},
			},
		},
	})

	evaluator, err := NewEvaluator(context.Background(), ctx, logger.NewTestLogger())
	require.NoError(t, err)

	// Get existing field
	result, err := evaluator.ExtractValue("cluster.name", "")
	require.NoError(t, err)
	assert.Equal(t, "test-cluster", result.Value)

	// Get nested field
	result, err = evaluator.ExtractValue("cluster.status.conditions", "")
	require.NoError(t, err)
	assert.Equal(t, []interface{}{map[string]interface{}{"type": "Reconciled", "status": "True"}}, result.Value)

	// Get non-existent field - returns nil value with result.Error set (not a returned error)
	result, err = evaluator.ExtractValue("cluster.nonexistent", "")
	assert.NoError(t, err)         // No parse error returned
	assert.Nil(t, result.Value)    // Value is nil (field not found)
	assert.NotNil(t, result.Error) // result.Error is set so callers can detect absence
}

func TestEvaluateCondition(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("status", "Ready")
	ctx.Set("replicas", 3)
	ctx.Set("provider", "aws")

	evaluator, err := NewEvaluator(context.Background(), ctx, logger.NewTestLogger())
	require.NoError(t, err)

	// Test equals - matched
	result, err := evaluator.EvaluateCondition("status", OperatorEquals, "Ready")
	require.NoError(t, err)
	assert.True(t, result.Matched)
	assert.Equal(t, "Ready", result.FieldValue)
	assert.Equal(t, "status", result.Field)
	assert.Equal(t, OperatorEquals, result.Operator)
	assert.Equal(t, "Ready", result.ExpectedValue)

	// Test equals - not matched
	result, err = evaluator.EvaluateCondition("status", OperatorEquals, "Failed")
	require.NoError(t, err)
	assert.False(t, result.Matched)
	assert.Equal(t, "Ready", result.FieldValue) // Still returns the actual value

	// Test greaterThan
	result, err = evaluator.EvaluateCondition("replicas", OperatorGreaterThan, 2)
	require.NoError(t, err)
	assert.True(t, result.Matched)
	assert.Equal(t, 3, result.FieldValue)

	// Test in operator
	result, err = evaluator.EvaluateCondition("provider", OperatorIn, []string{"aws", "gcp", "azure"})
	require.NoError(t, err)
	assert.True(t, result.Matched)
	assert.Equal(t, "aws", result.FieldValue)
}

func TestEvaluateConditions(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("status", "Ready")
	ctx.Set("replicas", 3)
	ctx.Set("provider", "aws")

	evaluator, err := NewEvaluator(context.Background(), ctx, logger.NewTestLogger())
	require.NoError(t, err)

	// All conditions pass
	conditions := []ConditionDef{
		{Field: "status", Operator: "equals", Value: "Ready"},
		{Field: "replicas", Operator: "greaterThan", Value: 1},
		{Field: "provider", Operator: "in", Value: []string{"aws", "gcp"}},
	}

	result, err := evaluator.EvaluateConditions(conditions)
	require.NoError(t, err)
	assert.True(t, result.Matched)
	assert.Equal(t, -1, result.FailedCondition)
	assert.Len(t, result.Results, 3)
	assert.Len(t, result.ExtractedFields, 3)

	// Verify extracted fields
	assert.Equal(t, "Ready", result.ExtractedFields["status"])
	assert.Equal(t, 3, result.ExtractedFields["replicas"])
	assert.Equal(t, "aws", result.ExtractedFields["provider"])

	// One condition fails
	conditions = []ConditionDef{
		{Field: "status", Operator: "equals", Value: "Ready"},
		{Field: "replicas", Operator: "greaterThan", Value: 10}, // This will fail
		{Field: "provider", Operator: "equals", Value: "aws"},
	}

	result, err = evaluator.EvaluateConditions(conditions)
	require.NoError(t, err)
	assert.False(t, result.Matched)
	assert.Equal(t, 1, result.FailedCondition) // Second condition (index 1) failed
	assert.Len(t, result.Results, 3)

	// Extracted fields are still populated even when conditions fail
	assert.Equal(t, "Ready", result.ExtractedFields["status"])
	assert.Equal(t, 3, result.ExtractedFields["replicas"])
}

func TestEvaluationResultStruct(t *testing.T) {
	result := EvaluationResult{
		Matched:       true,
		FieldValue:    "Ready",
		Field:         "status.phase",
		Operator:      OperatorEquals,
		ExpectedValue: "Ready",
	}

	assert.True(t, result.Matched)
	assert.Equal(t, "Ready", result.FieldValue)
	assert.Equal(t, "status.phase", result.Field)
	assert.Equal(t, OperatorEquals, result.Operator)
	assert.Equal(t, "Ready", result.ExpectedValue)
}

func TestNullHandling(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("cluster", map[string]interface{}{
		"name":   "test-cluster",
		"status": nil, // null value
		"spec": map[string]interface{}{
			"provider": nil, // null nested value
			"nested": map[string]interface{}{
				"value": "exists",
			},
		},
	})

	evaluator, err := NewEvaluator(context.Background(), ctx, logger.NewTestLogger())
	require.NoError(t, err)

	t.Run("access field on null parent returns nil value", func(t *testing.T) {
		// Accessing cluster.status.conditions when status is null - returns nil value (not error)
		result, err := evaluator.ExtractValue("cluster.status.conditions", "")
		assert.NoError(t, err)      // No parse error
		assert.Nil(t, result.Value) // Value is nil (field not found)
	})

	t.Run("exists operator on null returns error or false", func(t *testing.T) {
		result, err := evaluator.EvaluateCondition("cluster.status", OperatorExists, true)
		// Either error or not matched
		assert.True(t, err != nil || !result.Matched)
	})

	t.Run("existing field still works", func(t *testing.T) {
		result, err := evaluator.EvaluateCondition("cluster.name", OperatorEquals, "test-cluster")
		assert.NoError(t, err)
		assert.True(t, result.Matched)
	})
}

func TestDeepNullPath(t *testing.T) {
	ctx := NewEvaluationContext()
	ctx.Set("a", map[string]interface{}{
		"b": map[string]interface{}{
			"c": nil, // null at c
		},
	})

	evaluator, err := NewEvaluator(context.Background(), ctx, logger.NewTestLogger())
	require.NoError(t, err)

	// a.b.c is null, so a.b.c.d.e.f should return nil value (not error)
	result, err := evaluator.ExtractValue("a.b.c.d.e.f", "")
	assert.NoError(t, err)      // No parse error
	assert.Nil(t, result.Value) // Value is nil (field not found)

	// a.b exists and is not null
	result, err = evaluator.ExtractValue("a.b", "")
	assert.NoError(t, err)
	assert.NotNil(t, result.Value)
}
