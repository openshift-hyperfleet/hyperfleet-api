# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

HyperFleet API is a stateless REST API that serves as the pure data layer for the HyperFleet cluster lifecycle management system. It provides CRUD operations for clusters and node pools, accepts status updates from adapters, and stores all resource data in PostgreSQL. This API contains no business logic and creates no events - it is purely a data persistence and retrieval service.

## Architecture Context

HyperFleet API is one component in the HyperFleet architecture:

- **HyperFleet API** (this service): Pure CRUD data layer with PostgreSQL
- **Sentinel Service**: Centralized business logic and event publishing
- **Adapters**: Execute operations (DNS, Hypershift, etc.) and report status back to API

The API's role is strictly limited to:
1. Accept resource creation/update/delete requests
2. Persist resource data to PostgreSQL
3. Accept status updates from adapters via POST `/{resourceType}/{id}/statuses`
4. Serve resource data to Sentinel via GET `/{resourceType}`
5. Calculate aggregate status from adapter conditions

## Technology Stack

### Core Technologies
- **Language**: Go 1.24.9 with FIPS-compliant crypto (CGO_ENABLED=1, GOEXPERIMENT=boringcrypto)
- **Database**: PostgreSQL 14.2 with GORM ORM
- **API Specification**: TypeSpec → OpenAPI 3.0.3
- **Code Generation**: openapi-generator-cli v7.16.0
- **Container Runtime**: Podman
- **Testing**: gotestsum, Gomega, Resty, Testcontainers

### Why These Choices

**Go 1.24**: Required for FIPS compliance in enterprise/government deployments

**TypeSpec**: Provides type-safe API specification with better maintainability than writing OpenAPI YAML manually

**GORM**: Provides database abstraction with migration support and PostgreSQL-specific features

**Testcontainers**: Enables integration tests with real PostgreSQL instances without external dependencies

## Development Commands

### Building and Running
```bash
make build       # Build the hyperfleet-api binary to bin/
make install     # Build and install binary to GOPATH/bin
make run         # Run migrations and start server with authentication
make run-no-auth # Run server without authentication (development mode)
```

### Testing
```bash
make test                # Run unit tests
make test-integration    # Run integration tests
make ci-test-unit        # Run unit tests with JSON output for CI
make ci-test-integration # Run integration tests with JSON output for CI
```

### Code Quality
```bash
make verify # Run source code verification (vet, formatting)
make lint   # Run golangci-lint
```

### Database Operations
```bash
make db/setup     # Start PostgreSQL container locally
make db/login     # Connect to local PostgreSQL database
make db/teardown  # Stop and remove PostgreSQL container
./bin/hyperfleet-api migrate # Run database migrations
```

### Code Generation
```bash
make generate        # Regenerate Go models from openapi/openapi.yaml
make generate-vendor # Generate using vendor dependencies (offline mode)
```

## Project Structure

```
hyperfleet-api/
├── cmd/hyperfleet/              # Application entry point
│   ├── migrate/                 # Database migration command
│   ├── serve/                   # API server command
│   └── environments/            # Environment configuration
│       ├── development.go       # Local development settings
│       ├── integration_testing.go # Integration test settings
│       ├── unit_testing.go      # Unit test settings
│       └── production.go        # Production settings
├── pkg/
│   ├── api/                     # API models and OpenAPI spec
│   │   ├── openapi/             # Generated Go models
│   │   │   ├── api/openapi.yaml # Embedded OpenAPI spec (44KB, fully resolved)
│   │   │   └── model_*.go       # Generated model structs
│   │   └── openapi_embed.go     # Go embed directive for OpenAPI spec
│   ├── dao/                     # Data Access Objects
│   │   ├── cluster.go           # Cluster CRUD operations
│   │   ├── nodepool.go          # NodePool CRUD operations
│   │   ├── adapter_status.go    # Status CRUD operations
│   │   └── label.go             # Label operations
│   ├── db/                      # Database layer
│   │   ├── db.go                # GORM connection and session factory
│   │   ├── transaction_middleware.go # HTTP middleware for DB transactions
│   │   └── migrations/          # GORM migration files
│   ├── handlers/                # HTTP request handlers
│   │   ├── cluster_handler.go   # Cluster endpoint handlers
│   │   ├── nodepool_handler.go  # NodePool endpoint handlers
│   │   └── compatibility_handler.go # API compatibility endpoint
│   ├── services/                # Service layer (status aggregation, search)
│   │   ├── cluster_service.go   # Cluster business operations
│   │   └── nodepool_service.go  # NodePool business operations
│   ├── config/                  # Configuration management
│   ├── logger/                  # Structured logging
│   └── errors/                  # Error handling utilities
├── openapi/
│   └── openapi.yaml             # TypeSpec-generated OpenAPI spec (32KB, source)
├── test/
│   ├── integration/             # Integration tests for all endpoints
│   └── factories/               # Test data factories
└── Makefile                     # Build automation
```

