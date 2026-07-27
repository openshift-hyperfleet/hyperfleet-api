package util

import (
	"encoding/json"
	"strconv"
	"strings"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
)

// CELCostLimit is the maximum cost allowed for CEL expression evaluation.
// Prevents CPU spikes from misconfigured or pathological expressions.
// Used by both runtime evaluation and startup validation.
const CELCostLimit = 10000

// CEL context variable names (QUAL-01: prevent silent name mismatches)
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
	data, err := json.Marshal(v)
	if err != nil {
		return types.NewErr("toJson: %v", err)
	}

	// Size guard: prevent unbounded intermediate allocations from large payloads
	// Limit matches Kubernetes ConfigMap max size (1MB) as a reasonable upper bound
	const maxJSONSize = 1 * 1024 * 1024 // 1MB
	if len(data) > maxJSONSize {
		return types.NewErr("toJson: output exceeds 1MB limit (%d bytes)", len(data))
	}

	return types.String(string(data))
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
