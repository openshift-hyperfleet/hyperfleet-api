package criteria

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/ext"
	apperrors "github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/errors"
)

// CELEvaluator evaluates CEL expressions against a context
type CELEvaluator struct {
	env     *cel.Env
	evalCtx *EvaluationContext
}

// CELResult contains the result of evaluating a CEL expression.
// When using EvaluateSafe, errors are captured in Error instead of being returned,
// allowing the caller to decide how to handle failures (e.g., treat as false, log, etc.).
type CELResult struct {
	// Value is the result of the CEL expression evaluation (nil if error)
	Value interface{}
	// Error indicates if evaluation failed (nil if successful)
	// Common causes: "field not found", "null value access", "type mismatch"
	Error error
	// ValueType is the CEL type of Value (e.g., "bool", "string", "int", "map", "list")
	// Empty when evaluation failed
	ValueType string
	// Expression is the original expression that was evaluated
	Expression string
	// Matched indicates if the result is boolean true (for conditions)
	// Always false when Error is set
	Matched bool
}

// HasError returns true if the evaluation resulted in an error
func (r *CELResult) HasError() bool {
	return r.Error != nil
}

// newCELEvaluator creates a new CEL evaluator with the given context
// NOTE: Caller (NewEvaluator) is responsible for parameter validation
func newCELEvaluator(evalCtx *EvaluationContext) (*CELEvaluator, error) {
	// Build CEL environment with variables from context
	options := buildCELOptions(evalCtx)

	env, err := cel.NewEnv(options...)
	if err != nil {
		return nil, apperrors.NewCELEnvError("failed to initialize", err)
	}

	return &CELEvaluator{
		env:     env,
		evalCtx: evalCtx,
	}, nil
}

// buildCELOptions creates CEL environment options from the context
// Variables are dynamically registered based on what's in ctx.Data()
func buildCELOptions(ctx *EvaluationContext) []cel.EnvOption {
	options := make([]cel.EnvOption, 0)

	// Enable optional types for optional chaining syntax (e.g., a.?b.?c)
	options = append(options, cel.OptionalTypes())
	options = append(options, ext.Strings())
	options = append(options, ext.Lists())
	options = append(options, customCELFunctions()...)

	// Get a snapshot of the data for thread safety
	data := ctx.Data()
	for key, value := range data {
		celType := inferCELType(value)
		options = append(options, cel.Variable(key, celType))
	}

	return options
}

