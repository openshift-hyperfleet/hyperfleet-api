# Criteria

The `criteria` package provides a flexible evaluation engine for evaluating conditions and criteria against dynamic data. It supports multiple operators and nested field access, making it suitable for complex condition evaluation in adapter configurations.

## Overview

This package is used to evaluate preconditions, post-conditions, and other criteria defined in the adapter configuration. It provides a type-safe, expression-based evaluation system with support for various comparison operators.

## Features

- **Multiple Operators**: equals, notEquals, in, notIn, contains, greaterThan, lessThan, exists
- **Nested Field Access**: Evaluate deeply nested fields using dot notation (e.g., `status.conditions`)
- **JSONPath Support**: Extract complex values using Kubernetes JSONPath syntax
- **Type Flexibility**: Handles strings, numbers, arrays, maps, and complex nested structures
- **Context Management**: Maintain evaluation context with variable storage and retrieval
- **Error Handling**: Descriptive error messages for debugging

## Supported Operators

| Operator | Description | Example                                 |
|----------|-------------|-----------------------------------------|
| `equals` | Field equals value | `reconciledConditionStatus == "True"`   |
| `notEquals` | Field does not equal value | `status != "Failed"`                    |
| `in` | Field is in a list of values | `provider in ["aws", "gcp", "azure"]`   |
| `notIn` | Field is not in a list of values | `phase notIn ["Terminating", "Failed"]` |
| `contains` | String/array contains value | `"hello world" contains "world"`        |
| `greaterThan` | Numeric field is greater than value | `nodeCount > 3`                         |
| `lessThan` | Numeric field is less than value | `replicas < 10`                         |
| `exists` | Field exists and is not empty | `vpcId exists`                          |

## Usage

### Basic Evaluation

```go
import "github.com/openshift-hyperfleet/hyperfleet-adapter/internal/criteria"

// Create evaluation context
ctx := criteria.NewEvaluationContext()
ctx.Set("reconciledConditionStatus", "True")
ctx.Set("provider", "aws")
ctx.Set("nodeCount", 5)

// Create evaluator
evaluator, _ := criteria.NewEvaluator(context.Background(), ctx, log)

// Evaluate a single condition
result, err := evaluator.EvaluateCondition(
    "reconciledConditionStatus",
    criteria.OperatorEquals,
    "True",
)
if err != nil {
    log.Fatal(err)
}
fmt.Println("Cluster is reconciled:", result.Matched) // true
```

### Evaluating Multiple Conditions

```go
// Multiple conditions (AND logic)
// Use typed Operator constants for compile-time safety
conditions := []criteria.ConditionDef{
    {Field: "reconciledConditionStatus", Operator: criteria.OperatorIn, Value: []interface{}{"True"}},
    {Field: "provider", Operator: criteria.OperatorIn, Value: []interface{}{"aws", "gcp", "azure"}},
    {Field: "nodeCount", Operator: criteria.OperatorGreaterThan, Value: 1},
}

result, err := evaluator.EvaluateConditions(conditions)
if err != nil {
    log.Fatal(err)
}
fmt.Println("All conditions pass:", result.Matched)
```

### Nested Field Access

```go
// Set nested data
ctx.Set("cluster", map[string]interface{}{
    "status": map[string]interface{}{
        "conditions": []interface{}{
            map[string]interface{}{
                "type":   "Reconciled",
                "status": "True",
            },
        },
    },
})

// Evaluate nested field
result, err := evaluator.EvaluateCondition(
    "{.cluster.status.conditions[?(@.type=='Reconciled')].status}",
    criteria.OperatorEquals,
    "True",
)
```

### JSONPath Extraction

The `ExtractField` function supports both simple dot notation and Kubernetes JSONPath expressions for complex data extraction. It returns a `*FieldResult` containing the extracted value.

```go
// Simple dot notation (auto-converted to JSONPath internally)
result, err := criteria.ExtractField(data, ".name")
if err != nil {
    // Parse error (invalid JSONPath syntax)
}
fmt.Println(result.Value) // extracted value, or nil if not found

// JSONPath with array index
result, err := criteria.ExtractField(data, "{.items[0].name}")

// JSONPath with wildcard (returns slice)
result, err := criteria.ExtractField(data, "{.items[*].name}")

// JSONPath with filter expression
result, err := criteria.ExtractField(data, "{.items[?(@.adapter=='landing-zone-adapter')].data.namespace.status}")
```

**FieldResult structure:**

- `Value`: The extracted value (nil if field not found or empty)
- `Error`: Runtime extraction error (e.g., field not found) - not a parse error

#### Supported JSONPath Syntax

| Syntax | Description | Example |
|--------|-------------|---------|
| `.field` | Child field | `{.name}` |
| `[n]` | Array index | `{.items[0]}` |
| `[*]` | All elements | `{.items[*].name}` |
| `[?(@.x=='y')]` | Filter by value | `{.items[?(@.status=='Ready')]}` |
| `[start:end]` | Array slice | `{.items[0:2]}` |