## Core Components

### 1. API Specification Workflow

The API is specified using TypeSpec, which compiles to OpenAPI, which then generates Go models:

```
TypeSpec (.tsp files in hyperfleet-api-spec repo)
    ↓ tsp compile
openapi/openapi.yaml (32KB, uses $ref for DRY)
    ↓ make generate (openapi-generator-cli in Podman)
pkg/api/openapi/model_*.go (Go structs)
pkg/api/openapi/api/openapi.yaml (44KB, fully resolved, embedded in binary)
```

**Key Points**:
- TypeSpec definitions are maintained in a separate `hyperfleet-api-spec` repository
- `openapi/openapi.yaml` is the source of truth for this repository (generated from TypeSpec)
- `make generate` uses Podman to run openapi-generator-cli, ensuring consistent versions
- Generated code includes JSON tags, validation, and type definitions
- The fully resolved spec is embedded at compile time via `//go:embed`

### 2. Database Layer

**GORM Session Management**:
```go
// pkg/db/db.go
type SessionFactory interface {
    NewSession(ctx context.Context) *gorm.DB
    Close() error
}
```

**Transaction Middleware**:
All HTTP requests automatically get a database session via middleware at pkg/db/transaction_middleware.go:13:
```go
func TransactionMiddleware(next http.Handler, connection SessionFactory) http.Handler {
    // Creates session for each request
    // Stores in context
    // Auto-commits on success, rolls back on error
}
```

**Schema**:
```sql
-- Core resource tables
clusters (id, name, spec JSONB, generation, labels, created_at, updated_at)
node_pools (id, name, owner_id FK, spec JSONB, labels, created_at, updated_at)

-- Status tracking
adapter_statuses (owner_type, owner_id, adapter, observed_generation, conditions JSONB)

-- Labels for filtering
labels (owner_type, owner_id, key, value)
```

**Migration System**:
GORM AutoMigrate is used at startup via `./bin/hyperfleet-api migrate` command.

### 3. Data Access Objects (DAO)

DAOs provide CRUD operations with GORM:

**Example - Cluster DAO**:
```go
type ClusterDAO interface {
    Create(ctx context.Context, cluster *api.Cluster) (*api.Cluster, error)
    Get(ctx context.Context, id string) (*api.Cluster, error)
    List(ctx context.Context, listArgs *ListArgs) (*api.ClusterList, error)
    Update(ctx context.Context, cluster *api.Cluster) (*api.Cluster, error)
    Delete(ctx context.Context, id string) error
}
```

**Patterns**:
- All DAO methods take `context.Context` for transaction propagation
- Session is retrieved from context via `db.NewContext()`
- List operations support pagination via `ListArgs`
- Search is implemented via GORM WHERE clauses

### 4. HTTP Handlers

Handlers follow a consistent pattern at pkg/handlers/:

```go
func (h *clusterHandler) Create(w http.ResponseWriter, r *http.Request) {
    // 1. Parse request body
    var cluster openapi.Cluster
    json.NewDecoder(r.Body).Decode(&cluster)

    // 2. Call service/DAO
    result, err := h.service.Create(r.Context(), &cluster)

    // 3. Handle errors
    if err != nil {
        errors.SendError(w, r, err)
        return
    }

    // 4. Send response
    w.WriteHeader(http.StatusCreated)
    json.NewEncoder(w).Encode(result)
}
```

