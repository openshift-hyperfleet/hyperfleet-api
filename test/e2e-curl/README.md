# Status Aggregation — E2E Curl Tests

Black-box tests for the cluster status aggregation logic. Each test drives the
live API with `curl`, then asserts on the `Available` and `Ready` synthetic
conditions returned by `GET /clusters/:id`.

## Prerequisites

- `curl`, `jq`
- A running HyperFleet API server with auth disabled and two required adapters
  configured:

```bash
HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER='["adapter1","adapter2"]' make run-no-auth
```

Override the adapter names to match your server configuration:

```bash
ADAPTER1=dns ADAPTER2=validation ./run_all.sh
```

## Running

```bash
# All 12 tests
./run_all.sh

# Selected tests by prefix
./run_all.sh 03 07 09
```

Each test creates its own cluster (name suffixed with a random hex, e.g.
`tc07-3a1f`), so tests are fully isolated and can run against a shared server.

## Test Catalogue

| # | Directory | Scenario |
|---|-----------|----------|
| 01 | `01-initial-state` | Cluster just created — no adapter reports yet; both conditions False |
| 02 | `02-partial-adapters` | One required adapter True, the other missing — Available stays False |
| 03 | `03-all-adapters-ready` | All adapters True at gen=1 — Available=True, Ready=True |
| 04 | `04-generation-bump` | Cluster spec changes (gen→2) — Available frozen, Ready resets to False |
| 05 | `05-mixed-generations` | One adapter at gen1, one at gen2 — `all_at_X=false`, Available preserved |
| 06 | `06-stale-report` | Adapter reports an older `observed_generation` — discarded (204) |
| 07 | `07-all-adapters-new-gen` | Both adapters converge at gen2 — Available.ObsGen advances, Ready=True |
| 08 | `08-adapter-goes-false` | One required adapter flips to False — both Available and Ready become False |
| 09 | `09-stable-true` | Heartbeat re-reports (same status, same gen) — both LUTs refresh to new min(LRT) |
| 10 | `10-stable-false` | Heartbeat False re-reports — Available.LUT preserved (no change), Ready.LUT refreshes |
| 11 | `11-unknown-subsequent` | `Available=Unknown` on a subsequent report — discarded (204), state unchanged |
| 12 | `12-unknown-first` | `Available=Unknown` on a first report — discarded (204), nothing stored |

## Structure

```
e2e-curl/
  common.sh          # Shared helpers: API wrappers, assertions, logging
  run_all.sh         # Suite runner with per-test pass/fail tracking
  01-initial-state/
    test.sh
  ...
```

### `common.sh` helpers

| Helper | Purpose |
|--------|---------|
| `create_cluster NAME` | `POST /clusters` |
| `patch_cluster ID SPEC` | `PATCH /clusters/:id` (bumps generation) |
| `get_cluster ID` | `GET /clusters/:id` |
| `post_adapter_status ID ADAPTER GEN STATUS` | `POST /clusters/:id/statuses` |
| `condition_field JSON TYPE FIELD` | Extract a field from a named condition |
| `assert_eq LABEL EXPECTED ACTUAL` | Equality assertion |
| `assert_changed LABEL BEFORE AFTER` | Asserts a value changed |
| `assert_nonempty LABEL VALUE` | Asserts a value is non-null/non-empty |
| `show_state LABEL JSON` | One-line `Available=?@genN  Ready=?@genN` summary |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `HYPERFLEET_URL` | `http://localhost:8000` | Base URL of the API server |
| `ADAPTER1` | `adapter1` | Name of the first required adapter |
| `ADAPTER2` | `adapter2` | Name of the second required adapter |
