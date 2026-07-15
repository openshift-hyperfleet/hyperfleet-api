# Development Guide

This guide covers the complete development workflow for HyperFleet API, from initial setup to running tests.

## Prerequisites

Before running hyperfleet-api, ensure these prerequisites are installed. See [PREREQUISITES.md](../PREREQUISITES.md) for detailed installation instructions.

- **Go 1.26 or higher**
- **Podman**
- **PostgreSQL 14+**
- **Make**
- **pre-commit**

Verify installations:

```bash
go version          # Should show 1.26+
podman version
make --version
pre-commit --version
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

# 4. Setup PostgreSQL database (see Database Setup below)
make db/setup

# 5. Run database migrations
make db/migrate

# 6. Install git hooks
make install-hooks

# 7. Verify database schema
make db/login
\dt
```

**Important**: Generated code is not tracked in git. You must run `make generate-all` after cloning to generate both OpenAPI models and mocks.

## Configuration

The application loads configuration in this priority order: **CLI flags > environment variables > config file > defaults**.

**Config file:** Copy the example and adjust as needed:

```bash
cp configs/config.yaml.example configs/config.yaml
```

The loader searches for a config file in this order:

1. `--config` flag (explicit path)
2. `HYPERFLEET_CONFIG` environment variable
3. `/etc/hyperfleet/config.yaml` (production default)
4. `./configs/config.yaml` (development default)

If none are found, the application continues normally using environment variables and CLI flags.

**Environment variables:** Override any config value with the `HYPERFLEET_*` prefix:

```bash
export HYPERFLEET_DATABASE_HOST=localhost
export HYPERFLEET_DATABASE_PORT=5432
export HYPERFLEET_DATABASE_NAME=hyperfleet
export HYPERFLEET_DATABASE_USER=hyperfleet
export HYPERFLEET_DATABASE_PASSWORD=hyperfleet-dev-password
export HYPERFLEET_LOGGING_LEVEL=debug
export HYPERFLEET_SERVER_PORT=8000
```

See [Configuration Guide](config.md) for the complete reference and all available settings.

## Database Setup

**Option A: Local PostgreSQL container (quickest)**

```bash
make db/setup     # Creates a PostgreSQL container via Podman
make db/login     # Connect to the database for inspection
```

**Option B: External PostgreSQL**

Point the config or environment variables to your PostgreSQL instance:

```bash
export HYPERFLEET_DATABASE_HOST=my-postgres-host.example.com
export HYPERFLEET_DATABASE_PORT=5432
export HYPERFLEET_DATABASE_NAME=hyperfleet
export HYPERFLEET_DATABASE_USER=hyperfleet
export HYPERFLEET_DATABASE_PASSWORD=my-password
export HYPERFLEET_DATABASE_SSL_MODE=require   # for remote databases
```

## Running the Service

### Local Development (No Authentication)

```bash
make run-no-auth
```

**Note**: The default runtime environment is `production`. For local development without authentication, use `make run-no-auth` or set `HYPERFLEET_ENV=development` (see [Development Environment Configuration](#development-environment-configuration) below).

The service starts on `localhost:8000` — see [Accessing the API](../README.md#accessing-the-api) for all available endpoints.

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

### Production Mode (JWT Authentication)

**Terminal 1** (start server):

```bash
make run
```

`make run` auto-generates a dev JWT (valid 8 hours) at `/tmp/hf-dev-token.txt`.

**Terminal 2** (call API):

```bash
curl -H "Authorization: Bearer $(cat /tmp/hf-dev-token.txt)" \
  http://localhost:8000/api/hyperfleet/v1/clusters
```

To regenerate the token without restarting: `make dev-token`

See [Deployment](deployment.md) for Kubernetes/Helm deployment and [Authentication](authentication.md) for JWT configuration.

### Schema Validation (Local)

The API validates resource `spec` fields against an OpenAPI schema. Configure the schema path:

```bash
# Via flag
./bin/hyperfleet-api serve --server-openapi-schema-path ./openapi/openapi.yaml

# Via environment variable
export HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH=./openapi/openapi.yaml
```

The API **will fail to start** if the configured schema file is missing, unreadable, or invalid.

### CLI Subcommands

```bash
./bin/hyperfleet-api serve     # Start the HTTP server
./bin/hyperfleet-api migrate   # Run database migrations
./bin/hyperfleet-api version   # Print version, commit, and build date
```

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
| `make run` | Start server with JWT authentication |
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
//go:generate mockgen-v0.6.0 -source=resource.go -package=services -destination=resource_mock.go
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

### Pre-commit Hooks

This project uses pre-commit hooks for code quality and secret scanning.

#### Setup

```bash
# Install pre-commit
brew install pre-commit  # macOS
# or
pip install pre-commit

# Install hooks
make install-hooks

# Test
pre-commit run --all-files
```

The first run takes 3-5 minutes while LeakTK compiles (one-time), then it's instant.

#### Update Hooks

```bash
pre-commit autoupdate
pre-commit run --all-files
```

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

## Development Environment Configuration

### Development Environment Analysis

**Background**: Prior to HYPERFLEET-1133, the API defaulted to `DevelopmentEnv` (insecure). To protect production deployments, HYPERFLEET-1133 changed the default to `ProductionEnv` (secure by default).

**Analysis Question**: Is `e_development.go` still needed after this change?

**Decision**: **KEEP `e_development.go`** with improved documentation.
**Why keep it**:

- ✅ **One variable controls multiple settings** — `HYPERFLEET_ENV=development` forces JWT=false, TLS=false, SSL=disable
- ✅ **Convenient for scripts/CI** — One environment variable vs three separate flags
- ✅ **Semantic clarity** — "development mode" is clearer than remembering individual flags
- ✅ **Consistent with tests** — `unit_testing` and `integration_testing` use the same pattern
- ✅ **Production safe** — `EnvironmentDefault = ProductionEnv` prevents accidental use in production

**How to use**:

```bash
# Full development mode (JWT/TLS/DB SSL all disabled)
HYPERFLEET_ENV=development ./bin/hyperfleet-api serve

# JWT-only no-auth (TLS and DB SSL keep their defaults)
make run-no-auth

# Production mode (JWT/TLS enabled, default)
./bin/hyperfleet-api serve  # Uses EnvironmentDefault = ProductionEnv
```

**⚠️ IMPORTANT**: `HYPERFLEET_ENV=development` is for **local development ONLY**. Never use in production. The development environment forces insecure settings:

- JWT authentication: **disabled**
- TLS encryption: **disabled**
- Database SSL: **disabled**

**Production deployments**: Always use `EnvironmentDefault` (production) or explicitly enable security via Helm values:

```yaml
config:
  server:
    jwt:
      enabled: true  # Production requires JWT
    tls:
      enabled: true  # Production requires TLS
  database:
    ssl:
      mode: verify-full  # Production requires SSL
```

**Alternative to `HYPERFLEET_ENV=development`**: If you prefer explicit flags over environment-based config, you can pass flags directly:

```bash
./bin/hyperfleet-api serve \
  --server-jwt-enabled=false \
  --server-https-enabled=false \
  --db-ssl-mode=disable
```

However, `HYPERFLEET_ENV=development` is recommended for local development as it's simpler and less error-prone.

---

## Related Documentation

- [Configuration Guide](config.md) — Complete configuration reference
- [Database](database.md) — Database schema and migrations
- [Deployment](deployment.md) — Kubernetes/Helm deployment (ops)
- [Authentication](authentication.md) — Authentication configuration
- [API Resources](api-resources.md) — API endpoints and data models