// customCELFunctions registers helper functions used by config expressions.
// These helpers are primarily for payload construction where deeply nested
// resources/discoveries can be difficult to inspect safely.
func customCELFunctions() []cel.EnvOption {
	return []cel.EnvOption{
		cel.Function("toJson",
			cel.Overload(
				"toJson_dyn",
				[]*cel.Type{cel.DynType},
				cel.StringType,
				cel.UnaryBinding(func(arg ref.Val) ref.Val {
					value, ok := unwrapCELValue(arg)
					if !ok {
						return types.NewErr("toJson() received invalid value")
					}
					b, err := json.Marshal(value)
					if err != nil {
						return types.NewErr("toJson() failed to marshal value: %v", err)
					}
					return types.String(string(b))
				}),
			),
		),
		cel.Function("dig",
			cel.Overload(
				"dig_dyn_string",
				[]*cel.Type{cel.DynType, cel.StringType},
				cel.DynType,
				cel.BinaryBinding(func(target ref.Val, path ref.Val) ref.Val {
					targetValue, ok := unwrapCELValue(target)
					if !ok {
						return types.NewErr("dig() received invalid target")
					}

					pathValue, ok := path.Value().(string)
					if !ok {
						return types.NewErr("dig() path must be a string")
					}

					found, exists := digValue(targetValue, pathValue)
					if !exists {
						return types.NullValue
					}
					return types.DefaultTypeAdapter.NativeToValue(found)
				}),
			),
		),
		cel.Function("now",
			cel.Overload(
				"now_string",
				[]*cel.Type{},
				cel.StringType,
				cel.FunctionBinding(func(args ...ref.Val) ref.Val {
					return types.String(time.Now().Format(time.RFC3339))
				}),
			),
		),
		cel.Function("conditionStatus",
			cel.Overload(
				"conditionStatus_list_string",
				[]*cel.Type{cel.ListType(cel.DynType), cel.StringType},
				cel.StringType,
				cel.BinaryBinding(func(listArg ref.Val, typeArg ref.Val) ref.Val {
					condType, ok := typeArg.Value().(string)
					if !ok {
						return types.NewErr("conditionStatus() type must be a string")
					}
					conditions, ok := unwrapCELList(listArg)
					if !ok {
						return types.NewErr("conditionStatus() conditions must be a list")
					}
					status, _ := findCondition(conditions, condType)
					return types.String(status)
				}),
			),
		),
		cel.Function("conditionAge",
			cel.Overload(
				"conditionAge_list_string",
				[]*cel.Type{cel.ListType(cel.DynType), cel.StringType},
				cel.IntType,
				cel.BinaryBinding(func(listArg ref.Val, typeArg ref.Val) ref.Val {
					condType, ok := typeArg.Value().(string)
					if !ok {
						return types.NewErr("conditionAge() type must be a string")
					}
					conditions, ok := unwrapCELList(listArg)
					if !ok {
						return types.NewErr("conditionAge() conditions must be a list")
					}
					_, cond := findCondition(conditions, condType)
					if cond == nil {
						return types.Int(-1)
					}
					transitionTime, ok := cond["last_transition_time"].(string)
					if !ok {
						return types.Int(-1)
					}
					t, err := time.Parse(time.RFC3339, transitionTime)
					if err != nil {
						return types.Int(-1)
					}
					return types.Int(int64(time.Since(t).Seconds()))
				}),
			),
		),
		cel.Function("stableFor",
			cel.Overload(
				"stableFor_list_string_int",
				[]*cel.Type{cel.ListType(cel.DynType), cel.StringType, cel.IntType},
				cel.BoolType,
				cel.FunctionBinding(func(args ...ref.Val) ref.Val {
					if len(args) != 3 {
						return types.NewErr("stableFor() requires 3 arguments")
					}
					condType, ok := args[1].Value().(string)
					if !ok {
						return types.NewErr("stableFor() type must be a string")
					}
					threshold, ok := args[2].Value().(int64)
					if !ok {
						return types.NewErr("stableFor() seconds must be an int")
					}
					conditions, ok := unwrapCELList(args[0])
					if !ok {
						return types.NewErr("stableFor() conditions must be a list")
					}
					status, cond := findCondition(conditions, condType)
					if status != "True" || cond == nil {
						return types.Bool(false)
					}
					transitionTime, ok := cond["last_transition_time"].(string)
					if !ok {
						return types.Bool(false)
					}
					t, err := time.Parse(time.RFC3339, transitionTime)
					if err != nil {
						return types.Bool(false)
					}
					return types.Bool(int64(time.Since(t).Seconds()) >= threshold)
				}),
			),
		),
		cel.Function("statusFeedbackValue",
			cel.Overload(
				"statusFeedbackValue_dyn_string",
				[]*cel.Type{cel.DynType, cel.StringType},
				cel.StringType,
				cel.BinaryBinding(func(feedbackArg ref.Val, nameArg ref.Val) ref.Val {
					name, ok := nameArg.Value().(string)
					if !ok {
						return types.NewErr("statusFeedbackValue() name must be a string")
					}
					feedback, ok := unwrapCELValue(feedbackArg)
					if !ok {
						return types.String("")
					}
					feedbackMap, ok := feedback.(map[string]interface{})
					if !ok {
						return types.String("")
					}
					values, ok := feedbackMap["values"].([]interface{})
					if !ok {
						return types.String("")
					}
					for _, v := range values {
						entry, ok := v.(map[string]interface{})
						if !ok {
							continue
						}
						if entry["name"] == name {
							if fv, ok := entry["fieldValue"].(map[string]interface{}); ok {
								if s, ok := fv["string"].(string); ok {
									return types.String(s)
								}
							}
						}
					}
					return types.String("")
				}),
			),
		),
		cel.Function("triState",
			cel.Overload(
				"triState_bool_bool",
				[]*cel.Type{cel.BoolType, cel.BoolType},
				cel.StringType,
				cel.BinaryBinding(func(trueArg ref.Val, falseArg ref.Val) ref.Val {
					if trueArg.Value() == true {
						return types.String("True")
					}
					if falseArg.Value() == true {
						return types.String("False")
					}
					return types.String("Unknown")
				}),
			),
		),
	}
}

