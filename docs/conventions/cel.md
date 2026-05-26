# CEL Expressions

Used in precondition expressions, lifecycle delete conditions, and post-action `when` gates.

## Variables

Extracted params are injected as **top-level** names — write `clusterID`, not `params.clusterID`.

- `resources.*` — discovered resources by alias (e.g., `resources.myCluster`)
- `adapter.*` — adapter metadata (executionStatus, resourcesSkipped, skipReason, errorReason, errorMessage, executionError, resourceErrors). See `adapterMetadataToMap()` in `internal/executor/utils.go` for current fields.

## Custom Functions

- `now()` — current time as RFC3339 string
- `toJson(val)` — serialize any value to JSON string
- `dig(map, "dot.path")` — safe nested map access, returns null if missing

## String Extensions

`ext.Strings()` is registered — available on string values:

`charAt`, `indexOf`, `lastIndexOf`, `lowerAscii`, `replace`, `split`, `substring`, `trim`, `upperAscii`, `join`

## Examples

```cel
// Precondition: check cluster is ready
resources.managedCluster.status.conditions.exists(c, c.type == "Ready" && c.status == "True")

// Post-action gate: check execution status
adapter.?executionStatus.orValue("") == "success"

// Post-action gate: skip when resources were skipped
adapter.?resourcesSkipped.orValue(false)
```

## Reference

- CEL evaluator: `internal/criteria/cel_evaluator.go`
- Custom functions registered: `internal/criteria/cel_evaluator.go:71` (`ext.Strings()`)
- CEL validation at config load: `internal/configloader/validator.go`
