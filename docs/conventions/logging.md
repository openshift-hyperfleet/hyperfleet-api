# Logging Conventions

Uses `log/slog` wrapped in `pkg/logger`. Every log call takes `context.Context` as first parameter.

## Patterns

```go
// Basic
logger.Info(ctx, "message")

// Error logging — dominant pattern: attach error to context
errCtx := logger.WithErrorField(ctx, err)
logger.Errorf(errCtx, "Operation failed")

// Structured fields on context (carried through call chain)
ctx = logger.WithLogField(ctx, "cluster_id", clusterID)
```

## Additional API

- `logger.WithLogFields(ctx, logger.LogFields{...})` — multiple fields at once
- `logger.With("key", val)` — returns new logger with field (not context-based)
- `logger.Without("key")` — returns new logger with field removed

## Reference

- Logger interface: `pkg/logger/logger.go`
- Context helpers: `pkg/logger/context.go`
