# HyperFleet API

HyperFleet API - Simple REST API for cluster lifecycle management. Provides CRUD operations for clusters and status sub-resources. Pure data layer with PostgreSQL integration - no business logic or event creation. Stateless design enables horizontal scaling.

## Architecture


### Core Features

* OpenAPI 3.0 specification
* Automated Go code generation from OpenAPI
* Cluster and NodePool lifecycle management (create, patch, delete, force-delete)
* Generic resource types (WifConfigs, Channels, Versions) via plugin-based registration
* Adapter-based status reporting with Kubernetes-style conditions
* Soft-delete with adapter finalization and force-delete for stuck resources
* Descriptor-driven delete policies (restrict/cascade) for generic resources
* Configurable caller identity for audit fields (HTTP header or JWT claim)
* Runtime spec validation against custom OpenAPI schemas
* Pagination and search capabilities

### Technology Stack

- **Language**: Go 1.25+
- **API Definition**: OpenAPI 3.0
- **Code Generation**: oapi-codegen
- **Database**: PostgreSQL with GORM ORM
- **Container Runtime**: Podman
- **Testing**: Gomega + Resty

## Getting Started

### Deploying to Kubernetes

For Helm-based deployment to staging, production, or partner environments, see the **[Deployment Guide](docs/deployment.md)** — covers container images, Helm values, external databases, schema validation, monitoring, and production checklists.

### Local Development

For setting up a local development environment, see the **[Development Guide](docs/development.md)** — covers prerequisites, code generation, mock generation, database setup, running tests, pre-commit hooks, and development workflows.

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
- `GET/PATCH/DELETE /api/hyperfleet/v1/clusters/{id}`
- `POST /api/hyperfleet/v1/clusters/{id}/force-delete`
- `GET/PUT /api/hyperfleet/v1/clusters/{id}/statuses`

### NodePools

Groups of compute nodes within clusters.

**Main endpoints:**
- `GET /api/hyperfleet/v1/nodepools`
- `GET/POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools`
- `GET/PATCH/DELETE /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}`
- `POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/force-delete`
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

## Documentation

### Core Documentation

- **[API Resources](docs/api-resources.md)** - API endpoints, data models, and search capabilities
- **[Development Guide](docs/development.md)** - Local setup, testing, code generation, and workflows
- **[Database](docs/database.md)** - Schema, migrations, and data model
- **[Deployment](docs/deployment.md)** - Container images, Kubernetes deployment, and configuration
- **[Configuration](docs/config.md)** - Complete configuration reference (database, server, caller identity, adapters)
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
