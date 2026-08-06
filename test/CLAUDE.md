# Claude Code Guidelines for Tests

## Unit Tests

- Run: `make test`
- Use Gomega assertions: `. "github.com/onsi/gomega"` with `RegisterTestingT(t)`
- Mock generation: `make generate-mocks` — never write mocks manually
- Mocks use `go.uber.org/mock/gomock`

## Integration Tests

- Run: `make test-integration` (sets `TESTCONTAINERS_RYUK_DISABLED=true`)
- Located in `integration/`
- Testcontainers auto-creates isolated PostgreSQL per test suite — no external DB needed
- Setup: `test.RegisterIntegration(t)` returns `(helper, httpClient)`
- HTTP assertions use Resty client

## Test Factories

Reference: `factories/`

- `factories.NewCluster(id)` — creates a Cluster resource via service layer
- `factories.NewClusterList(name, count)` — creates multiple Cluster resources
- `NewClusterWithStatus(f, dbFactory, id, isAvailable, isReconciled)` — Cluster resource with conditions
- `NewClusterWithLabels(f, dbFactory, id, labels)` — Cluster resource with labels
- ID generation: RFC4122 UUID v7 (36-char lowercase format with hyphens, time-ordered with millisecond precision)
- Kubernetes usage: Lowercase UUIDs are DNS-1123 compliant and can be used directly as K8s resource names

## TestMain Setup

`integration/integration_test.go` — `TestMain(m *testing.M)`:
- Schema validation: `TestMain` auto-sets `HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH` to `test/validation-schema.yaml` (permissive, all registered entities) if present, else `openapi/openapi.yaml`
- 45-second timeout safeguard for CI (prevents hung Prow jobs)

## Key Environment Variables

- `TESTCONTAINERS_RYUK_DISABLED=true` — required for testcontainers in CI
- `HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH` — path to OpenAPI schema for spec validation (auto-set by `TestMain` to `test/validation-schema.yaml`)