### 5. Status Aggregation Pattern

The API calculates aggregate status from adapter-specific conditions:

**Adapter Status Structure**:
```json
{
  "adapter": "dns-adapter",
  "observed_generation": 1,
  "conditions": [
    {
      "adapter": "dns-adapter",
      "type": "Ready",
      "status": "True",
      "observed_generation": 1,
      "reason": "ClusterProvisioned",
      "message": "Cluster successfully provisioned",
      "created_at": "2025-11-17T15:04:05Z",
      "updated_at": "2025-11-17T15:04:05Z"
    }
  ]
}
```

**Aggregation Logic**:
- Phase is `Ready` if all adapters report `Ready=True`
- Phase is `Failed` if any adapter reports `Ready=False`
- Phase is `NotReady` otherwise (progressing, unknown, or missing conditions)
- `observed_generation` tracks which spec version the adapter has seen

**Why This Pattern**:
Kubernetes-style conditions allow multiple independent adapters to report status without coordination. The API simply aggregates these into a summary phase for client convenience.

## API Resources

### Cluster

**Endpoints**:
- `GET /api/hyperfleet/v1/clusters` - List with pagination and search
- `POST /api/hyperfleet/v1/clusters` - Create new cluster
- `GET /api/hyperfleet/v1/clusters/{cluster_id}` - Get single cluster
- `GET /api/hyperfleet/v1/clusters/{cluster_id}/statuses` - Get adapter statuses
- `POST /api/hyperfleet/v1/clusters/{cluster_id}/statuses` - Report status from adapter

**Key Fields**:
- `spec` (JSON): Cloud provider configuration (region, version, nodes, etc.)
- `generation` (int): Increments on each spec change, enables optimistic concurrency
- `labels` (map): Key-value pairs for categorization and filtering
- `status.observed_generation`: Latest generation that adapters have processed

### NodePool

**Endpoints**:
- `GET /api/hyperfleet/v1/nodepools` - List all node pools
- `GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools` - List cluster's node pools
- `POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools` - Create node pool
- `GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}` - Get single node pool
- `GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses` - Get statuses
- `POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses` - Report status

**Key Fields**:
- `owner_references.id`: Parent cluster ID (enforced via foreign key)
- `spec` (JSON): Instance type, replica count, disk size, etc.
- Status follows same pattern as Cluster

## hyperfleet CLI Commands

The `hyperfleet` binary provides two main subcommands:

### `hyperfleet serve` - Start the API Server

Serves the hyperfleet REST API with full authentication, database connectivity, and monitoring capabilities.

**Basic Usage:**
```bash
./bin/hyperfleet-api serve                              # Start server on localhost:8000
./bin/hyperfleet-api serve --api-server-bindaddress :8080  # Custom bind address
./bin/hyperfleet-api serve --enable-authz=false --enable-jwt=false  # No authentication
```

**Key Configuration Options:**

- **Server Binding:**
  - `--api-server-bindaddress` - API server bind address (default: "localhost:8000")
  - `--api-server-hostname` - Server's public hostname
  - `--enable-https` - Enable HTTPS rather than HTTP
  - `--https-cert-file` / `--https-key-file` - TLS certificate files

- **Database Configuration:**
  - `--db-host-file` - Database host file (default: "secrets/db.host")
  - `--db-name-file` - Database name file (default: "secrets/db.name")
  - `--db-user-file` - Database username file (default: "secrets/db.user")
  - `--db-password-file` - Database password file (default: "secrets/db.password")
  - `--db-port-file` - Database port file (default: "secrets/db.port")
  - `--db-sslmode` - Database SSL mode: disable | require | verify-ca | verify-full (default: "disable")
  - `--db-max-open-connections` - Maximum open DB connections (default: 50)
  - `--enable-db-debug` - Enable database debug mode

- **Authentication & Authorization:**
  - `--enable-jwt` - Enable JWT authentication validation (default: true)
  - `--enable-authz` - Enable authorization on endpoints (default: true)
  - `--jwk-cert-url` - JWK Certificate URL for JWT validation (default: Red Hat SSO)
  - `--jwk-cert-file` - Local JWK Certificate file
  - `--acl-file` - Access control list file

