package util

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// limitedWriter is an io.Writer that stops writing after a size limit is exceeded.
// Used by toJSONFunc to bound allocation during JSON encoding.
type limitedWriter struct {
	data     []byte
	limit    int
	exceeded bool
}

// Write implements io.Writer. Returns an error when the limit is exceeded.
func (w *limitedWriter) Write(p []byte) (n int, err error) {
	if w.exceeded {
		return 0, errors.New("size limit exceeded")
	}

	// Check if adding this chunk would exceed the limit
	if len(w.data)+len(p) > w.limit {
		w.exceeded = true
		return 0, errors.New("size limit exceeded")
	}

	w.data = append(w.data, p...)
	return len(p), nil
}

// CELCostLimit is the maximum cost allowed for CEL expression evaluation.
// Prevents CPU spikes from misconfigured or pathological expressions.
// Used by both runtime evaluation and startup validation.
const CELCostLimit = 10000

// CEL context variable names
// These constants ensure consistency between environment declaration and activation map
const (
	CELVarStatuses = "statuses" // Array of adapter statuses
	CELVarResource = "resource" // Full cluster/nodepool object
	CELVarEnv      = "env"      // Environment variables map
)

// NewConditionMappingEnvironment creates a CEL environment for condition mapping
// with context variables and custom functions.
// This environment is used both for validation (at config load time) and runtime evaluation.
func NewConditionMappingEnvironment() (*cel.Env, error) {
	return cel.NewEnv(
		// Enable optional chaining for safe navigation
		cel.OptionalTypes(),

		// Context variables
		cel.Variable(CELVarStatuses, cel.ListType(cel.DynType)),
		cel.Variable(CELVarResource, cel.DynType),
		cel.Variable(CELVarEnv, cel.MapType(cel.StringType, cel.StringType)),

		// Custom functions (reused from hyperfleet-adapter patterns)
		cel.Function("toJson",
			cel.Overload("toJson_dyn",
				[]*cel.Type{cel.DynType},
				cel.StringType,
				cel.UnaryBinding(toJSONFunc))),

		cel.Function("dig",
			cel.Overload("dig_dyn_string",
				[]*cel.Type{cel.DynType, cel.StringType},
				cel.DynType,
				cel.BinaryBinding(digFunc))),
	)
}

// toJSONFunc implements the toJson() CEL function
func toJSONFunc(val ref.Val) ref.Val {
	v := val.Value()

	// Size guard: prevent unbounded intermediate allocations from large payloads
	// Limit matches Kubernetes ConfigMap max size (1MB) as a reasonable upper bound
	const maxJSONSize = 1 * 1024 * 1024 // 1MB

	// Use json.Encoder with a size-limited writer to bound allocation during encoding
	// (not after). This prevents OOM when encoding very large structures.
	var buf limitedWriter
	buf.limit = maxJSONSize
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false) // Match json.Marshal behavior

	if err := enc.Encode(v); err != nil {
		if buf.exceeded {
			return types.NewErr("toJson: output exceeds 1MB limit")
		}
		return types.NewErr("toJson: %v", err)
	}

	// json.Encoder.Encode appends a trailing newline — trim it to preserve
	// current behavior and match json.Marshal output
	result := buf.data
	if len(result) > 0 && result[len(result)-1] == '\n' {
		result = result[:len(result)-1]
	}

	return types.String(string(result))
}

// digFunc implements the dig() CEL function for safe nested navigation
// Supports both map keys and array indices (e.g., "statuses.0.conditions.1.type")
func digFunc(target ref.Val, path ref.Val) ref.Val {
	pathStr, ok := path.Value().(string)
	if !ok {
		return types.NullValue
	}

	pathStr = strings.TrimSpace(pathStr)
	if pathStr == "" {
		return target
	}

	// Navigate through nested map/list structure
	current := target.Value()
	parts := strings.Split(pathStr, ".")
	for _, rawPart := range parts {
		part := strings.TrimSpace(rawPart)
		if part == "" {
			continue
		}

		switch v := current.(type) {
		case map[string]interface{}:
			next, found := v[part]
			if !found {
				return types.NullValue
			}
			current = next
		case []interface{}:
			// Try parsing as array index
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(v) {
				return types.NullValue
			}
			current = v[idx]
		default:
			return types.NullValue
		}
	}

	return types.DefaultTypeAdapter.NativeToValue(current)
}
