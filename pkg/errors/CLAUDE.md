# Claude Code Guidelines for Errors

Reference: `errors.go`

## ServiceError Type

```
type ServiceError struct {
    RFC9457Code string           // HYPERFLEET-CAT-NUM format
    Type        string           // RFC 9457 type URI
    Title       string           // Short human-readable summary
    Reason      string           // Context-specific detail (supports fmt.Sprintf)
    HttpCode    int              // HTTP status code
    Details     []ValidationDetail  // Field-level validation errors
}
```

## Error Code Categories

Format: `HYPERFLEET-[CAT]-[NUM]`

| Category | Code | Meaning |
|---|---|---|
| VAL | Validation | Request validation failures |
| AUT | Auth | Authentication errors |
| AUZ | Authz | Authorization errors |
| NTF | NotFound | Resource not found |
| CNF | Conflict | Resource conflicts |
| LMT | RateLimit | Rate limiting |
| INT | Internal | Internal server errors |
| SVC | Service | Upstream service errors |

## Constructor Functions

- `NotFound(reason, values...)` — 404
- `GeneralError(reason, values...)` — 500
- `Validation(reason, values...)` — 400
- `ValidationWithDetails(reason, details)` — 400 with field-level errors
- `MalformedRequest(reason, values...)` — 400 (unparseable body)
- `BadRequest(reason, values...)` — 400
- `Unauthorized(reason, values...)` — 401
- `Conflict(reason, values...)` — 409
- `FailedToParseSearch(reason, values...)` — 400
- `New(code, reason, values...)` — Custom error from registered code

## RFC 9457 Problem Details

`AsProblemDetails(instance, traceID)` converts to `openapi.Error` with:
- `type` URI, `title`, `detail` (reason), `status` (HTTP code)
- `code` (HYPERFLEET-CAT-NUM), `timestamp`, `traceId`
- `validationErrors` array for field-level details

## Related CLAUDE.md Files

- `pkg/handlers/CLAUDE.md` — Handlers that return these errors
- `pkg/services/CLAUDE.md` — Services that construct these errors
