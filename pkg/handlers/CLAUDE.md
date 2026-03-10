# Claude Code Guidelines for Handlers

Handlers use the `handlerConfig` pipeline defined in `framework.go`.

## Pipeline Functions

- `handle(w, r, cfg, httpStatus)` — Full pipeline: unmarshal `cfg.MarshalInto` from request body → run `cfg.Validate` functions → execute `cfg.Action` → write response
- `handleGet(w, r, cfg)` — No body read, action only, returns 200
- `handleList(w, r, cfg)` — No body read, action only, returns 200
- `handleDelete(w, r, cfg)` — No body read, action only, returns 204
- `handleCreateWithNoContent(w, r, cfg)` — Body read + validate, returns 204 if action returns nil

## handlerConfig Fields

- `MarshalInto`: pointer to struct for JSON unmarshalling (e.g., `&openapi.ClusterCreateRequest{}`)
- `Validate`: slice of `func() *errors.ServiceError` — each returns nil on success
- `Action`: `func() (interface{}, *errors.ServiceError)` — core business logic
- `ErrorHandler`: optional custom error handler (rarely used)

## Patterns

- Validation helpers check fields on the unmarshalled struct and return `errors.Validation("reason")` on failure
- Use `presenters.Convert*()` to convert request types to internal models before calling services
- Use `presenters.Present*()` to convert internal models to response types
- Handler structs hold service interfaces — never access DAOs directly from handlers
- Extract path params via `mux.Vars(r)` (gorilla/mux)

## Error Responses

Errors are written via `handleError()` in `framework.go` which calls `err.AsProblemDetails()` for RFC 9457 format. The trace ID comes from `r.Header.Get("X-Request-Id")`.

## Related CLAUDE.md Files

- `pkg/services/CLAUDE.md` — Service layer patterns
- `pkg/errors/CLAUDE.md` — Error constructors and codes