See: [Kubernetes JSONPath Reference](https://kubernetes.io/docs/reference/kubectl/jsonpath/)

### Unified Value Extraction

The `ExtractValue` method provides a unified interface for extracting values using either field (JSONPath) or expression (CEL). This is used by captures, conditions, and payload building.

```go
// Extract using JSONPath (using evaluator from Basic Evaluation example above)
result, err := evaluator.ExtractValue("{.status.conditions[?(@.type=='Reconciled')].status}", "")

// Extract using CEL expression
result, err = evaluator.ExtractValue("", "items.filter(i, i.status == 'active').size()")
```

The `ExtractValueResult` contains:

- `Value`: The extracted value (nil if field not found or empty)
- `Source`: The field path or expression used
- `Error`: Runtime extraction error (if any)

**Error handling:**

- Returns `error` (2nd return) only for **parse errors** (invalid JSONPath/CEL syntax)
- Field not found → `result.Value = nil` (allows caller to use default value)

### Context Management

```go
// Create context
ctx := criteria.NewEvaluationContext()

// Set values
ctx.Set("key", "value")

// Get values
val, ok := ctx.Get("key")
if ok {
    fmt.Println("Value:", val)
}

// Merge contexts
ctx2 := criteria.NewEvaluationContext()
ctx2.Set("newKey", "newValue")
ctx.Merge(ctx2) // ctx now has both key and newKey
```

#### GetField Method

```go
func (c *EvaluationContext) GetField(path string) (*FieldResult, error)
```

Retrieves a field using dot notation (e.g., `"cluster.status.conditions"`) or JSONPath (e.g., `"{.items[0].name}"`).

**Return Values:**

- `*FieldResult.Value`: The extracted value (`string`, `float64`, `bool`, `map[string]interface{}`, `[]interface{}`, or `nil` if not found)
- `*FieldResult.Error`: Runtime extraction error (e.g., JSONPath execution failure)
- `error` (2nd return): Parse error for invalid path syntax

**Error Conditions:**

| Condition | `error` (2nd return) | `result.Value` | `result.Error` |
|-----------|---------------------|----------------|----------------|
| Empty path | `"empty field path"` | - | - |
| Invalid JSONPath syntax | `"invalid field path..."` | - | - |
| Field not found | `nil` | `nil` | `nil` |
| JSONPath execution failure | `nil` | `nil` | set |

## Integration with Config Loader

The criteria package is designed to work seamlessly with conditions defined in adapter configurations:

```go
import (
    "github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
    "github.com/openshift-hyperfleet/hyperfleet-adapter/internal/criteria"
)

// Load config
config, _ := config_loader.Load("adapter-config.yaml")

// Get precondition
precond := config.GetPreconditionByName("clusterStatus")

// Create evaluation context with API response data
ctx := criteria.NewEvaluationContext()
ctx.Set("reconciledConditionStatus", "True")
ctx.Set("cloudProvider", "aws")
ctx.Set("vpcId", "vpc-12345")

// Evaluate precondition conditions
evaluator, _ := criteria.NewEvaluator(context.Background(), ctx, log)
conditions := make([]criteria.ConditionDef, len(precond.Conditions))
for i, cond := range precond.Conditions {
    conditions[i] = criteria.ConditionDef{
        Field:    cond.Field,
        Operator: criteria.Operator(cond.Operator), // Cast string to Operator type
        Value:    cond.Value,
    }
}

result, err := evaluator.EvaluateConditions(conditions)
if err != nil {
    log.Fatal(err)
}

if result.Matched {
    fmt.Println("Precondition passed - proceeding with resource creation")
} else {
    fmt.Println("Precondition failed - skipping resource creation")
}
```

## Error Handling

The package provides descriptive error messages:

```go
ctx := criteria.NewEvaluationContext()
ctx.Set("count", "not a number")

evaluator, _ := criteria.NewEvaluator(context.Background(), ctx, log)
result, err := evaluator.EvaluateCondition(
    "count",
    criteria.OperatorGreaterThan,
    5,
)

if err != nil {
    // Error message: "evaluation error for field 'count': failed to convert field value to number: ..."
    fmt.Println("Evaluation error:", err)
}
```

## Testing

The package includes comprehensive tests:

```bash
# Run all tests
go test ./internal/criteria/...

# Run with coverage
go test -cover ./internal/criteria/...

# Run integration tests
go test -v ./internal/criteria/... -run Integration
```

### Test Coverage

- Unit tests for each operator
- Nested field access tests
- Type conversion tests
- Error handling tests
- Real-world scenario tests

## Performance Considerations

- **Field Access**: Nested field lookups use reflection for struct fields
- **Type Conversions**: Numeric comparisons automatically convert between int, float, etc.
- **Context Reuse**: Reuse `EvaluationContext` and `Evaluator` for multiple evaluations
- **Memory**: Context stores references to data, not copies (where possible)

## Best Practices

1. **Reuse Contexts**: Create one context and update values between evaluations
2. **Validate Types**: Ensure values are of expected types for operators
3. **Check Errors**: Always check errors from evaluation methods
4. **Use Typed Values**: Prefer typed values over string conversions
5. **Descriptive Field Names**: Use clear, descriptive field names for debugging

## Related Packages

- `internal/configloader`: Parses adapter configurations with condition definitions
- `internal/k8sclient`: Uses criteria for resource status evaluation

## Configuration Template Examples

See `configs/adapter-task-config-template.yaml` for examples of condition usage:

```yaml
preconditions:
  - name: "clusterStatus"
    conditions:
      - field: "reconciledConditionStatus"
        operator: "equals"
        value: "True"
      - field: "cloudProvider"
        operator: "in"
        value: ["aws", "gcp", "azure"]
      - field: "vpcId"
        operator: "exists"
```
