# HyperFleet API

HyperFleet API - Simple REST API for cluster lifecycle management. Provides CRUD operations for clusters and status sub-resources. Pure data layer with PostgreSQL integration - no business logic or event creation. Stateless design enables horizontal scaling.

![HyperFleet](rhtap-hyperfleet_sm.png)

## Architecture

### Technology Stack

- **Language**: Go 1.24.9
- **API Definition**: TypeSpec → OpenAPI 3.0.3
- **Code Generation**: openapi-generator-cli v7.16.0
- **Database**: PostgreSQL with GORM ORM
- **Container Runtime**: Podman
- **Testing**: Gomega + Resty

### Core Features

* TypeSpec-based API specification
* OpenAPI 3.0 code generation workflow
* Cluster and NodePool lifecycle management
* Adapter-based status reporting with Kubernetes-style conditions
* Pagination and search capabilities
* Complete integration test coverage
* Database migrations with GORM
* Embedded OpenAPI specification using `//go:embed`

## Project Structure

```
hyperfleet-api/
├── cmd/hyperfleet/              # Application entry point
├── pkg/
│   ├── api/                     # API models and handlers
│   │   ├── openapi/             # Generated Go models from OpenAPI
│   │   │   ├── api/             # Embedded OpenAPI specification
│   │   │   └── model_*.go       # Generated model structs
│   │   └── openapi_embed.go     # Go embed directive
│   ├── dao/                     # Data access layer
│   ├── db/                      # Database setup and migrations
│   ├── handlers/                # HTTP request handlers
│   ├── services/                # Business logic
│   └── server/                  # Server configuration
├── openapi/                     # API specification source
│   └── openapi.yaml             # TypeSpec-generated OpenAPI spec (32KB)
├── test/
│   ├── integration/             # Integration tests
│   └── factories/               # Test data factories
└── Makefile                     # Build automation
```

## API Resources

### Cluster Management

Cluster resources represent Kubernetes clusters managed across different cloud providers.

**Endpoints:**
```
GET    /api/hyperfleet/v1/clusters
POST   /api/hyperfleet/v1/clusters
GET    /api/hyperfleet/v1/clusters/{cluster_id}
GET    /api/hyperfleet/v1/clusters/{cluster_id}/statuses
POST   /api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Data Model:**
```json
{
  "kind": "Cluster",
  "id": "string",
  "name": "string",
  "generation": 1,
  "spec": {
    "region": "us-west-2",
    "version": "4.15",
    "nodes": 3
  },
  "labels": {
    "env": "production"
  },
  "status": {
    "phase": "Ready",
    "observed_generation": 1,
    "adapters": [...]
  }
}
```

**Status Phases:**
- `NotReady` - Cluster is being provisioned or has failing conditions
- `Ready` - All adapter conditions report success
- `Failed` - Cluster provisioning or operation failed

### NodePool Management

NodePool resources represent groups of compute nodes within a cluster.

**Endpoints:**
```
GET    /api/hyperfleet/v1/nodepools
GET    /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
POST   /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
GET    /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
GET    /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses
POST   /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses
```

**Data Model:**
```json
{
  "kind": "NodePool",
  "id": "string",
  "name": "string",
  "owner_references": {
    "kind": "Cluster",
    "id": "cluster_id"
  },
  "spec": {
    "instance_type": "m5.2xlarge",
    "replicas": 3,
    "disk_size": 120
  },
  "labels": {},
  "status": {
    "phase": "Ready",
    "adapters": [...]
  }
}
```

### Adapter Status Pattern

Resources report status through adapter-specific condition sets following Kubernetes conventions.

**Structure:**
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
      "updated_at": "2025-11-17T15:04:05Z",
    }
  ],
  "data": {}
}
```

**Note**: The `created_at` and `updated_at` fields in conditions are optional and typically set by the service.

**Condition Types:**
- `Ready` - Resource is operational
- `Available` - Resource is available for use
- `Progressing` - Resource is being modified
- Custom types defined by adapters

### List Response Pattern

All list endpoints return consistent pagination metadata:

```json
{
  "kind": "ClusterList",
  "page": 1,
  "size": 10,
  "total": 100,
  "items": [...]
}
```

**Pagination Parameters:**
- `?page=N` - Page number (default: 1)
- `?pageSize=N` - Items per page (default: 100)

**Search Parameters (clusters only):**
- `?search=name='cluster-name'` - Filter by name

## Development Workflow

### Prerequisites

Before running hyperfleet-api, ensure these prerequisites are installed. See [PREREQUISITES.md](./PREREQUISITES.md) for details.

- Go 1.24 or higher
- Podman
- PostgreSQL 13+
- Make

### Initial Setup

```bash
# 1. Generate OpenAPI code (must run first as pkg/api/openapi is required by go.mod)
make generate

# 2. Install dependencies
go mod download

# 3. Build the binary
make binary

# 4. Setup PostgreSQL database
make db/setup

# 5. Run database migrations
./hyperfleet-api migrate

# 6. Verify database schema
make db/login
psql -h localhost -U hyperfleet hyperfleet
\dt
```

