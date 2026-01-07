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
# 1. Generate OpenAPI code and mocks
make generate-all

# 2. Install dependencies
go mod download

# 3. Build the binary
make build

# 4. Setup PostgreSQL database
make db/setup

# 5. Run database migrations
./bin/hyperfleet-api migrate

# 6. Verify database schema
make db/login
\dt
```

**Important**: Generated code is not tracked in git. You must run `make generate-all` after cloning to generate both OpenAPI models and mocks.

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
- Health check: `http://localhost:8083/healthcheck`
- Metrics: `http://localhost:8080/metrics`

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
# Generate OpenAPI client code
make generate

# Generate mocks for testing
make generate-mocks

# Generate both OpenAPI and mocks
make generate-all

# Build binary
make build

# Run database migrations
./bin/hyperfleet-api migrate

# Start server (no auth)
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
| `make generate` | Generate Go models from OpenAPI spec |
| `make generate-mocks` | Generate mock implementations for testing |
| `make generate-all` | Generate both OpenAPI models and mocks |
| `make build` | Build hyperfleet-api executable to bin/ |
| `make test` | Run unit tests |
| `make test-integration` | Run integration tests |
| `make run-no-auth` | Start server without authentication |
| `make run` | Start server with OCM authentication |
| `make db/setup` | Create PostgreSQL container |
| `make db/teardown` | Remove PostgreSQL container |
| `make db/login` | Connect to database shell |

## Development Workflow

### Code Generation

HyperFleet API generates Go models from OpenAPI specifications using `openapi-generator-cli`.

**Workflow**:
```text
openapi/openapi.yaml
    ↓
make generate (podman + openapi-generator-cli)
    ↓
pkg/api/openapi/model_*.go (Go structs)
pkg/api/openapi/api/openapi.yaml (embedded spec)
```

**Generated artifacts**:
- Go model structs with JSON tags (`model_*.go`)
- Fully resolved OpenAPI specification (embedded in binary)

**Important**:
- Generated files are NOT tracked in git
- Must run `make generate` after cloning
- Must run after OpenAPI spec updates

**OpenAPI spec source**:
The `openapi/openapi.yaml` is maintained in the [hyperfleet-api-spec](https://github.com/openshift-hyperfleet/hyperfleet-api-spec) repository using TypeSpec. When the spec changes, the compiled YAML is copied here. Developers working on hyperfleet-api only need to run `make generate` - no TypeSpec knowledge required.

**Commands**:
```bash
# Generate Go models from OpenAPI spec
make generate

# Generate both OpenAPI models and mocks
make generate-all
```

**Troubleshooting**:
```bash
# If "pkg/api/openapi not found"
make generate
go mod download

# If generator container fails
podman info  # Check podman is running
make generate
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
# Generate mocks only
make generate-mocks

# Generate OpenAPI models and mocks together
make generate-all
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

3. **Update OpenAPI spec if needed**:
   - Make changes in the [hyperfleet-api-spec](https://github.com/openshift-hyperfleet/hyperfleet-api-spec) repository
   - Copy updated `openapi.yaml` to this repository
   - Run `make generate` to regenerate Go models

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

### "pkg/api/openapi not found"

**Problem**: Missing generated OpenAPI code

**Solution**:
```bash
make generate
go mod download
```

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