// findCondition searches a conditions list for a matching type.
// Returns the status string and the condition map if found, or ("Unknown", nil) if absent.
func findCondition(conditions []interface{}, condType string) (string, map[string]interface{}) {
	for _, c := range conditions {
		cond, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		if cond["type"] == condType {
			if status, ok := cond["status"].(string); ok {
				return status, cond
			}
			return "Unknown", cond
		}
	}
	return "Unknown", nil
}

// unwrapCELList converts a CEL ref.Val list to a Go []interface{}.
func unwrapCELList(val ref.Val) ([]interface{}, bool) {
	raw, ok := unwrapCELValue(val)
	if !ok {
		return nil, false
	}
	if list, ok := raw.([]interface{}); ok {
		return list, true
	}
	return nil, false
}

func unwrapCELValue(value ref.Val) (interface{}, bool) {
	if value == nil {
		return nil, true
	}
	if _, isErr := value.(*types.Err); isErr {
		return nil, false
	}
	return value.Value(), true
}

// digValue safely traverses map/list structures using dot-separated paths.
// Returns (nil, false) when a path segment does not exist.
func digValue(root interface{}, path string) (interface{}, bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return root, true
	}

	current := root
	parts := strings.Split(path, ".")
	for _, rawPart := range parts {
		part := strings.TrimSpace(rawPart)
		if part == "" {
			continue
		}

		switch v := current.(type) {
		case map[string]interface{}:
			next, ok := v[part]
			if !ok {
				return nil, false
			}
			current = next
		case []interface{}:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(v) {
				return nil, false
			}
			current = v[idx]
		default:
			return nil, false
		}
	}

	return current, true
}

// inferCELType infers the CEL type from a Go value
func inferCELType(value interface{}) *cel.Type {
	if value == nil {
		return cel.DynType
	}

	switch value.(type) {
	case string:
		return cel.StringType
	case bool:
		return cel.BoolType
	case int, int8, int16, int32, int64:
		return cel.IntType
	case uint, uint8, uint16, uint32, uint64:
		return cel.UintType
	case float32, float64:
		return cel.DoubleType
	case []interface{}:
		return cel.ListType(cel.DynType)
	case map[string]interface{}:
		return cel.MapType(cel.StringType, cel.DynType)
	default:
		return cel.DynType
	}
}

// EvaluateSafe evaluates a CEL expression with safe handling for evaluation errors.
//
// Error handling strategy:
//   - Parse errors: returned as error (fail fast - indicates bug in expression)
//   - Program creation errors: returned as error (fail fast - indicates invalid expression)
//   - Evaluation errors: captured in CELResult.Error (safe - data might not exist yet)
//
// Use this when you expect that some fields might not exist or be null, and you want
// to handle those cases gracefully (e.g., treat as "not matched") rather than failing.
//
// Common evaluation error reasons captured in result:
//   - "field not found": when accessing a key that doesn't exist (e.g., data.missing.field)
//   - "null value access": when accessing a field on a null value
//   - "type mismatch": when operations are applied to incompatible types
func (e *CELEvaluator) EvaluateSafe(expression string) (*CELResult, error) {
	expression = strings.TrimSpace(expression)
	if expression == "" {
		return &CELResult{
			Value:      true,
			Matched:    true,
			ValueType:  "bool",
			Expression: expression,
		}, nil
	}

	// Parse the expression - errors here indicate bugs in configuration
	ast, issues := e.env.Parse(expression)
	if issues != nil && issues.Err() != nil {
		return nil, apperrors.NewCELParseError(expression, issues.Err())
	}

	// Safety check: ensure AST is valid after parse
	if ast == nil {
		return nil, apperrors.NewCELParseError(expression, nil)
	}

	// Create the program directly from parsed AST
	// Skip type-check: we use DynType, so type errors are caught during evaluation
	prg, err := e.env.Program(ast)
	if err != nil {
		return nil, apperrors.NewCELProgramError(expression, err)
	}

	// Evaluate the expression - errors here are SAFE (data might not exist yet)
	// Get a snapshot of the data for thread-safe evaluation
	out, _, err := prg.Eval(e.evalCtx.Data())
	if err != nil {
		// Capture evaluation error in result - this is the "safe" part
		// These errors are expected when data fields don't exist yet
		// Caller should handle logging based on CELResult.Error
		return &CELResult{
			Value:      nil,
			Matched:    false,
			Expression: expression,
			Error:      apperrors.NewCELEvalError(expression, err),
		}, nil // No error returned - evaluation errors are captured in result
	}

	// Convert result
	result := &CELResult{
		Value:      out.Value(),
		ValueType:  out.Type().TypeName(),
		Expression: expression,
	}

	// Check if result is boolean true
	// This is the most common use case for CEL expressions
	// has("result.value") will result the value to bool
	if boolVal, ok := out.Value().(bool); ok {
		result.Matched = boolVal
	} else {
		// Non-boolean results are considered "matched" if not nil/empty
		// This can used to dig values from the result
		// For example, if the result is a map, you can use result.value.key to get the value of the key
		result.Matched = !isEmptyValue(out)
	}

	return result, nil
}