- **OCM Integration:**
  - `--enable-ocm-mock` - Enable mock OCM clients (default: true)
  - `--ocm-base-url` - OCM API base URL (default: integration environment)
  - `--ocm-token-url` - OCM token endpoint URL (default: Red Hat SSO)
  - `--ocm-client-id-file` - OCM API client ID file (default: "secrets/ocm-service.clientId")
  - `--ocm-client-secret-file` - OCM API client secret file (default: "secrets/ocm-service.clientSecret")
  - `--self-token-file` - OCM API privileged offline SSO token file
  - `--ocm-debug` - Enable OCM API debug logging

- **Monitoring & Health Checks:**
  - `--health-check-server-bindaddress` - Health check server address (default: "localhost:8083")
  - `--enable-health-check-https` - Enable HTTPS for health check server
  - `--metrics-server-bindaddress` - Metrics server address (default: "localhost:8080")
  - `--enable-metrics-https` - Enable HTTPS for metrics server

- **Performance Tuning:**
  - `--http-read-timeout` - HTTP server read timeout (default: 5s)
  - `--http-write-timeout` - HTTP server write timeout (default: 30s)
  - `--label-metrics-inclusion-duration` - Telemetry collection timeframe (default: 168h)

### `hyperfleet migrate` - Run Database Migrations

Executes database schema migrations to set up or update the database structure.

**Basic Usage:**
```bash
./bin/hyperfleet-api migrate                           # Run all pending migrations
./bin/hyperfleet-api migrate --enable-db-debug        # Run with database debug logging
```

**Configuration Options:**
- **Database Connection:** (same as serve command)
  - `--db-host-file`, `--db-name-file`, `--db-user-file`, `--db-password-file`
  - `--db-port-file`, `--db-sslmode`, `--db-rootcert`
  - `--db-max-open-connections` - Maximum DB connections (default: 50)
  - `--enable-db-debug` - Enable database debug mode

**Migration Process:**
- Applies all pending migrations in order
- Creates migration tracking table if needed
- Idempotent - safe to run multiple times
- Logs each migration applied

### Common Global Flags

All subcommands support these logging flags:
- `--logtostderr` - Log to stderr instead of files (default: true)
- `--alsologtostderr` - Log to both stderr and files
- `--log_dir` - Directory for log files
- `--stderrthreshold` - Minimum log level for stderr (default: 2)
- `-v, --v` - Log level for verbose logs
- `--vmodule` - Module-specific log levels
- `--log_backtrace_at` - Emit stack trace at specific file:line

## Development Workflow

### Environment Setup

```bash
# Prerequisites: Go 1.24, Podman, PostgreSQL client tools

# Generate OpenAPI code (required before go mod download)
make generate

# Download Go module dependencies
go mod download

# Initialize secrets directory with default values
make secrets

# Start PostgreSQL
make db/setup

# Build binary
make build

# Run migrations
./bin/hyperfleet-api migrate

# Start server (no authentication)
make run-no-auth
```

### Code Generation

When the TypeSpec specification changes:

```bash
# Regenerate Go models from openapi/openapi.yaml
make generate

# This will:
# 1. Remove pkg/api/openapi/*
# 2. Build Docker image with openapi-generator-cli
# 3. Generate model_*.go files
# 4. Copy fully resolved openapi.yaml to pkg/api/openapi/api/
```

### Testing

**Unit Tests**:
```bash
OCM_ENV=unit_testing make test
```

**Integration Tests**:
```bash
OCM_ENV=integration_testing make test-integration
```

Integration tests use Testcontainers to spin up real PostgreSQL instances. Each test gets a fresh database to ensure isolation.

### Database Operations

```bash
# Connect to database
make db/login

# Inspect schema
\dt

# Stop database
make db/teardown
```

## Configuration Management

### Environment-Based Configuration

The application uses `OCM_ENV` environment variable to select configuration:

