# CLAUDE.md

## Project Identity

HyperFleet API is a **stateless REST API** serving as the pure CRUD data layer for HyperFleet cluster lifecycle management. It persists clusters, node pools, and adapter statuses to PostgreSQL — no business logic, no events. Sentinel handles orchestration; adapters execute and report back.

- **Language**: Go 1.24+ with FIPS crypto (`CGO_ENABLED=1 GOEXPERIMENT=boringcrypto`)
- **Database**: PostgreSQL 14.2 with GORM ORM
- **API Spec**: TypeSpec → OpenAPI 3.0.3 → oapi-codegen → Go models
- **Architecture**: Plugin-based route registration, transaction-per-request middleware

## Critical First Steps

**Generated code is not checked into git.** Before building, testing, or even running `go mod download`:

```
make generate-all    # Generates OpenAPI types + mock implementations
```

Setup sequence for a fresh clone:
1. `make generate-all` — generate OpenAPI models and mocks
2. `go mod download` — fetch dependencies
3. `make secrets` — initialize secrets/ with defaults
4. `make db/setup` — start local PostgreSQL container
5. `make build` — build binary (uses `CGO_ENABLED=1 GOEXPERIMENT=boringcrypto`)
6. `./bin/hyperfleet-api migrate` — apply database migrations
7. `make run-no-auth` — start server without authentication

Tool management uses [Bingo](https://github.com/bwplotka/bingo) — tool versions are pinned in `.bingo/`.

## Verification Commands

| Command | What it does | Requires DB? |
|---|---|---|
| `make verify` | go vet + gofmt check | No |
| `make lint` | golangci-lint | No |
| `make test` | Unit tests (`OCM_ENV=unit_testing`) | No |
| `make test-integration` | Integration tests (testcontainers) | No (auto-creates) |
| `make test-helm` | Helm chart lint + template validation | No |
| `make verify-all` | verify + lint + test (single command) | No |
| `make test-all` | lint + test + test-integration + test-helm | Auto-creates |

Use `make verify-all` for fast feedback. Use `make test-all` for full validation.

## Code Conventions

### Commits
Format: `HYPERFLEET-### - type: description` (e.g., `HYPERFLEET-123 - fix: handle nil pointer in status aggregation`)
Co-author line: `Co-Authored-By: Claude <noreply@anthropic.com>`

### Import Ordering
1. Standard library
2. External packages
3. Internal packages (`github.com/openshift-hyperfleet/hyperfleet-api/...`)

### Error Handling
All service methods return `*errors.ServiceError` (not stdlib error). Use constructor functions:
- Reference: `pkg/errors/errors.go` — `NotFound()`, `Validation()`, `GeneralError()`, `Conflict()`, `ValidationWithDetails()`
- Error codes follow `HYPERFLEET-CAT-NUM` format (e.g., `HYPERFLEET-NTF-001`)
- Errors convert to RFC 9457 Problem Details via `AsProblemDetails()`

### Logging
Use the structured logging API — never `fmt.Println` or `log.Print`:
- Reference: `pkg/logger/logger.go`
- `logger.Info(ctx, "message")`, `logger.Error(ctx, "message")`
- Chainable: `logger.With(ctx, "key", value).WithError(err).Error("failed")`

### Handler Pipeline
Handlers use the `handlerConfig` pipeline — Reference: `pkg/handlers/framework.go`
- `handle()` — full pipeline: unmarshal body → validate → action → respond
- `handleGet()` / `handleList()` / `handleDelete()` — no body variants
- Validation functions return `*errors.ServiceError`; action functions return `(interface{}, *errors.ServiceError)`

### Service Pattern
Interface + `sql*Service` implementation with constructor injection:
- Reference: `pkg/services/cluster.go` — `ClusterService` interface, `sqlClusterService` struct
- Constructor: `NewClusterService(dao, adapterStatusDao, config) ClusterService`
- Generate mocks: `make generate-mocks` (uses `go generate` directives)

### DAO Pattern
Interface + `sql*Dao` implementation using SessionFactory:
- Reference: `pkg/dao/cluster.go` — `ClusterDao` interface, `sqlClusterDao` struct
- Get session: `db.New(ctx)` — extracts transaction from request context
- On write errors: call `db.MarkForRollback(ctx, err)`

### Plugin Registration
Each resource registers via `init()` function:
- Reference: `plugins/clusters/plugin.go`
- `registry.RegisterService()`, `server.RegisterRoutes()`, `presenters.RegisterPath()`, `presenters.RegisterKind()`

### Test Patterns
- Gomega assertions with `RegisterTestingT(t)`
- Test factories: `test/factories/` — create resources via service layer
- Integration tests: `test/integration/` — use `test.RegisterIntegration(t)` for setup
- Testcontainers for PostgreSQL — auto-creates isolated DB per test suite

## Architecture Quick Reference

**Request flow**: Router → Middleware (logging, auth, transaction) → Handler → Service → DAO → GORM → PostgreSQL

- Transaction middleware creates GORM transactions for **write requests only** (POST/PUT/PATCH/DELETE): `pkg/db/transaction_middleware.go`
- Read requests (GET) skip transaction creation for performance
- OpenAPI source spec: `openapi/openapi.yaml` (TypeSpec-generated, uses `$ref`)
- Generated code: `pkg/api/openapi/` (models + embedded spec) — **never edit**
- Codegen config: `openapi/oapi-codegen.yaml` — uses oapi-codegen (not openapi-generator-cli)
- Status aggregation: Service layer synthesizes `Available` and `Ready` conditions from adapter reports
- Plugin-based: each resource type registers routes/services in `plugins/*/plugin.go`

Two `openapi.yaml` files exist:
- `openapi/openapi.yaml` — source (32KB, has `$ref`)
- `pkg/api/openapi/api/openapi.yaml` — generated (44KB, fully resolved, embedded in binary)

## Boundaries

- **Never edit** files in `pkg/api/openapi/` — they are generated by `make generate`
- **Never edit** `*_mock.go` files — regenerate with `make generate-mocks`
- **Never set** `status.phase` manually — it is calculated from adapter conditions
- **Never create** direct DB connections — use `SessionFactory.New(ctx)` for transaction participation
- **FIPS required**: always build with `CGO_ENABLED=1 GOEXPERIMENT=boringcrypto`
- **Spec source of truth**: `openapi/openapi.yaml` (TypeSpec output); don't modify generated spec

## Related CLAUDE.md Files

Subdirectories contain context-specific guidance that loads when you work in those areas:

- `pkg/handlers/CLAUDE.md` — Handler pipeline and handlerConfig patterns
- `pkg/services/CLAUDE.md` — Service interface and status aggregation patterns
- `pkg/dao/CLAUDE.md` — DAO interface, session access, and rollback patterns
- `pkg/db/CLAUDE.md` — SessionFactory and transaction middleware
- `pkg/errors/CLAUDE.md` — Error constructors, codes, and RFC 9457 details
- `plugins/CLAUDE.md` — Plugin registration (init-based)
- `test/CLAUDE.md` — Test conventions, factories, and environment variables
- `charts/CLAUDE.md` — Helm chart testing and configuration
- `openapi/CLAUDE.md` — OpenAPI spec, code generation, and oapi-codegen config
