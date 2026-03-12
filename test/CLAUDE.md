# Claude Code Guidelines for Tests

## Unit Tests

- Run: `make test` (sets `OCM_ENV=unit_testing` automatically)
- Use Gomega assertions: `. "github.com/onsi/gomega"` with `RegisterTestingT(t)`
- Mock generation: `make generate-mocks` — never write mocks manually
- Mocks use `go.uber.org/mock/gomock`

## Integration Tests

- Run: `make test-integration` (sets `OCM_ENV=integration_testing` and `TESTCONTAINERS_RYUK_DISABLED=true`)
- Located in `integration/`
- Testcontainers auto-creates isolated PostgreSQL per test suite — no external DB needed
- Setup: `test.RegisterIntegration(t)` returns `(helper, httpClient)`
- HTTP assertions use Resty client

## Test Factories

Reference: `factories/`

- `factories.NewCluster(id)` — creates cluster via service layer
- `factories.NewClusterList(name, count)` — creates multiple clusters
- `NewClusterWithStatus(f, dbFactory, id, isAvailable, isReady)` — cluster with conditions
- `NewClusterWithLabels(f, dbFactory, id, labels)` — cluster with labels
- ID generation: KSUID + lowercase Base32 (K8s DNS-1123 compliant)

## TestMain Setup

`integration/integration_test.go` — `TestMain(m *testing.M)`:
- Sets default adapter env vars (`HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER`, `HYPERFLEET_ADAPTERS_REQUIRED_NODEPOOL`)
- Sets `HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH` via `runtime.Caller`
- 45-second timeout safeguard for CI (prevents hung Prow jobs)

## Key Environment Variables

- `OCM_ENV` — selects config environment: `unit_testing`, `integration_testing`, `development`
- `TESTCONTAINERS_RYUK_DISABLED=true` — required for testcontainers in CI
- `HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER` / `HYPERFLEET_ADAPTERS_REQUIRED_NODEPOOL` — adapter lists for tests
- `HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH` — OpenAPI schema path for spec validation