**Note**: The `pkg/api/openapi/` directory is not tracked in git. You must run `make generate` after cloning or pulling changes to the OpenAPI specification.

### Running the Service

**Local development (no authentication):**
```bash
make run-no-auth
```

The service starts on `localhost:8000`:
- REST API: `http://localhost:8000/api/hyperfleet/v1/`
- OpenAPI spec: `http://localhost:8000/openapi`
- Swagger UI: `http://localhost:8000/openapi-ui`
- Health check: `http://localhost:8083/healthcheck`
- Metrics: `http://localhost:8080/metrics`

**Test the API:**
```bash
# Check API compatibility
curl http://localhost:8000/api/hyperfleet/v1/compatibility | jq

# List clusters
curl http://localhost:8000/api/hyperfleet/v1/clusters | jq

# Create a cluster
curl -X POST http://localhost:8000/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "Cluster",
    "name": "prod-cluster-1",
    "spec": {
      "region": "us-west-2",
      "version": "4.15",
      "nodes": 3
    },
    "labels": {
      "env": "production"
    }
  }' | jq
```

### Configuration

HyperFleet API can be configured via environment variables:

#### Schema Validation

**`OPENAPI_SCHEMA_PATH`**
- **Description**: Path to the OpenAPI specification file used for validating cluster and nodepool spec fields
- **Default**: `openapi/openapi.yaml` (repository base schema)
- **Required**: No (service will start with default schema if not specified)
- **Usage**:
  - **Local development**: Uses default repository schema
  - **Production**: Set via Helm deployment to inject provider-specific schema from ConfigMap

**Example:**
```bash
# Local development (uses default)
./hyperfleet-api serve

# Custom schema path
export OPENAPI_SCHEMA_PATH=/path/to/custom/openapi.yaml
./hyperfleet-api serve

# Production (Helm sets this automatically)
# OPENAPI_SCHEMA_PATH=/etc/hyperfleet/schemas/openapi.yaml
```

**How it works:**
1. The schema validator loads the OpenAPI specification at startup
2. When POST/PATCH requests are made to create or update resources, the `spec` field is validated against the schema
3. Invalid specs return HTTP 400 with detailed field-level error messages
4. Unknown resource types or missing schemas are gracefully handled (validation skipped)

**Provider-specific schemas:**
In production deployments, cloud providers can inject their own OpenAPI schemas via Helm:
```bash
helm install hyperfleet-api ./chart \
  --set-file provider.schema=gcp-schema.yaml
```

The injected schema is mounted at `/etc/hyperfleet/schemas/openapi.yaml` and automatically used for validation.

### Testing

```bash
# Unit tests
make test

# Integration tests (requires running database)
make test-integration
```

**Test Coverage:**

All 12 API endpoints have integration test coverage:

| Endpoint | Coverage |
|----------|----------|
| GET /compatibility | ✓ |
| GET /clusters | ✓ (list, pagination, search) |
| POST /clusters | ✓ |
| GET /clusters/{id} | ✓ |
| GET /clusters/{id}/statuses | ✓ |
| POST /clusters/{id}/statuses | ✓ |
| GET /nodepools | ✓ (list, pagination) |
| GET /clusters/{id}/nodepools | ✓ |
| POST /clusters/{id}/nodepools | ✓ |
| GET /clusters/{id}/nodepools/{nodepool_id} | ✓ |
| GET /clusters/{id}/nodepools/{nodepool_id}/statuses | ✓ |
| POST /clusters/{id}/nodepools/{nodepool_id}/statuses | ✓ |

## Code Generation Workflow

### TypeSpec to OpenAPI

