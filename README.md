# HyperFleet API

HyperFleet API - Simple REST API for cluster lifecycle management. Provides CRUD operations for clusters and status sub-resources. Pure data layer with PostgreSQL integration - no business logic or event creation. Stateless design enables horizontal scaling.

## Architecture

### Technology Stack

- **Language**: Go 1.25+
- **API Definition**: OpenAPI 3.0
- **Code Generation**: oapi-codegen
- **Database**: PostgreSQL with GORM ORM
- **Container Runtime**: Podman
- **Testing**: Gomega + Resty

### Core Features

* OpenAPI 3.0 specification
* Automated Go code generation from OpenAPI
* Cluster and NodePool lifecycle management
* Generic resource types (WifConfigs, Channels, Versions) via plugin-based registration
* Adapter-based status reporting with Kubernetes-style conditions
* Runtime spec validation against custom OpenAPI schemas
* Pagination and search capabilities
* Complete integration test coverage
* Database migrations with gormigrate
* Embedded OpenAPI specification using `//go:embed`

### Project Structure

```text
hyperfleet-api/
├── cmd/hyperfleet-api/          # Application entry point
├── pkg/
│   ├── api/                     # API models and type definitions
│   ├── dao/                     # Data access layer
│   ├── db/                      # Database setup, migrations, and session management
│   ├── handlers/                # HTTP request handlers
│   └── services/                # Service layer (status aggregation, CRUD)
├── plugins/                     # Plugin-based resource registration
│   ├── clusters/                # Cluster resource plugin
│   ├── nodePools/               # NodePool resource plugin
│   ├── wifconfigs/              # WifConfig resource plugin
│   ├── channels/                # Channel resource plugin
│   ├── versions/                # Version resource plugin (child of Channel)
│   └── generic/                 # Generic resource framework
├── openapi/                     # Generated artifacts from hyperfleet-api-spec module
├── test/                        # Integration tests and factories
├── docs/                        # Detailed documentation
└── Makefile                     # Build automation
```

## Quick Start

### Prerequisites

- **Go 1.25+**, **Podman**, **PostgreSQL 14+**, **Make**

See [PREREQUISITES.md](PREREQUISITES.md) for installation instructions.

### Installation

```bash
# 1. Generate OpenAPI code and mocks
make generate-all

# 2. Install dependencies
go mod download

# 3. Build binary
make build

# 4. Setup database (local PostgreSQL container)
make db/setup

# 5. Copy config file
cp configs/config.yaml.example configs/config.yaml

# 6. Run migrations
./bin/hyperfleet-api migrate

# 7. Start service (no auth)
make run-no-auth
```

**Note**: Generated code is not tracked in git. You must run `make generate-all` after cloning.

The `migrate` and `serve` commands require a configuration file. The loader checks `--config` flag, then `HYPERFLEET_CONFIG` env var, then `/etc/hyperfleet/config.yaml`, then `./configs/config.yaml`.
If none are found, the command fails with `failed to load configuration`. Copy the example config or point to your own.

