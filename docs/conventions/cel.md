# CEL Expressions

> **Audience:** Framework developers contributing to the adapter codebase. For practical CEL patterns aimed at adapter authors, see [Adapter Authoring Guide — Appendix A](../adapter-authoring-guide.md#appendix-a-cel-quick-reference).

Used in precondition expressions, lifecycle delete conditions, and post-action `when` gates.

## Variables

Extracted params are injected as **top-level** names — write `clusterID`, not `params.clusterID`.

- `resources.*` — discovered resources by alias (e.g., `resources.myCluster`)
- `adapter.*` — adapter metadata (executionStatus, resourcesSkipped, skipReason, errorReason, errorMessage, executionError, resourceErrors). See `adapterMetadataToMap()` in `internal/executor/utils.go` for current fields.

## Custom Functions

### Utility

- `now()` — current time as RFC3339 string
- `toJson(val)` — serialize any value to JSON string
- `dig(map, "dot.path")` — safe nested map access, returns null if missing

### Domain-Specific

- `conditionStatus(conditions, type)` — returns the `status` field (`"True"`, `"False"`, etc.) of the first condition matching `type`, or `"Unknown"` if absent
- `conditionAge(conditions, type)` — returns elapsed seconds since `last_transition_time` for the matching condition, or `-1` if absent
- `stableFor(conditions, type, seconds)` — returns `true` only when `conditionStatus` is `"True"` AND `conditionAge` is at least the threshold
- `statusFeedbackValue(statusFeedback, name)` — returns `fieldValue.string` of the named Maestro statusFeedback value, or `""` if absent
- `triState(trueCond, falseCond)` — returns `"True"` when first arg is true, `"False"` when second is true, `"Unknown"` otherwise

## String Extensions

`ext.Strings()` is registered — available on string values:

`charAt`, `indexOf`, `lastIndexOf`, `lowerAscii`, `replace`, `split`, `substring`, `trim`, `upperAscii`, `join`

## List Extensions

`ext.Lists()` is registered — available on list values:

- `<list>.distinct()` — remove duplicate elements
- `<list>.sort()` — sort comparable elements (string, int, bool, …)
- `<list>.sortBy(e, keyExpr)` — sort objects by a derived key expression
- `<list>.slice(start, end)` — sub-list from `start` (inclusive) to `end` (exclusive)
- `<list>.flatten()` — recursively collapse nested lists; `flatten(depth)` limits depth
- `lists.range(n)` — generate `[0, 1, …, n-1]`

## Examples

```cel
// Precondition: check cluster is ready
resources.managedCluster.status.conditions.exists(c, c.type == "Ready" && c.status == "True")

// Post-action gate: check execution status
adapter.?executionStatus.orValue("") == "success"

// Post-action gate: skip when resources were skipped
adapter.?resourcesSkipped.orValue(false)

// Deduplicate and sort a tag list
spec.tags.distinct().sort()

// Sort node pools by replica count, extract names
spec.node_pools.sortBy(p, p.replicas).map(p, p.name)

// Flatten condition type+status pairs into one list
status.conditions.map(c, [c.type, c.status]).flatten()
```

### Before/After: Domain-Specific Functions

```cel
// Before — double-filter pattern (12+ occurrences per adapter config):
status.conditions.filter(c, c.type == "Reconciled").size() > 0
  ? status.conditions.filter(c, c.type == "Reconciled")[0].status
  : "Unknown"

// After:
conditionStatus(conditions, "Reconciled")
```

```cel
// Before — stability window with timestamp arithmetic:
status.conditions.filter(c, c.type == "Reconciled").size() > 0
  && status.conditions.filter(c, c.type == "Reconciled")[0].status == "True"
  && (timestamp(now()) - timestamp(
       status.conditions.filter(c, c.type == "Reconciled")[0].last_transition_time
     )).getSeconds() > 300

// After:
stableFor(conditions, "Reconciled", 300)
```

```cel
// Before — statusFeedback value extraction (4-step filter chain):
statusFeedback.values.filter(v, v.name == "phase").size() > 0
  ? statusFeedback.values.filter(v, v.name == "phase")[0].fieldValue.string
  : ""

// After:
statusFeedbackValue(statusFeedback, "phase")
```

```cel
// Before — nested ternary for tri-state condition:
isReady ? "True" : (isFailed ? "False" : "Unknown")

// After:
triState(isReady, isFailed)
```

## Reference

- CEL evaluator: `internal/criteria/cel_evaluator.go`
- Custom functions registered: `internal/criteria/cel_evaluator.go:71` (`ext.Strings()`, `ext.Lists()`)
- CEL validation at config load: `internal/configloader/validator.go`
