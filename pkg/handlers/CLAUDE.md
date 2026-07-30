# Claude Code Guidelines for Handlers

## Request Processing

Handlers follow a standard pipeline:

1. **Decode and validate** — Use `decodeAndValidate(r, &req, validateFuncs)` to unmarshal request body and run validation
   - Pass `"strict"` as 4th parameter for PATCH requests only to reject unknown/immutable fields via `DisallowUnknownFields()`
   - Validation functions return `*errors.ServiceError` on failure
2. **Convert** — Use `presenters.Convert*()` to convert request types to internal models
3. **Call service** — Pass converted models to service layer methods
4. **Present** — Use `presenters.Present*()` to convert internal models to response types
5. **Write response** — Use `writeJSONResponse(w, r, statusCode, data)` or `handleError(r, w, err)`

## Handler Types

- **ResourceHandler** — Handles both flat (`/api/hyperfleet/v1/clusters`) and owner-nested (`/api/hyperfleet/v1/clusters/{parent_id}/nodepools`) routes for a single entity kind
  - Extract parent_id to determine route type - `r.PathValue("parent_id")`
  - See struct comment for safety invariant (plugins must register correctly)
- **RootResourceHandler** — Handles kind-agnostic routes (`/api/hyperfleet/v1/resources`, `/api/hyperfleet/v1/resources/{id}`)
  - Resolves entity kind dynamically via `registry.Get(kind)`
  - Handles adapter status operations (`/api/hyperfleet/v1/resources/{id}/statuses`)

## Validation Patterns

Validation functions close over the request struct and return `func() *errors.ServiceError`:

```go
validateFuncs := []validate{
    validateKind(&req, "Kind", "kind", expectedKind),
    validateName(&req, "Name", "name", minLen, maxLen),
    validateSpec(&req, "Spec", "spec"),
    validateLabels(&req, "Labels"),
}
```

Common validators (in `validation.go`):
- `validateKind(req, fieldName, jsonName, expectedKind)` — Ensures `kind` matches descriptor
- `validateName(req, fieldName, jsonName, minLen, maxLen)` — Name length constraints
- `validateSpec(req, fieldName, jsonName)` — Spec is non-nil
- `validateLabels(req, fieldName)` — Labels follow RFC 1123 rules
- `validatePatchRequest(req)` — At least one field is set
- `validateNotEmpty(req, fieldName, jsonName)` — String field is non-empty
- `validateMaxLength(req, fieldName, jsonName, maxLen)` — String length constraint

## Owner Verification

For nested routes, verify the child belongs to the parent:

```go
if err := h.checkOwnership(r, id); err != nil {
    handleError(r, w, err)
    return
}
```

This is a no-op for flat routes (no `parent_id` in path).

## Error Handling

Use `handleError(r, w, err)` to write RFC 9457 Problem Details responses:
- Logs errors with structured fields (trace ID, error code, HTTP status)
- Automatically extracts trace ID from request context
- Sets `instance` to the request path
- Error constructor functions (from `pkg/errors`)


## Path Parameters

Extract route parameters with r.PathValue

```go
id := r.PathValue("id")
// Extract parent_id
if parentID := r.PathValue("parent_id"); parentID != "" {
    if _, svcErr := h.resourceService.GetByOwner(ctx, h.descriptor.Kind, id, parentID); svcErr != nil {
        handleError(r, w, svcErr)
        return
    }
}
```

## Field Filtering

Apply `?fields=` query parameter filtering:

```go
result, svcErr := applyFieldFilter(r, presenters.PresentResource(resource))
if svcErr != nil {
    handleError(r, w, svcErr)
    return
}

```

For list responses:
```go
if listArgs.Fields != nil {
    filtered, svcErr := presenters.SliceFilter(listArgs.Fields, result)
    if svcErr != nil {
        handleError(r, w, svcErr)
        return  // REQUIRED — omitting this causes a double-write
    }
    writeJSONResponse(w, r, http.StatusOK, filtered)
    return  // REQUIRED — prevents falling through to the unfiltered write below
}
writeJSONResponse(w, r, http.StatusOK, result)
```

## Related Files

- `pkg/services/CLAUDE.md` — Service layer patterns
- `pkg/errors/CLAUDE.md` — Error constructors and codes
- `helpers.go` — `writeJSONResponse`,`decodeAndValidate`, `handleError`, `applyFieldFilter` implementations
- `validation.go` — Validator functions (`validateKind`, `validateName`, `validateSpec`, etc.)