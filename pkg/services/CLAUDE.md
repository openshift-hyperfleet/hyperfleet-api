# Claude Code Guidelines for Services

## Interface Pattern

Every service defines an interface + concrete `sql*Service` implementation.

Reference: `cluster.go` — `ClusterService` interface, `sqlClusterService` struct

```
type ClusterService interface { ... }
type sqlClusterService struct { ... }
func NewClusterService(dao, adapterStatusDao, config) ClusterService
```

## Conventions

- All methods return `*errors.ServiceError` (from `pkg/errors/`), never stdlib `error`
- Constructor injection: DAOs and config passed to `New*Service()` constructor
- Compile-time interface check: `var _ ClusterService = &sqlClusterService{}`
- Mock generation: add `//go:generate mockgen` directive, then `make generate-mocks`

## Status Aggregation

`UpdateClusterStatusFromAdapters()` in `cluster.go` synthesizes two top-level conditions:

- **Available**: True when all required adapters report `Available=True` at the same `observed_generation` (not necessarily the current resource generation). When adapters are at different generations, Available preserves its previous value (last-known-good semantics). `ObservedGeneration` = the common adapter generation when consistent; preserved from existing state otherwise.
- **Ready**: True if all adapters report `Available=True` AND `observed_generation` matches current generation

Ready's `LastUpdatedTime` is computed in `status_aggregation.computeReadyLastUpdated`: when Ready=False it is the minimum of `LastReportTime` across all required adapters (falls back to `now` if any required adapter has no stored status yet); when Ready=True it is the minimum of `LastReportTime` across required adapters that have Available=True at the current generation. True→False transitions override this with the triggering adapter's `observedTime`.

`ProcessAdapterStatus()` validates mandatory conditions (`Available`, `Applied`, `Health`) before persisting. Rejects `Available=Unknown` on subsequent reports (only allowed on first report).

## GenericService

`generic.go` provides `List()` with pagination, search, and ordering.

- `ListArguments` has Page, Size, Search, OrderBy, Fields, Preloads
- Search validation: `SearchDisallowedFields` map blocks searching certain fields per resource type
- Default ordering: `created_time desc`

## Related CLAUDE.md Files

- `pkg/handlers/CLAUDE.md` — Handler pipeline that calls services
- `pkg/dao/CLAUDE.md` — DAO layer that services depend on
- `pkg/errors/CLAUDE.md` — ServiceError type returned by all methods
