# HyperFleet Adapter

Event-driven Kubernetes resource manager. Consumes CloudEvents from a message broker, executes configured actions (K8s resource apply, HyperFleet API calls, Maestro ManifestWork), and reports status back.

Go 1.25.0 · Cobra CLI · Viper config · golangci-lint (bingo-managed) · Tekton CI (Konflux)

## Setup (fresh clone)

```bash
make install-hooks    # Install pre-commit hooks (secret scanning, linting, etc.)
make build            # Build binary → bin/hyperfleet-adapter
```

## Verification Checklist

```bash
make fmt              # Format code + imports (golangci-lint fmt with gci)
make lint             # golangci-lint (config: .golangci.yml)
make test             # Unit tests with race detection (excludes test/ dir)
make test-integration # Integration tests via testcontainers (needs Docker/Podman)
make build            # Build binary → bin/hyperfleet-adapter
```

`make test-all` runs `make lint`, `make test`, `make test-integration`, and `make test-helm`.

### Pre-commit Hooks
Install: `make install-hooks`

Hooks:
- `leaktk.git.pre-commit` — secret scanning (open-source, no VPN required)
- `hyperfleet-commitlint` — validates commit message format (commit-msg stage)
- `hyperfleet-gofmt` — Go code formatting
- `hyperfleet-golangci-lint` — linting
- `hyperfleet-go-vet` — Go vet checks
- `trailing-whitespace` — removes trailing whitespace
- `end-of-file-fixer` — ensures files end with newline
- `check-added-large-files` — prevents large files from being committed

## CLI

Subcommands: `adapter serve`, `adapter config-dump`, `adapter version`. Config paths via `-c`/`HYPERFLEET_ADAPTER_CONFIG` and `-t`/`HYPERFLEET_TASK_CONFIG`. All flags have env var equivalents — run `adapter serve --help`.

Dry-run mode: `adapter serve --dry-run-event event.json` processes a single event with mock clients, no broker or cluster needed.

## Two Config Files

Adapter loads two configs merged at startup: deployment config (`adapter-config.yaml` — infra, clients, logging) and task config (`adapter-task-config.yaml` — params, preconditions, resources, post-actions). Override rules differ — see Gotchas. Templates in `configs/`.

## Source of Truth

| Topic | Location |
|-------|----------|
| Configuration reference | `docs/configuration.md` |
| Adapter authoring guide | `docs/adapter-authoring-guide.md` |
| Metrics & Prometheus queries | `docs/metrics.md` |
| Alerts | `docs/alerts.md` |
| Runbook | `docs/runbook.md` |
| Helm chart | `charts/` |
| CI pipelines | `.tekton/` (Konflux/Tekton PipelineRuns) |

## Code Conventions

@docs/conventions/logging.md
@docs/conventions/cel.md

### Error Handling

`pkg/errors` provides ServiceError constructors for API-style errors with numeric codes and HTTP status:

```go
errors.NotFound("cluster %s not found", clusterID)      // → *ServiceError
errors.KubernetesError("failed to get resource: %v", err)
```

IMPORTANT: These return `*ServiceError`, not `error`. Use `.AsError()` to convert.

## Boundaries

- Every CLI flag must have a corresponding env var (Viper convention)

## Gotchas

- IMPORTANT: **Two config files, different override rules.** Deployment config supports env/flag overrides via Viper. Task config is pure YAML — env vars do nothing there. Mixing them up wastes debugging time.
- IMPORTANT: **Naming conventions differ by layer.** Config YAML: `snake_case` (`subscription_id`). Go code: `CamelCase` (`SubscriptionID`). Helm values: `camelCase` (`subscriptionId`). Wrong casing silently drops values.
- **Tracing default mismatch.** Binary defaults to tracing ON. Helm chart defaults to OFF. Local `adapter serve` will attempt OTLP export unless you set `HYPERFLEET_TRACING_ENABLED=false`.
- **Integration tests build a container on first run.** `make test-integration` calls `make image-integration-test` if `INTEGRATION_ENVTEST_IMAGE` is unset. First run takes minutes.

## Non-Obvious Packages

- `internal/executor/` — event execution pipeline (params → preconditions → resources → post-actions)
- `internal/transportclient/` — unified apply interface abstracting K8s direct and Maestro ManifestWork

## Links

- [Architecture Docs](https://github.com/openshift-hyperfleet/architecture)
- [HyperFleet API Spec](https://github.com/openshift-hyperfleet/hyperfleet-api-spec)
- [Broker Library](https://github.com/openshift-hyperfleet/hyperfleet-broker)
