# Contributing to HyperFleet API

## Development Setup

Complete local environment setup for the HyperFleet API.

```bash
# 1. Clone the repository
git clone https://github.com/openshift-hyperfleet/hyperfleet-api.git
cd hyperfleet-api

# 2. Install prerequisites
# See PREREQUISITES.md for detailed installation instructions
# Required: Go 1.24+, Podman, PostgreSQL 13+, Make

# 3. Generate OpenAPI code and mocks (REQUIRED FIRST STEP)
# Generated code is not checked into git
make generate-all

# 4. Install dependencies
go mod download

# 5. Initialize secrets directory
make secrets

# 6. Setup local PostgreSQL database
make db/setup

# 7. Build the project
make build

# 8. Run database migrations
./bin/hyperfleet-api migrate

# 9. Verify setup works
make test

# 10. Start the server (without authentication for local development)
make run-no-auth
```

**First-time setup notes:**
- **Critical**: Run `make generate-all` before any other commands. Generated code is not in git.
- Tool versions are pinned using [Bingo](https://github.com/bwplotka/bingo) in the `.bingo/` directory
- The build uses FIPS-compliant crypto: `CGO_ENABLED=1 GOEXPERIMENT=boringcrypto`
- Local development runs without authentication by default (`make run-no-auth`)
- Integration tests automatically create isolated PostgreSQL containers via testcontainers

## Repository Structure

Overview of key directories and their purpose.

```
hyperfleet-api/
├── cmd/hyperfleet-api/          # Application entry point and CLI commands
│   └── main.go                  # Server start, migrate, version commands
├── pkg/
│   ├── api/                     # Generated OpenAPI types (DO NOT EDIT)
│   │   └── openapi/             # Generated from openapi/openapi.yaml
│   ├── dao/                     # Data access layer (database interactions)
│   ├── db/                      # Database session factory, migrations, transaction middleware
│   ├── errors/                  # RFC 9457 Problem Details error model
│   ├── handlers/                # HTTP request handlers using handlerConfig pipeline
│   ├── logger/                  # Structured logging (slog-based)
│   ├── presenters/              # Response presenters (DAO models → API responses)
│   └── services/                # Business logic layer (status aggregation, validation)
├── plugins/                     # Plugin-based route registration (clusters, nodepools)
│   ├── clusters/                # Cluster resource plugin
│   └── nodepools/               # NodePool resource plugin
├── openapi/                     # API specification source
│   ├── openapi.yaml             # Source spec (TypeSpec output, has $ref)
│   └── oapi-codegen.yaml        # Code generation configuration
├── test/
│   ├── factories/               # Test data factories
│   └── integration/             # Integration tests (testcontainers)
├── charts/                      # Helm chart for Kubernetes deployment
├── docs/                        # Detailed documentation
├── .bingo/                      # Pinned tool versions (managed by Bingo)
├── Makefile                     # Build automation and common commands
└── CLAUDE.md                    # AI agent context (see also subdirectory CLAUDE.md files)
```

## Testing

How to run unit tests, integration tests, linting, and validation.

### Unit Tests
```bash
# Run all unit tests
make test

# Run tests with coverage report
make test

# Run unit tests in CI mode (JSON output)
make ci-test-unit
```

Unit tests run with `OCM_ENV=unit_testing` and do not require a running database.

### Integration Tests
```bash
# Run integration tests
# These automatically create isolated PostgreSQL containers via testcontainers
make test-integration

# Run integration tests in CI mode (JSON output)
make ci-test-integration
```

Integration tests use testcontainers to create isolated PostgreSQL instances. No manual database setup required.

### Linting and Quality Checks
```bash
# Run all quality checks (fast - no database required)
make verify-all

# Run only static analysis
make verify

# Run golangci-lint
make lint

# Run Helm chart linting
make test-helm

# Run ALL checks (lint + unit + integration + helm)
make test-all
```

**Recommended workflow:**
- Use `make verify-all` for fast feedback during development (no database)
- Use `make test-all` for full validation before pushing

## Common Development Tasks

Build commands, running locally, generating code, and database operations.

### Building
```bash
# Clean build (generates code, builds binary)
make build

# Build all commands under cmd/
make cmds

# Clean temporary files
make clean

# Install binary to GOPATH/bin
make install
```

### Running Locally
```bash
# Run with authentication disabled (local development)
make run-no-auth

# Run with full authentication
make run

# Access the API
curl http://localhost:8000/api/hyperfleet/v1/clusters | jq

# Available endpoints:
# - REST API: http://localhost:8000/api/hyperfleet/v1/
# - OpenAPI spec: http://localhost:8000/api/hyperfleet/v1/openapi
# - Swagger UI: http://localhost:8000/api/hyperfleet/v1/openapi.html
# - Health checks: http://localhost:8080/healthz, http://localhost:8080/readyz
# - Metrics: http://localhost:9090/metrics
```

### Code Generation
```bash
# Generate OpenAPI types from openapi/openapi.yaml
make generate

# Generate mock implementations for services
make generate-mocks

# Generate all code (OpenAPI + mocks) - REQUIRED after fresh clone
make generate-all

# Update vendor directory (if using vendoring)
make generate-vendor
```

**Important:** Never edit files in `pkg/api/openapi/` or `*_mock.go` files. They are auto-generated.

### Database Operations
```bash
# Setup local PostgreSQL container
make db/setup

# Login to local PostgreSQL (psql)
make db/login

# Run migrations
./bin/hyperfleet-api migrate

# Teardown local PostgreSQL container
make db/teardown
```

### Container Images
```bash
# Build container image
make image

# Build and push container image
make image-push

# Build and push to personal Quay registry (requires QUAY_USER env var)
make image-dev
```

## Commit Standards

Follow the HyperFleet commit message format and refer to the [architecture repository commit standards](https://github.com/openshift-hyperfleet/architecture).

**Format:** `HYPERFLEET-### - type: description`

**Example:**
```
HYPERFLEET-123 - fix: handle nil pointer in status aggregation

Add null check before accessing adapter status conditions.

Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>
```

**Types:** `feat`, `fix`, `docs`, `chore`, `test`, `refactor`

**Co-author line:** Always include when pair programming or using AI assistance.

## Release Process

HyperFleet API uses semantic versioning and automated releases.

### Versioning
- Version tags follow semver: `v0.1.0`, `v0.1.1`, `v0.2.0`
- See [CHANGELOG.md](CHANGELOG.md) for release history
- Helm chart version is independent from app version

### Release Workflow
1. Version tags are created on `main` branch
2. Release branches follow pattern: `release-X.Y` (e.g., `release-0.1`)
3. Container images are built and pushed automatically by CI/CD
4. Helm chart is versioned independently in `charts/Chart.yaml`

**Note:** Releases are managed by team leads. Contributors should focus on PRs to `main`.

---

## Additional Resources

- **Architecture Documentation**: https://github.com/openshift-hyperfleet/architecture
- **API Documentation**: See [docs/api-resources.md](docs/api-resources.md)
- **Development Guide**: See [docs/development.md](docs/development.md)
- **Search & Filtering**: See [docs/search.md](docs/search.md)
- **CLAUDE.md**: AI agent context (main + subdirectories)
