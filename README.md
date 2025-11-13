# HyperFleet API

HyperFleet API - Simple REST API for cluster lifecycle management. Provides CRUD operations for clusters and status sub-resources. Pure data layer with PostgreSQL integration - no business logic or event creation. Stateless design enables horizontal scaling. Part of HyperFleet v2 event-driven architecture.

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
* Event-driven architecture with advisory locks
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
  "adapter": "hive-adapter",
  "observed_generation": 1,
  "conditions": [
    {
      "adapter": "hive-adapter",
      "type": "Ready",
      "status": "True",
      "observed_generation": 1,
      "reason": "ClusterProvisioned",
      "message": "Cluster successfully provisioned"
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
- Podman or Docker
- PostgreSQL 13+
- Make

### Initial Setup

```bash
# 1. Install dependencies
go install gotest.tools/gotestsum@latest
go mod download

# 2. Build the binary
make binary

# 3. Setup PostgreSQL database
make db/setup

# 4. Run database migrations
./hyperfleet migrate

# 5. Verify database schema
make db/login
psql -h localhost -U hyperfleet hyperfleet
\dt
```

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

### Event System

- Advisory locks prevent concurrent processing
- Event channels notify on resource changes
- Idempotent event handlers
- Lock IDs based on resource operations

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
./hyperfleet migrate

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
    "adapter": "hive-adapter",
    "observed_generation": 1,
    "conditions": [
      {
        "adapter": "hive-adapter",
        "type": "Ready",
        "status": "True",
        "observed_generation": 1,
        "reason": "ClusterProvisioned",
        "message": "Cluster successfully provisioned"
      }
    ]
  }' | jq

# 4. Get cluster with aggregated status
curl http://localhost:8000/api/hyperfleet/v1/clusters/$CLUSTER_ID | jq
```

## License

[License information to be added]
