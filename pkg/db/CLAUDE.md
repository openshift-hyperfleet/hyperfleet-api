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

`TransactionMiddleware(next http.Handler, connection SessionFactory, requestTimeout time.Duration) http.Handler`

**Transaction Strategy (Optimized for Performance)**:
- **Write operations** (POST/PUT/PATCH/DELETE): Create GORM transactions for ACID guarantees
- **Read operations** (GET): Skip transaction creation for performance

**Write Request Flow**:
1. Applies request timeout context
2. Creates new context with `NewContext(ctx, connection)` — begins transaction
3. Extracts transaction ID via `db_context.TxID(ctx)`
4. Sets transaction ID in logger context for trace correlation
5. Defers `Resolve(txCtx)` for automatic rollback/commit
6. Passes modified request to next handler

**Read Request Flow**:
1. Applies request timeout context
2. Skips transaction creation (performance optimization)
3. DAOs get non-transactional sessions via `SessionFactory.New(ctx)`
4. No transaction overhead

**Trade-off**: List operations (COUNT + SELECT) may show inconsistent pagination totals under concurrent deletes (cosmetic issue, low probability).

## MarkForRollback

On any write error in DAOs, call `MarkForRollback(ctx, err)`. Without this, the middleware will commit a partially-failed transaction.

## Related CLAUDE.md Files

- `pkg/dao/CLAUDE.md` — DAOs that consume SessionFactory
- `pkg/services/CLAUDE.md` — Services that depend on transactions
