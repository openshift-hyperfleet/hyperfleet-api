# AGENTS.md

This file provides guidance to AI coding agents working with the HyperFleet API repository.

For Claude Code users: also see `CLAUDE.md` (auto-loaded) and `.claude/rules/` (loaded per file context).

## Commands

### Setup (fresh clone)
```
make generate-all     # REQUIRED FIRST — generated code not in git
go mod download
make secrets          # Initialize secrets/ with defaults
make db/setup         # Start local PostgreSQL container
make build            # Build binary (CGO_ENABLED=1 GOEXPERIMENT=boringcrypto)
./bin/hyperfleet-api migrate
make run-no-auth      # Start server without auth
```

### Build & Run
```
make build            # Build hyperfleet-api binary to bin/
make install          # Build and install to GOPATH/bin
make run              # Build, migrate, and run with auth
make run-no-auth      # Build, migrate, and run without auth
```

### Code Generation
```
make generate         # Regenerate Go models from openapi/openapi.yaml (oapi-codegen)
make generate-mocks   # Regenerate mock implementations (go generate)
make generate-all     # Both of the above
```

### Verification
```
make verify           # go vet + gofmt check
make lint             # golangci-lint
make test             # Unit tests (OCM_ENV=unit_testing)
make test-integration # Integration tests with testcontainers (OCM_ENV=integration_testing)
make test-helm        # Helm chart lint + template validation
make verify-all       # verify + lint + test — fast, no DB needed
make test-all         # lint + test + test-integration + test-helm — full suite
```

### Database
```
make db/setup         # Start PostgreSQL container
make db/login         # Connect to local PostgreSQL
make db/teardown      # Stop and remove container
```

Run `make help` for the complete target list.

## Testing

**Unit tests**: `make test` — sets `OCM_ENV=unit_testing`, runs `./pkg/...` and `./cmd/...`

**Integration tests**: `make test-integration` — sets `OCM_ENV=integration_testing` and `TESTCONTAINERS_RYUK_DISABLED=true`. Testcontainers auto-creates isolated PostgreSQL instances. Located in `test/integration/`.

**Helm tests**: `make test-helm` — lints and renders templates with multiple value combinations.

**Mock generation**: `make generate-mocks` — uses `go generate` directives with `go.uber.org/mock/gomock`. Never write mocks manually.

**Test factories**: `test/factories/` — create resources via the service layer, not directly in DB. Use `NewCluster()`, `NewClusterWithStatus()`, `NewClusterWithLabels()`.

**Integration test setup**: `test.RegisterIntegration(t)` returns `(helper, client)`. Uses Gomega assertions and Resty HTTP client.

**Environment variables for tests**:
- `OCM_ENV` — selects config: `unit_testing`, `integration_testing`, `development`
- `TESTCONTAINERS_RYUK_DISABLED=true` — required in CI
- `HYPERFLEET_CLUSTER_ADAPTERS` / `HYPERFLEET_NODEPOOL_ADAPTERS` — adapter lists (defaults set in TestMain)

## Project Structure

```
cmd/hyperfleet-api/           # Entry point + subcommands (serve, migrate)
  environments/               # Environment configs (development, unit_testing, etc.)
pkg/
  api/openapi/                # GENERATED — models + embedded spec (never edit)
  handlers/                   # HTTP handlers using handlerConfig pipeline
    framework.go              # handle/handleGet/handleList/handleDelete pipeline
  services/                   # Service interfaces + sqlXxxService implementations
  dao/                        # DAO interfaces + sqlXxxDao implementations
  db/                         # SessionFactory, transaction middleware, migrations
  errors/                     # ServiceError type, RFC 9457 Problem Details
  logger/                     # Structured logging (slog-based)
  config/                     # Configuration management
plugins/                      # Plugin registration (init-based)
  clusters/plugin.go          # RegisterService + RegisterRoutes + RegisterPath + RegisterKind
  nodepools/plugin.go
  generic/plugin.go
openapi/
  openapi.yaml                # SOURCE spec (TypeSpec output, 32KB, uses $ref)
  oapi-codegen.yaml           # Code generation config
test/
  integration/                # Integration tests (testcontainers)
  factories/                  # Test data factories
charts/                       # Helm chart for Kubernetes deployment
```

**Generated code** (not in git — run `make generate-all`):
- `pkg/api/openapi/` — Go models + embedded spec
- `*_mock.go` — Mock implementations

## Code Style

### Imports
Order: stdlib → external → internal (`github.com/openshift-hyperfleet/hyperfleet-api/...`)

### Errors
Use constructor functions from `pkg/errors/errors.go`: `NotFound()`, `Validation()`, `GeneralError()`, `Conflict()`, `ValidationWithDetails()`. Error codes: `HYPERFLEET-CAT-NUM` format. All service methods return `*errors.ServiceError`.

### Logging
Use `pkg/logger/` — `logger.Info(ctx, "msg")`, `logger.With(ctx, "key", val).Error("msg")`. Never use `fmt.Println` or `log.Print`.

### Handlers
Use `handlerConfig` pipeline from `pkg/handlers/framework.go`:
- `handle(w, r, cfg, status)` — unmarshal → validate → action → respond
- `handleGet/handleList/handleDelete` — no-body variants
- Validation: `func() *errors.ServiceError`; Action: `func() (interface{}, *errors.ServiceError)`

### Services
Interface + `sql*Service` struct. Constructor injection of DAOs. Return `*errors.ServiceError`. Add `//go:generate mockgen` directive for mocks.

### DAOs
Interface + `sql*Dao` struct. Get session via `sessionFactory.New(ctx)`. Call `db.MarkForRollback(ctx, err)` on write errors. Return stdlib `error`.

### Plugins
Register via `init()`: `registry.RegisterService()`, `server.RegisterRoutes()`, `presenters.RegisterPath()`, `presenters.RegisterKind()`. See `plugins/clusters/plugin.go`.

## Git Workflow

### Commit Format
```
HYPERFLEET-### - type: description
```
Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`

Add co-author line for AI-assisted commits:
```
Co-Authored-By: Claude <noreply@anthropic.com>
```

### Pre-commit Hooks
Install: `pre-commit install && pre-commit install --hook-type pre-push`

Hooks:
- `rh-pre-commit` — Red Hat security compliance (requires internal GitLab access; skip with `SKIP=rh-pre-commit`)
- `validate-agents-md` — validates AGENTS.md exists (runs on push)
- `ai-attribution-reminder` — reminds about AI co-author attribution

### Branching
Create feature branches from `main`. PRs target `main`.

## Boundaries

- **Never edit** `pkg/api/openapi/` or `*_mock.go` — regenerate with `make generate-all`
- **Never set** `status.phase` manually — calculated from adapter conditions
- **Never create** direct DB connections — use `SessionFactory.New(ctx)` for transaction participation
- **FIPS required**: build with `CGO_ENABLED=1 GOEXPERIMENT=boringcrypto`
- **Spec source of truth**: `openapi/openapi.yaml` (TypeSpec output); generated spec at `pkg/api/openapi/api/openapi.yaml` is never edited
- **TypeSpec** definitions live in a separate `hyperfleet-api-spec` repository
- **Tool versions** managed by Bingo (`.bingo/`) — don't manually install oapi-codegen or golangci-lint
