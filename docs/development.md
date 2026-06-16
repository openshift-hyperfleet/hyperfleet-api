# Setup

```bash
git clone https://github.com/openshift-hyperfleet/hyperfleet-adapter.git
cd hyperfleet-adapter
make mod-tidy
make build        # → bin/hyperfleet-adapter
```

# Verification

```bash
make fmt              # Format code + imports (golangci-lint fmt with gci)
make lint             # golangci-lint (config: .golangci.yml)
make test             # Unit tests with race detection
make test-integration # Integration tests via testcontainers (needs Docker/Podman)
make test-all         # All of the above + make test-helm
```

# Make Targets

| Target | Description |
|--------|-------------|
| `make build` | Build binary |
| `make test` | Run unit tests |
| `make test-integration` | Run integration tests with envtest (unprivileged) |
| `make test-integration-k3s` | Run integration tests with K3s (faster, may need privileges) |
| `make test-all` | Run all tests (unit + integration + helm) |
| `make test-coverage` | Generate test coverage report |
| `make lint` | Run golangci-lint |
| `make fmt` | Format code |
| `make image` | Build container image |
| `make image-push` | Build and push container image |
| `make image-dev` | Build and push to personal Quay registry |
| `make mod-tidy` | Tidy Go module dependencies |
| `make clean` | Clean build artifacts |

Use `make help` to see all available targets.

# Integration Tests

<details>
<summary>Setup and run integration tests</summary>

Integration tests use **Testcontainers** with **envtest** — works in any CI/CD platform without privileged containers.

## Prerequisites

- **Docker or Podman** must be running (both supported)
- The Makefile automatically detects and configures your container runtime
- **Podman users**: Corporate proxy settings are auto-detected from Podman machine. Define and export the environment variable : 
```
DOCKER_HOST=unix://$XDG_RUNTIME_DIR/podman/podman.sock
```

## Run

```bash
make test-integration       # envtest (default, unprivileged)
make test-integration-k3s   # K3s (faster, may need privileges)
```

The first run downloads `golang:alpine` and installs envtest (~20-30 seconds). Subsequent runs are cached.

</details>

# Tool Dependencies (Bingo)

Build tools are pinned via [bingo](https://github.com/bwplotka/bingo) in `.bingo/` manifests:

```bash
bingo get           # Install all tools
bingo list          # List managed tools
```

# Container Image

```bash
# Build container image
make image

# Build with custom tag
make image IMAGE_TAG=v1.0.0

# Build and push to default registry
make image-push

# Build and push to personal Quay registry (for development)
QUAY_USER=myuser make image-dev
```

Default image: `quay.io/openshift-hyperfleet/hyperfleet-adapter:latest`

The container build embeds version metadata (version, git commit, build date) into the binary.

---

# Dry-Run Mode

Dry-run mode simulates the full execution pipeline locally without connecting to any real infrastructure. It processes a single CloudEvent from a JSON file and produces a detailed trace.

```bash
hyperfleet-adapter serve \
  --config ./adapter-config.yaml \
  --task-config ./task-config.yaml \
  --dry-run-event ./event.json
```

<details>
<summary>Dry-run flags and input files</summary>

## Flags

| Flag | Required | Description |
|------|----------|-------------|
| `--dry-run-event <path>` | Yes | Path to a CloudEvent JSON file to process |
| `--dry-run-api-responses <path>` | No | Path to mock API responses JSON file (defaults to 200 OK for all requests) |
| `--dry-run-discovery <path>` | No | Path to mock discovery overrides JSON file (simulates server-populated fields) |
| `--dry-run-verbose` | No | Show rendered manifests and API request/response bodies in output |
| `--dry-run-output <format>` | No | Output format: `text` (default) or `json` |

## CloudEvent file (`--dry-run-event`)

```json
{
  "specversion": "1.0",
  "id": "abc123",
  "type": "io.hyperfleet.cluster.updated",
  "source": "/api/hyperfleet/v1/clusters/abc123",
  "data": {
    "id": "abc123",
    "kind": "Cluster",
    "href": "/api/hyperfleet/v1/clusters/abc123",
    "generation": 5
  }
}
```

## Mock API responses (`--dry-run-api-responses`)

Requests are matched by HTTP method and URL regex. When multiple responses are defined for a match, they are returned sequentially (the last one repeats):

```json
{
  "responses": [
    {
      "match": { "method": "GET", "urlPattern": "/api/hyperfleet/v1/clusters/.*" },
      "responses": [
        { "statusCode": 200, "body": { "id": "abc123", "name": "my-cluster", "generation": 5 } }
      ]
    },
    {
      "match": { "method": "PUT", "urlPattern": "/api/hyperfleet/v1/clusters/.*/statuses" },
      "responses": [{ "statusCode": 200, "body": {} }]
    }
  ]
}
```

If no file is provided, all API requests return 200 OK.

## Discovery overrides (`--dry-run-discovery`)

Maps rendered resource names to complete Kubernetes objects, simulating server-populated fields (status, uid, resourceVersion):

```json
{
  "abc123": {
    "apiVersion": "v1",
    "kind": "Namespace",
    "metadata": { "name": "abc123", "uid": "a1b2c3d4", "resourceVersion": "100" },
    "status": { "phase": "Active" }
  }
}
```

## Output

The trace walks through each phase: parameter extraction, preconditions (with API calls), resources (CREATE/UPDATE/RECREATE/DELETE), discovery results, and post-actions. Use `--dry-run-verbose` for full manifests and API bodies, or `--dry-run-output json` for machine-readable output.

Example input files are available in `test/testdata/dryrun/`.

</details>