The API specification is defined using TypeSpec and compiled to OpenAPI 3.0 from [hyperfleet-api-spec](https://github.com/openshift-hyperfleet/hyperfleet-api-spec):

```
TypeSpec definitions (.tsp files)
    ↓
tsp compile
    ↓
openapi/openapi.yaml (32KB, source specification)
```

### OpenAPI to Go Models

Generated Go code is created via Docker-based workflow:

```
openapi/openapi.yaml
    ↓
make generate (podman + openapi-generator-cli v7.16.0)
    ↓
pkg/api/openapi/model_*.go (Go model structs)
pkg/api/openapi/api/openapi.yaml (44KB, fully resolved spec)
```

**Generation process:**
1. `make generate` removes existing generated code
2. Builds Docker image with openapi-generator-cli
3. Runs code generator inside container
4. Copies generated files to host

**Generated artifacts:**
- Model structs with JSON tags
- Type definitions for all API resources
- Validation tags for required fields
- Fully resolved OpenAPI specification

**Important**: Generated files in `pkg/api/openapi/` are not tracked in git. Developers must run `make generate` after cloning or pulling changes to the OpenAPI specification.

### Runtime Embedding

The fully resolved OpenAPI specification is embedded at compile time using Go 1.16+ `//go:embed`:

```go
// pkg/api/openapi_embed.go
//go:embed openapi/api/openapi.yaml
var openapiFS embed.FS

func GetOpenAPISpec() ([]byte, error) {
    return fs.ReadFile(openapiFS, "openapi/api/openapi.yaml")
}
```

This embedded specification is:
- Compiled into the binary
- Served at `/openapi` endpoint
- Used by Swagger UI at `/openapi-ui`
- Zero runtime file I/O required

## Database Schema

### Core Tables

**clusters**
- Primary resources for cluster management
- Includes spec (region, version, nodes)
- Stores metadata (labels, generation)
- Tracks created_by, updated_by

**node_pools**
- Child resources owned by clusters
- Contains spec (instance_type, replicas, disk_size)
- Maintains owner_id foreign key to clusters
- Soft delete support

**adapter_statuses**
- Polymorphic status records
- owner_type: 'Cluster' or 'NodePool'
- owner_id: References clusters or node_pools
- Stores adapter name and conditions JSON
- Tracks observed_generation

**labels**
- Key-value pairs for resource categorization
- owner_type and owner_id for polymorphic relationships
- Supports filtering and search

## OpenAPI Specification Structure

**Source file (`openapi/openapi.yaml` - 32KB):**
- TypeSpec compilation output
- Uses `$ref` for parameter reuse (78 references)
- Compact, maintainable structure
- Input for code generation

**Generated file (`pkg/api/openapi/api/openapi.yaml` - 44KB):**
- openapi-generator output
- Fully resolved (no external `$ref`)
- Inline parameter definitions (54 references)
- Includes server configuration
- Embedded in Go binary

**Key differences:**
- Source file: Optimized for maintainability
- Generated file: Optimized for runtime serving

## Build Commands

```bash
# Generate OpenAPI client code
make generate

# Build binary
make binary

# Run database migrations
./hyperfleet-api migrate

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

## Container Image

Build and push container images using the multi-stage Dockerfile:

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

Default image: `quay.io/openshift-hyperfleet/hyperfleet-api:latest`

## Kubernetes Deployment

### Using Helm Chart

The project includes a Helm chart for Kubernetes deployment with configurable PostgreSQL support.

**Development deployment (with built-in PostgreSQL):**
```bash
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --create-namespace
```

**Production deployment (with external database like GCP Cloud SQL):**
```bash
# First, create a secret with database credentials
kubectl create secret generic hyperfleet-db-external \
  --namespace hyperfleet-system \
  --from-literal=db.host=<your-cloudsql-ip> \
  --from-literal=db.port=5432 \
  --from-literal=db.name=hyperfleet \
  --from-literal=db.user=hyperfleet \
  --from-literal=db.password=<your-password>

# Deploy with external database
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --set database.postgresql.enabled=false \
  --set database.external.enabled=true \
  --set database.external.secretName=hyperfleet-db-external
```

**Custom image deployment:**
```bash
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --set image.registry=quay.io/myuser \
  --set image.repository=hyperfleet-api \
  --set image.tag=v1.0.0
```

**Upgrade deployment:**
```bash
helm upgrade hyperfleet-api ./charts/ --namespace hyperfleet-system
```

**Uninstall:**
```bash
helm uninstall hyperfleet-api --namespace hyperfleet-system
```

### Helm Values

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.registry` | Container registry | `quay.io/openshift-hyperfleet` |
| `image.repository` | Image repository | `hyperfleet-api` |
| `image.tag` | Image tag | `latest` |
| `database.postgresql.enabled` | Deploy built-in PostgreSQL | `true` |
| `database.external.enabled` | Use external database | `false` |
| `database.external.secretName` | Secret with db credentials | `""` |
| `auth.enableJwt` | Enable JWT authentication | `true` |
| `auth.enableAuthz` | Enable authorization | `true` |

## API Authentication

**Development mode (no auth):**
```bash
make run-no-auth
curl http://localhost:8000/api/hyperfleet/v1/clusters
```

**Production mode (OCM auth):**
```bash
make run
ocm login --token=${OCM_ACCESS_TOKEN} --url=http://localhost:8000
ocm get /api/hyperfleet/v1/clusters
```

## Example Usage

### Create Cluster and NodePool

```bash
# 1. Create cluster
CLUSTER=$(curl -s -X POST http://localhost:8000/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "Cluster",
    "name": "production-cluster",
    "spec": {
      "region": "us-east-1",
      "version": "4.16",
      "nodes": 5
    },
    "labels": {
      "env": "production",
      "team": "platform"
    }
  }')

CLUSTER_ID=$(echo $CLUSTER | jq -r '.id')

# 2. Create node pool
curl -X POST http://localhost:8000/api/hyperfleet/v1/clusters/$CLUSTER_ID/nodepools \
  -H "Content-Type: application/json" \
  -d '{
    "kind": "NodePool",
    "name": "worker-pool",
    "spec": {
      "instance_type": "m5.2xlarge",
      "replicas": 10,
      "disk_size": 200
    },
    "labels": {
      "pool_type": "worker"
    }
  }' | jq

# 3. Report adapter status
curl -X POST http://localhost:8000/api/hyperfleet/v1/clusters/$CLUSTER_ID/statuses \
  -H "Content-Type: application/json" \
  -d '{
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
  }' | jq

# 4. Get cluster with aggregated status
curl http://localhost:8000/api/hyperfleet/v1/clusters/$CLUSTER_ID | jq
```

## License

[License information to be added]