- `development` - Local development with localhost database
- `unit_testing` - In-memory or minimal database
- `integration_testing` - Testcontainers-based PostgreSQL
- `production` - Production credentials from secrets

**Environment Implementation**: See cmd/hyperfleet/environments/framework.go:66

Each environment can override:
- Database connection settings
- OCM client configuration (mock vs real)
- Service implementations
- Handler configurations

### Configuration Files

Configuration is loaded from `secrets/` directory:

```
secrets/
├── db.host          # Database hostname
├── db.name          # Database name
├── db.password      # Database password
├── db.port          # Database port
├── db.user          # Database username
├── ocm-service.clientId
├── ocm-service.clientSecret
└── ocm-service.token
```

Initialize with defaults: `make secrets`

## Logging

Structured logging is provided via pkg/logger/logger.go:36:

```go
log := logger.NewOCMLogger(ctx)
log.Infof("Processing cluster %s", clusterID)
log.Extra("cluster_id", clusterID).Extra("operation", "create").Info("Cluster created")
```

**Log Context**:
- `[opid=xxx]` - Operation ID for request tracing
- `[accountID=xxx]` - User account ID from JWT
- `[tx_id=xxx]` - Database transaction ID

## Error Handling

Errors use a structured error type defined in pkg/errors/:

```go
type ServiceError struct {
    HttpCode int
    Code     string
    Reason   string
}
```

**Pattern**:
```go
if err != nil {
    serviceErr := errors.GeneralError("Failed to create cluster")
    errors.SendError(w, r, serviceErr)
    return
}
```

Errors are automatically converted to OpenAPI error responses with operation IDs for debugging.

## Authentication & Authorization

The API supports two modes:

**No Auth** (development):
```bash
make run-no-auth
```

**OCM JWT Auth** (production):
- Validates JWT tokens from Red Hat SSO
- Extracts account ID and username from claims
- Enforces organization-based access control

**Implementation**: JWT middleware validates tokens and populates context with user information.

## Key Design Patterns

### 1. Context-Based Session Management

Database sessions are stored in request context via middleware. This ensures:
- Automatic transaction lifecycle
- Thread-safe session access
- Proper cleanup on request completion

### 2. Polymorphic Status Tables

`adapter_statuses` uses `owner_type` + `owner_id` to support multiple resource types:
```sql
SELECT * FROM adapter_statuses
WHERE owner_type = 'Cluster' AND owner_id = '123'
```

This avoids creating separate status tables for each resource type.

### 3. Generation-Based Optimistic Concurrency

The `generation` field increments on each spec update:
```go
cluster.Generation++  // On each update
```

Adapters report `observed_generation` in status to indicate which version they've processed. This enables:
- Detecting when spec has changed since adapter last processed
- Preventing race conditions in distributed systems
- Tracking reconciliation progress

### 4. Embedded OpenAPI Specification

The OpenAPI spec is embedded at compile time using Go 1.16+ `//go:embed`:

```go
//go:embed openapi/api/openapi.yaml
var openapiFS embed.FS
```

This means:
- No file I/O at runtime
- Spec is always available even in containers
- Swagger UI works without external files
- Binary is self-contained

## Testing Strategy

### Integration Test Coverage

All 12 API endpoints have integration test coverage in test/integration/:

- Cluster CRUD operations
- NodePool CRUD operations
- Status reporting and aggregation
- Pagination behavior
- Search functionality
- Error cases (not found, validation errors)

### Test Data Factories

Test factories in test/factories/ provide consistent test data:

```go
factories.NewClusterBuilder().
    WithName("test-cluster").
    WithSpec(clusterSpec).
    Build()
```

### Testcontainers Pattern

Integration tests use Testcontainers to create isolated PostgreSQL instances:

```go
// Each test suite gets a fresh database
container := testcontainers.PostgreSQL()
defer container.Terminate()
```

This ensures:
- No state leakage between tests
- Tests can run in parallel
- No external database dependency

### Database Issues During Testing

If integration tests fail with PostgreSQL-related errors (missing columns, transaction issues), recreate the database:

