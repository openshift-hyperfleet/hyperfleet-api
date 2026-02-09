# Development Guide

This guide covers the complete development workflow for HyperFleet API, from initial setup to running tests.

## Prerequisites

Before running hyperfleet-api, ensure these prerequisites are installed. See [PREREQUISITES.md](../PREREQUISITES.md) for detailed installation instructions.

- **Go 1.24 or higher**
- **Podman**
- **PostgreSQL 13+**
- **Make**

Verify installations:
```bash
go version      # Should show 1.24+
podman version
make --version
```

## Initial Setup

Set up your local development environment:

```bash
# 1. Generate mocks for testing
make generate-mocks

# 2. Install dependencies
go mod download

# 3. Build the binary
make build

# 4. Initialize secrets
make secrets

# 5. Setup PostgreSQL database
make db/setup

# 6. Run database migrations
./bin/hyperfleet-api migrate

# 7. Start the service (development mode)
make run-no-auth
```

**Important**: Mocks are generated from source interfaces. Run `make generate-mocks` after cloning.

## Pre-commit Hooks (Optional)

This project uses pre-commit hooks for code quality and security checks.

### Setup

```bash
# Install pre-commit
brew install pre-commit  # macOS
# or
pip install pre-commit

# Install hooks
pre-commit install
pre-commit install --hook-type pre-push

# Test
pre-commit run --all-files
```

### For External Contributors

The `.pre-commit-config.yaml` includes `rh-pre-commit` which requires access to Red Hat's internal GitLab. External contributors can skip it:

```bash
# Skip internal hook when committing
SKIP=rh-pre-commit git commit -m "your message"
```

Or comment out the internal hook in `.pre-commit-config.yaml`.

### Update Hooks

```bash
pre-commit autoupdate
pre-commit run --all-files
```

## Running the Service

### Local Development (No Authentication)

```bash
make run-no-auth
```

The service starts on `localhost:8000`:
- REST API: `http://localhost:8000/api/hyperfleet/v1/`
- OpenAPI spec: `http://localhost:8000/api/hyperfleet/v1/openapi`
- Swagger UI: `http://localhost:8000/api/hyperfleet/v1/openapi.html`
- Liveness probe: `http://localhost:8080/healthz`
- Readiness probe: `http://localhost:8080/readyz`
- Metrics: `http://localhost:9090/metrics`

### Testing the API

```bash
# List clusters
curl http://localhost:8000/api/hyperfleet/v1/clusters | jq

# Create a cluster
curl -X POST http://localhost:8000/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "Cluster",
    "name": "prod-cluster-1",
    "spec": {...},
    "labels": {"env": "production"}
  }' | jq
```

### Production Mode (OCM Authentication)

```bash
make run
ocm login --token=${OCM_ACCESS_TOKEN} --url=http://localhost:8000
ocm get /api/hyperfleet/v1/clusters
```

See [Deployment](deployment.md) and [Authentication](authentication.md) for complete configuration options.

## Testing

```bash
# Unit tests
make test

# Integration tests (requires running database)
make test-integration
```

All API endpoints have integration test coverage.

## Build Commands

### Common Commands

```bash
# Generate mocks for testing
make generate-mocks

# Build binary
make build

# Run database migrations
./bin/hyperfleet-api migrate

# Start server (no auth, local CRDs)
make run-no-auth

# Run tests
make test
make test-integration

# Database management
make db/setup      # Create PostgreSQL container
make db/teardown   # Remove PostgreSQL container
make db/login      # Connect to database shell
```

### Build Targets

| Command | Description |
|---------|-------------|
| `make generate-mocks` | Generate mock implementations for testing |
| `make build` | Build hyperfleet-api executable to bin/ |
| `make test` | Run unit tests |
| `make test-integration` | Run integration tests |
| `make run-no-auth` | Start server without authentication (loads CRDs from local files) |
| `make run` | Start server with OCM authentication (loads CRDs from local files) |
| `make db/setup` | Create PostgreSQL container |
| `make db/teardown` | Remove PostgreSQL container |
| `make db/login` | Connect to database shell |

## Development Workflow

### CRD-Driven API

HyperFleet API dynamically generates its OpenAPI specification and routes from Kubernetes Custom Resource Definitions (CRDs). The CRD files are located in `charts/crds/`.

**How it works**:
- At startup, the API loads CRD definitions and generates routes dynamically
- OpenAPI spec is generated at runtime from the loaded CRDs
- No code generation required for API types

**CRD Loading Priority**:
1. If `CRD_PATH` environment variable is set, load from that directory
2. Otherwise, try to load from Kubernetes API
3. If both fail, dynamic routes are disabled (warning logged)

**Environment Variable**:
```bash
# Set CRD_PATH to load CRDs from local files (used by make run/run-no-auth)
CRD_PATH=/path/to/crds ./bin/hyperfleet-api serve

# The Makefile targets set this automatically:
make run-no-auth  # Sets CRD_PATH=$(PWD)/charts/crds
```

### Mock Generation

Mock implementations of service interfaces are used for unit testing. Mocks are generated using `mockgen`.

**When to regenerate mocks**:
- After modifying service interface definitions in `pkg/services/`
- When adding or removing methods from service interfaces
- After initial clone (mocks are not committed to git)

**How it works**:
Service files contain `//go:generate` directives that specify how to generate mocks:
```go
//go:generate mockgen-v0.6.0 -source=cluster.go -package=services -destination=cluster_mock.go
```

**Commands**:
```bash
# Generate mocks
make generate-mocks
```

### Tool Dependency Management (Bingo)

HyperFleet API uses [bingo](https://github.com/bwplotka/bingo) to manage Go tool dependencies with pinned versions.

**Managed tools**:
- `mockgen` - Mock generation for testing
- `golangci-lint` - Code linting
- `gotestsum` - Enhanced test output

**Common operations**:
```bash
# Install all tools
bingo get

# Install a specific tool
bingo get <tool>

# Update a tool to latest version
bingo get <tool>@latest

# List all managed tools
bingo list
```

Tool versions are tracked in `.bingo/*.mod` files and loaded automatically via `include .bingo/Variables.mk` in the Makefile.

### Making Changes

1. **Create a feature branch**:
   ```bash
   git checkout -b feature/my-feature
   ```

2. **Make your changes** to the code

3. **Update CRDs if needed**:
   - Modify CRD files in `charts/crds/`
   - The API will pick up changes on restart

4. **Regenerate mocks if service interfaces changed**:
   ```bash
   make generate-mocks
   ```

5. **Run tests**:
   ```bash
   make test
   make test-integration
   ```

6. **Commit your changes**:
   ```bash
   git add .
   git commit -m "feat: add new feature"
   # Pre-commit hooks will run automatically
   ```

7. **Push and create pull request**:
   ```bash
   git push origin feature/my-feature
   ```

## Troubleshooting

### "undefined: Mock*" or missing mock files

**Problem**: Missing generated mock implementations

**Solution**:
```bash
make generate-mocks
```

### Database Connection Errors

**Problem**: Cannot connect to PostgreSQL

**Solution**:
```bash
# Check if container is running
podman ps | grep postgres

# Restart database
make db/teardown
make db/setup
```

### Test Failures

**Problem**: Integration tests failing

**Solution**:
```bash
# Ensure database is running
make db/setup

# Run migrations
./bin/hyperfleet-api migrate

# Run tests again
make test-integration
```

## Related Documentation

- [Database](database.md) - Database schema and migrations
- [Deployment](deployment.md) - Container and Kubernetes deployment
- [API Resources](api-resources.md) - API endpoints and data models