For production database setup (external PostgreSQL, Cloud SQL, etc.), see [docs/deployment.md](docs/deployment.md#production-deployment).

### Accessing the API

The service starts on `localhost:8000`:

- **REST API**: `http://localhost:8000/api/hyperfleet/v1/`
- **OpenAPI spec**: `http://localhost:8000/api/hyperfleet/v1/openapi`
- **Swagger UI**: `http://localhost:8000/api/hyperfleet/v1/openapi.html`
- **Liveness probe**: `http://localhost:8080/healthz`
- **Readiness probe**: `http://localhost:8080/readyz`
- **Metrics**: `http://localhost:9090/metrics`

```bash
# Test the API
curl http://localhost:8000/api/hyperfleet/v1/clusters | jq
```

## API Resources

### Clusters

Kubernetes clusters with provider-specific configurations, labels, and adapter-based status reporting.

**Main endpoints:**
- `GET/POST /api/hyperfleet/v1/clusters`
- `GET /api/hyperfleet/v1/clusters/{id}`
- `GET/PUT /api/hyperfleet/v1/clusters/{id}/statuses`

### NodePools

Groups of compute nodes within clusters.

**Main endpoints:**
- `GET /api/hyperfleet/v1/nodepools`
- `GET/POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools`
- `GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}`
- `GET/PUT /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses`

### Generic Resources

The API also supports generic resource types registered via the plugin system. Currently available:

- **WifConfigs** — `GET/POST /api/hyperfleet/v1/wifconfigs`, `GET/PATCH/DELETE .../wifconfigs/{id}`
- **Channels** — `GET/POST /api/hyperfleet/v1/channels`, `GET/PATCH/DELETE .../channels/{id}`
- **Versions** — `GET/POST /api/hyperfleet/v1/channels/{parent_id}/versions`, `GET/PATCH/DELETE .../versions/{id}` (child of Channel)

All resources support pagination, label-based search, and spec validation. Clusters and NodePools additionally support adapter status reporting. See [docs/api-resources.md](docs/api-resources.md) for complete API documentation.

## Example Usage

```bash
# Create a cluster
curl -X POST http://localhost:8000/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{"kind": "Cluster", "name": "my-cluster", "spec": {...}, "labels": {...}}' | jq

# Search clusters
curl -G http://localhost:8000/api/hyperfleet/v1/clusters \
  --data-urlencode "search=labels.environment='production'" | jq

# Search reconciled clusters in a specific region
curl -G http://localhost:8000/api/hyperfleet/v1/clusters \
  --data-urlencode "search=status.conditions.Reconciled='True' and labels.region='us-east'" | jq
```

See [docs/search.md](docs/search.md) for search and filtering documentation.

## Development

### Common Commands

```bash
make build               # Build binary to bin/
make run-no-auth         # Run without authentication
make test                # Run unit tests
make test-integration    # Run integration tests
make generate            # Generate OpenAPI models
make generate-mocks      # Generate test mocks
make generate-all        # Generate OpenAPI models and mocks
make db/setup            # Create PostgreSQL container
make image               # Build container image
```

See [docs/development.md](docs/development.md) for detailed workflows.

### CLI Subcommands

```bash
./bin/hyperfleet-api serve     # Start the HTTP server
./bin/hyperfleet-api migrate   # Run database migrations
./bin/hyperfleet-api version   # Print version, commit, and build date
```

### Pre-commit Hooks

This project uses [pre-commit](https://pre-commit.io/) for code quality checks. See [docs/development.md](docs/development.md#pre-commit-hooks-optional) for setup instructions.

## Documentation

### Core Documentation

- **[API Resources](docs/api-resources.md)** - API endpoints, data models, and search capabilities
- **[Development Guide](docs/development.md)** - Local setup, testing, code generation, and workflows
- **[Database](docs/database.md)** - Schema, migrations, and data model
- **[Deployment](docs/deployment.md)** - Container images, Kubernetes deployment, and configuration
- **[Authentication](docs/authentication.md)** - Development and production auth
- **[Logging](docs/logging.md)** - Structured logging, OpenTelemetry integration, and data masking
- **[Validation Schema](openapi/README.md#validation-schema)** - How to supply a custom OpenAPI schema for runtime `spec` field validation

### Additional Resources

- **[PREREQUISITES.md](PREREQUISITES.md)** - Prerequisite installation
- **[Search and Filtering](docs/search.md)** - Guide to TSL query syntax, operators, and examples
- **[docs/continuous-delivery-migration.md](docs/continuous-delivery-migration.md)** - CD migration guide
- **[docs/dao.md](docs/dao.md)** - Data access patterns
- **[docs/testcontainers.md](docs/testcontainers.md)** - Testcontainers usage

## License

This project is licensed under the Apache License 2.0. See [LICENSE](LICENSE) for details.
