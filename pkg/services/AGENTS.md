# Claude Code Guidelines for Services

## Interface Pattern

Every service defines an interface + concrete `sql*Service` implementation.

Reference: `resource.go` — `ResourceService` interface, `sqlResourceService` struct

```go
type ResourceService interface { ... }
type sqlResourceService struct { ... }
func NewResourceService(resourceDao, resourceLabelDao, adapterStatusDao, resourceConditionDao, generic) ResourceService
```

## Conventions

- All methods return `*errors.ServiceError` (from `pkg/errors/`), never stdlib `error`
- Constructor injection: DAOs and config passed to `New*Service()` constructor
- Mutation services receive `db.TxRunner` and open one transaction with `Do` at their public boundary. Every dependent DAO call must use the callback context. Reconciliation outcomes are emitted only after `Do` succeeds.
- Compile-time interface check: `var _ ResourceService = &sqlResourceService{}`
- Mock generation: add `//go:generate mockgen` directive, then `make generate-mocks`

## Status Aggregation

`UpdateStatusFromAdapters()` in `resource.go` synthesizes two top-level conditions:
- **Available**: True if all required adapters report `Available=True` (any generation)
- **Ready**: True if all adapters report `Available=True` AND `observed_generation` matches current generation

`ProcessAdapterStatus()` validates mandatory conditions (`Available`, `Applied`, `Health`) before persisting. Rejects `Available=Unknown` on subsequent reports (only allowed on first report).

## GenericService

`generic.go` provides `List()` with pagination, search, and ordering.
- `ListArguments` has Page, Size, Search, Order, Fields, Preloads
- Default ordering: `created_time desc`

## Related AGENTS.md Files

- `pkg/handlers/AGENTS.md` — Handler pipeline that calls services
- `pkg/dao/AGENTS.md` — DAO layer that services depend on
- `pkg/errors/AGENTS.md` — ServiceError type returned by all methods