// EvaluateAs evaluates a CEL expression and returns the result as the specified type.
// This is a type-safe generic function that handles all type assertions properly.
// Returns an error if:
//   - Parse/program error occurs (from EvaluateSafe)
//   - Evaluation error occurs (captured in result.Error)
//   - Type assertion fails (returns CELTypeMismatchError)
func EvaluateAs[T any](e *CELEvaluator, expression string) (T, error) {
	var zero T
	result, err := e.EvaluateSafe(expression)
	if err != nil {
		return zero, err
	}
	if result.Error != nil {
		return zero, result.Error
	}

	val, ok := result.Value.(T)
	if !ok {
		return zero, apperrors.NewCELTypeMismatchError(expression,
			fmt.Sprintf("%T", zero), fmt.Sprintf("%T", result.Value))
	}
	return val, nil
}

// EvaluateBool evaluates a CEL expression that should return a boolean.
func (e *CELEvaluator) EvaluateBool(expression string) (bool, error) {
	return EvaluateAs[bool](e, expression)
}

// EvaluateString evaluates a CEL expression that should return a string.
func (e *CELEvaluator) EvaluateString(expression string) (string, error) {
	return EvaluateAs[string](e, expression)
}

// EvaluateInt evaluates a CEL expression that should return an int64.
func (e *CELEvaluator) EvaluateInt(expression string) (int64, error) {
	return EvaluateAs[int64](e, expression)
}

// EvaluateUint evaluates a CEL expression that should return a uint64.
func (e *CELEvaluator) EvaluateUint(expression string) (uint64, error) {
	return EvaluateAs[uint64](e, expression)
}

// EvaluateFloat64 evaluates a CEL expression that should return a float64.
func (e *CELEvaluator) EvaluateFloat64(expression string) (float64, error) {
	return EvaluateAs[float64](e, expression)
}

// EvaluateArray evaluates a CEL expression that should return a slice.
func (e *CELEvaluator) EvaluateArray(expression string) ([]any, error) {
	return EvaluateAs[[]any](e, expression)
}

// EvaluateMap evaluates a CEL expression that should return a map.
func (e *CELEvaluator) EvaluateMap(expression string) (map[string]any, error) {
	return EvaluateAs[map[string]any](e, expression)
}

// isEmptyValue checks if a CEL value is empty/nil
func isEmptyValue(val ref.Val) bool {
	if val == nil {
		return true
	}

	switch v := val.(type) {
	case types.Null:
		return true
	case types.String:
		return string(v) == ""
	case types.Bool:
		return false // Boolean values (true or false) are never empty
	default:
		// Check if it's a list or map
		if lister, ok := val.(interface{ Size() ref.Val }); ok {
			size := lister.Size()
			if intSize, ok := size.(types.Int); ok {
				return int64(intSize) == 0
			}
		}
		return false
	}
}
