# Claude Code Guidelines for Database Layer

## SessionFactory

Reference: `session.go`

```
type SessionFactory interface {
    Init(*config.DatabaseConfig)
    DirectDB() *sql.DB
    New(ctx context.Context) *gorm.DB    // Get active transaction or non-transactional DB session
    CheckConnection() error
    Close() error
    ResetDB()                             // For testing
    NewListener(ctx context.Context, channel string, callback func(id string))
}
```

Always use `New(ctx)` to get a GORM session — never create direct DB connections. When the context contains an active service-owned transaction, `New(ctx)` returns that transaction; otherwise, it returns a non-transactional session.

## Transactions

Reference: `transactions.go`

`TxRunner.Do(ctx, callback)` owns application write transactions. The container
constructs the runner from its session factory; services receive the runner and
own one public mutation boundary each.

Concrete mutation services open one transaction around their complete atomic operation.
The callback receives the transaction context; return an error to roll back and
return nil to commit before the service returns success. Nested transactions are rejected.
Read-only services do not open transactions. `RequestTimeoutMiddleware` applies request
deadlines to both reads and writes without owning transactions.

**Mutation Flow**:
1. Request timeout middleware derives a deadline context.
2. The mutation service calls its injected `TxRunner.Do` dependency.
3. The runner begins one transaction and passes a derived transaction context to the callback.
4. Callback errors and panics roll back; a nil callback error commits before service success returns.
5. Nested transaction creation is rejected. Reusable work participates through
   private helpers rather than calling another public mutation service.

**Read Request Flow**:
1. Applies request timeout context
2. Skips transaction creation (performance optimization)
3. DAOs get non-transactional sessions via `SessionFactory.New(ctx)`
4. No transaction overhead

**Trade-off**: List operations (COUNT + SELECT) may show inconsistent pagination totals under concurrent deletes (cosmetic issue, low probability).

DAOs return ordinary errors and never decide whether to roll back. Always pass the
callback context to every DAO call, including reads and locking reads.

## Related CLAUDE.md Files

- `pkg/dao/CLAUDE.md` — DAOs that consume SessionFactory
- `pkg/services/CLAUDE.md` — Services that depend on transactions
