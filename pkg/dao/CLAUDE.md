# Claude Code Guidelines for DAOs

## Interface Pattern

Every DAO defines an interface + concrete `sql*Dao` implementation.

Reference: `cluster.go` — `ClusterDao` interface, `sqlClusterDao` struct

```
type ClusterDao interface { ... }
type sqlClusterDao struct { sessionFactory *db.SessionFactory }
func NewClusterDao(sessionFactory *db.SessionFactory) ClusterDao
```

## Session Access

Always get the GORM session from context — never create direct DB connections:
```
db := d.sessionFactory.New(ctx)
```
This retrieves the transaction-aware session created by `TransactionMiddleware`.

## Error Handling

On any write error, mark the transaction for rollback:
```
db.MarkForRollback(ctx, err)
```
This is critical — without it, the middleware will commit a partially-failed transaction.

## Patterns

- Generation increment is handled by the service layer's `Patch` method via `IncrementGeneration()`; `Save()` persists the result
- Use `clause.Associations` carefully — omit on updates to prevent cascading deletes of related records
- All methods accept `context.Context` as first parameter for transaction propagation
- Return stdlib `error` (not `*errors.ServiceError`) — service layer wraps DAO errors into ServiceErrors

## GenericDao

`generic.go` provides a chainable query builder:
- `GetInstanceDao(ctx, model)` — creates a new DAO instance for a model
- Chain: `.Preload()`, `.OrderBy()`, `.Joins()`, `.Group()`, `.Where()`
- Execute: `.Fetch(offset, limit, &resultList)`, `.Count(model, &total)`

## Related CLAUDE.md Files

- `pkg/services/CLAUDE.md` — Service layer that consumes DAOs
- `pkg/db/CLAUDE.md` — SessionFactory and transaction middleware
