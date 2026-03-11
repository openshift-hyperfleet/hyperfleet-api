# Claude Code Guidelines for Database Layer

## SessionFactory

Reference: `session.go`

```
type SessionFactory interface {
    Init(*config.DatabaseConfig)
    DirectDB() *sql.DB
    New(ctx context.Context) *gorm.DB    // Get transaction-aware DB session
    CheckConnection() error
    Close() error
    ResetDB()                             // For testing
    NewListener(ctx context.Context, channel string, callback func(id string))
}
```

Always use `New(ctx)` to get a GORM session — never create direct DB connections. This ensures participation in the request transaction.

## Transaction Middleware

Reference: `transaction_middleware.go`

`TransactionMiddleware(next http.Handler, connection SessionFactory) http.Handler`

Flow:
1. Creates new context with `NewContext(r.Context(), connection)` — begins transaction
2. Extracts transaction ID via `db_context.TxID(ctx)`
3. Sets transaction ID in logger context for trace correlation
4. Defers `Resolve(r.Context())` for automatic rollback/commit
5. Passes modified request to next handler

## MarkForRollback

On any write error in DAOs, call `MarkForRollback(ctx, err)`. Without this, the middleware will commit a partially-failed transaction.

## Related CLAUDE.md Files

- `pkg/dao/CLAUDE.md` — DAOs that consume SessionFactory
- `pkg/services/CLAUDE.md` — Services that depend on transactions