```bash
# From project root directory
make db/teardown  # Stop and remove PostgreSQL container
make db/setup     # Start fresh PostgreSQL container
./bin/hyperfleet-api migrate # Apply migrations
make test-integration  # Run tests again
```

**Note:** Always run `make` commands from the project root directory where the Makefile is located.

## Common Development Tasks

### Debugging Database Issues

```bash
# Connect to database
make db/login

# Check what GORM created
\dt                    # List tables
\d clusters            # Describe clusters table
\d adapter_statuses    # Check status table

# Inspect data
SELECT id, name, generation FROM clusters;
SELECT owner_type, owner_id, adapter, conditions FROM adapter_statuses;
```

### Viewing OpenAPI Specification

```bash
# Start server
make run-no-auth

# View raw OpenAPI spec
curl http://localhost:8000/openapi

# Use Swagger UI
open http://localhost:8000/openapi-ui
```

## Server Configuration

The server is configured in cmd/hyperfleet/server/:

**Ports**:
- `8000` - Main API server
- `8080` - Metrics endpoint
- `8083` - Health check endpoint

**Middleware Chain**:
1. Request logging
2. Operation ID injection
3. JWT authentication (if enabled)
4. Database transaction creation
5. Route handler

**Implementation**: See cmd/hyperfleet/server/server.go:19

## Common Pitfalls

### 1. Forgetting to Run Migrations

**Symptom**: Server starts but endpoints return errors about missing tables

**Solution**: Always run `./bin/hyperfleet-api migrate` after pulling code or changing schemas

### 2. Using Wrong OpenAPI File

**Problem**: There are two openapi.yaml files:
- `openapi/openapi.yaml` (32KB, source, has $ref)
- `pkg/api/openapi/api/openapi.yaml` (44KB, generated, fully resolved)

**Rule**: Only edit the source file. The generated file is overwritten by `make generate`.

### 3. Context Session Access

**Wrong**:
```go
db := gorm.Open(...)  // Creates new connection
```

**Right**:
```go
db := db.NewContext(ctx)  // Gets session from middleware
```

Always use the context-based session to participate in the HTTP request transaction.

### 4. Status Phase Calculation

The API automatically calculates status.phase from adapter conditions. Don't set phase manually - it will be overwritten.

## Performance Considerations

### Database Indexes

Ensure indexes exist for common queries:
```sql
CREATE INDEX idx_clusters_name ON clusters(name);
CREATE INDEX idx_adapter_statuses_owner ON adapter_statuses(owner_type, owner_id);
CREATE INDEX idx_labels_owner ON labels(owner_type, owner_id);
```

### JSONB Queries

Spec and conditions are stored as JSONB, enabling:
```sql
-- Query by spec field
SELECT * FROM clusters WHERE spec->>'region' = 'us-west-2';

-- Query by condition
SELECT * FROM adapter_statuses
WHERE conditions @> '[{"type": "Ready", "status": "True"}]';
```

### Connection Pooling

GORM manages connection pooling automatically. Configure via:
```go
db.DB().SetMaxOpenConns(100)
db.DB().SetMaxIdleConns(10)
```

## Deployment

The API is designed to be stateless and horizontally scalable:

- No in-memory state
- All data in PostgreSQL
- No event creation or message queues
- Kubernetes-ready (multiple replicas)

**Health Check**: `GET /healthcheck` returns 200 OK when database is accessible

**Metrics**: Prometheus metrics available at `/metrics`

## References

- **Architecture Documentation**: `/Users/ymsun/Documents/workspace/src/github.com/openshift-hyperfleet/architecture`
- **TypeSpec Repository**: `hyperfleet-api-spec` (API specification source)
- **GORM Documentation**: https://gorm.io/docs/
- **OpenAPI Generator**: https://openapi-generator.tech/
- **Testcontainers**: https://testcontainers.com/

## Getting Help

Common issues and solutions:

1. **Database connection errors**: Check `make db/setup` was run and container is running
2. **Generated code issues**: Run `make generate` to regenerate from OpenAPI spec
3. **Test failures**: Ensure PostgreSQL container is running and `OCM_ENV` is set
4. **Build errors**: Verify Go version is 1.24+ with `go version`
