# HyperFleet API Operator Guide

A practical guide for deploying, configuring, and operating the HyperFleet API component.

---

## Table of Contents

1. [Introduction](#1-introduction)
2. [Concepts](#2-concepts)
   - [Resource Model](#21-resource-model)
      - [Core Resource Structure](#211-core-resource-structure)
      - [Resource Hierarchy](#212-resource-hierarchy)
   - [Adapter Registration](#22-adapter-registration)
   - [Status Aggregation](#23-status-aggregation)
      - [Resource conditions](#231-resource-conditions)
      - [Adapter condition types](#232-adapter-condition-types)
      - [Resource-level condition aggregation](#233-resource-level-condition-aggregation)
      - [Special handling for `Available=Unknown`](#234-special-handling-for-availableunknown)
   - [Generation](#24-generation)
3. [Configuration Reference](#3-configuration-reference)
   - [Adapter Requirements (REQUIRED)](#31-adapter-requirements-required)
   - [Database Configuration](#32-database-configuration)
   - [Authentication Configuration](#33-authentication-configuration)
   - [Server Binding](#34-server-binding)
   - [Logging Configuration](#35-logging-configuration)
   - [Schema Validation](#36-schema-validation)
4. [Deployment Checklist](#4-deployment-checklist)
5. [Additional Resources](#additional-resources)

**Appendices:**
- [Appendix A: API Integration](#appendix-a-api-integration)
- [Appendix B: Troubleshooting](#appendix-b-troubleshooting)

---

## 1. Introduction

The HyperFleet API is the **central data layer** within the HyperFleet system. As a stateless REST service, it stores resource specifications and aggregates status reported from distributed adapters. The API exposes REST endpoints for creating and managing resources, while serving as the **source of truth** that Sentinel and adapters poll for resource state.

**IMPORTANT:** The HyperFleet API is a **mandatory core component**. You cannot run HyperFleet without it. Deploy the API before deploying Sentinel, adapters.

**Key Benefits:**

- **Stateless design** - Horizontal scaling without coordination overhead
- **Provider-agnostic specs** - Store resource (e.g., cluster, nodepool) configurations independent of infrastructure provider
- **Status aggregation** - Unified resource status computed from distributed adapter reports
- **Generation tracking** - Coordinate spec changes across distributed adapters to ensure eventual consistency
- **Pure data layer** - No business logic or event generation—separation of concerns enables simpler operations

**Core Responsibilities:**

1. **CRUD operations** for resources (cluster, nodepool) with provider-agnostic specifications
2. **Status aggregation** from multiple adapters into unified resource conditions
3. **Generation tracking** to coordinate spec changes across distributed adapters
4. **Resource lifecycle management** with referential integrity between parent and child resources

The API does **not** contain business logic or event generation—those responsibilities belong to separate controllers (Sentinel, adapters). This separation enables horizontal scaling and simplifies the deployment model.

---

## 2. Concepts

### 2.1 Resource Model

The API stores and manages resources using a consistent data model. Understanding this model is essential for working with the API.

#### 2.1.1 Core Resource Structure

Every resource (cluster, nodepool) has these key fields:

```
Resource (e.g., Cluster)
├── id                    (32-character unique identifier, auto-generated)
├── kind                  (Resource "Cluster" or "NodePool")
├── name                  (Unique identifier)
├── spec                  (JSONB - desired state, provider-specific)
├── labels                (Key-value metadata for filtering)
├── generation            (Version counter, auto-incremented on spec changes)
├── status
│   └── conditions[]      (Aggregated state)
├── created_time
├── updated_time
├── created_by
└── updated_by
```

**Field Details:**

| Field | Type | Purpose | Managed By |
|-------|------|---------|------------|
| **spec** | JSONB | Desired state - what you want the resource to look like | User (via API) |
| **status** | JSONB | Observed state - what adapters report about the resource | API (aggregated from adapter reports) |
| **generation** | int32 | Version counter that increments when spec changes | API (automatic) |
| **labels** | JSONB | Key-value pairs for filtering and organization | User (via API) |

**How the Resource Model Works:**

1. **Desired State (spec)**: When you create or update a resource, you provide a `spec` containing the desired configuration (e.g., cluster region, version, node count). The API stores this without business-logic interpretation, but validates it against the OpenAPI schema when a schema is configured.

2. **Automatic Version Tracking (generation)**: Every time you update the `spec`, the API automatically increments the `generation` counter. This allows distributed adapters to detect when they need to reconcile infrastructure changes.

3. **Observed State (status)**: Adapters report their progress and results back to the API via status endpoints. The API aggregates these reports into unified resource-level conditions (e.g., `Ready`, `Available`).

4. **Filtering (labels)**: Labels are key-value pairs you can attach to resources for organization and filtering (e.g., `environment: production`, `region: us-east-1`). E.g., Sentinel instances can define resource selectors based on labels to watch specific subsets of resources, enabling horizontal scaling across multiple Sentinel deployments.

<details>
<summary><b>Resource Lifecycle Example</b> (click to expand)</summary>

The following example uses a cluster resource, but all resource types (clusters, nodepools) follow the same lifecycle pattern:

```bash
# 1. User creates cluster
POST /api/hyperfleet/v1/clusters
{
  "name": "my-cluster",
  "spec": {"region": "us-east-1", "version": "4.14"},
  "labels": {"environment": "production"}
}

→ API stores:
  - generation: 1 (initial)
  - status: {conditions: []} (empty, no adapter reports yet)

# 2. View cluster status
GET /api/hyperfleet/v1/clusters/{id}
{
  "id": "2opdkciuv7itslp5guihuhckhp8da0uo",
  "kind": "Cluster",
  "name": "my-cluster",
  "generation": 1,
  "spec": {
    "region": "us-east-1",
    "version": "4.14"
  },
  "labels": {
    "environment": "production"
  },
  "status": {
    "conditions": [
      {
        "type": "Available",
        "status": "True",
        "observed_generation": 1,
        "last_transition_time": "2026-03-10T07:56:35Z"
      },
      {
        "type": "Ready",
        "status": "True",
        "observed_generation": 1,
        "last_transition_time": "2026-03-10T07:56:35Z"
      }
    ]
  },
  "created_time": "2026-03-10T06:03:30Z",
  "updated_time": "2026-03-10T07:56:35Z"
}

→ API returns aggregated status with Available and Ready conditions

# 3. View adapter statuses
GET /api/hyperfleet/v1/clusters/{id}/statuses
{
  "items": [
    {
      "adapter": "validation",
      "observed_generation": 1,
      "conditions": [
        {
          "type": "Available",
          "status": "True",
          "reason": "ValidationPassed",
          "message": "Environment validated successfully"
        },
        {
          "type": "Applied",
          "status": "True",
          "reason": "JobApplied",
          "message": "Validation job applied successfully"
        },
        {
          "type": "Health",
          "status": "True",
          "reason": "Healthy",
          "message": "Adapter executed successfully"
        }
      ],
      "last_report_time": "2026-03-10T07:56:05Z"
    },
    {
      "adapter": "dns",
      "observed_generation": 1,
      "conditions": [
        {
          "type": "Available",
          "status": "True",
          "reason": "DnsReady",
          "message": "DNS records created successfully"
        },
        {
          "type": "Applied",
          "status": "True",
          "reason": "RecordsCreated",
          "message": "DNS configuration applied"
        },
        {
          "type": "Health",
          "status": "True",
          "reason": "Healthy",
          "message": "Adapter executed successfully"
        }
      ],
      "last_report_time": "2026-03-10T07:56:05Z"
    }
  ],
  "total": 2
}

→ API returns individual adapter status reports
```

</details>

#### 2.1.2 Resource Hierarchy

The API supports hierarchical resource structures where resources can have parent-child relationships. Currently, clusters can have child nodepools:

```
Cluster
├── spec, status, labels, generation
└── NodePools (children)
    ├── spec, status, labels, generation
    └── owner_references → Parent Cluster
```

**Key aspects:**

- **Nested API paths**: NodePools are created under their parent cluster using `/clusters/{cluster-id}/nodepools`
- **Parent reference**: NodePools store a reference to their parent cluster via the `owner_references` field
- **Same structure**: NodePools have the same field structure as clusters (spec, status, labels, generation)
- **Consistent status model**: NodePool status and adapter statuses work the same way as cluster resources

<details>
<summary><b>NodePool Creation Example</b> (click to expand)</summary>

Create a nodepool belonging to a cluster by specifying the cluster ID in the API path:

```bash
# Create nodepool under a cluster
POST /api/hyperfleet/v1/clusters/{cluster-id}/nodepools
{
  "kind": "NodePool",
  "name": "nodepool-workers",
  "labels": {
    "workload": "gpu",
    "tier": "compute"
  },
  "spec": {
    "replicas": 2,
    "machineType": "n1-standard-8"
  }
}

# View nodepool status (same structure as cluster status)
GET /api/hyperfleet/v1/clusters/{cluster-id}/nodepools/{nodepool-id}

# View nodepool adapter statuses (same structure as cluster adapter statuses)
GET /api/hyperfleet/v1/clusters/{cluster-id}/nodepools/{nodepool-id}/statuses
```

The nodepool resource structure and status aggregation work identically to clusters - the only difference is the API path includes the parent cluster ID.

</details>

### 2.2 Adapter Registration

**How registration works:** You define which adapters are required for each resource type to be marked as `Ready`. Adapters are registered at API startup via environment variables or Helm configuration, and the API will not start if this configuration is missing.

Only **registered adapters** participate in status aggregation:
- The `Ready` condition checks if all registered adapters report `Available=True` at the current `resource.spec.generation`
- Unregistered adapters can report status, but don't affect resource readiness
- Changing registered adapters requires an API restart (configuration is read at startup)

**Configuration:**

Currently, the API supports cluster and nodepool resource types:

```bash
# Required cluster adapters (example - adjust to match your deployment)
HYPERFLEET_CLUSTER_ADAPTERS='["cluster-validation","dns","pullsecret","hypershift"]'

# Required nodepool adapters (example - adjust to match your deployment)
HYPERFLEET_NODEPOOL_ADAPTERS='["nodepool-validation","hypershift"]'
```

When using Helm (recommended):

```yaml
adapters:
  cluster:   # Example adapter names - adjust to match your deployment
    - cluster-validation
    - dns
    - pullsecret
    - hypershift
  nodepool:  # Example adapter names - adjust to match your deployment
    - nodepool-validation
    - hypershift
```

### 2.3 Status Aggregation

**What is status aggregation?** Instead of checking each adapter individually, the API combines (aggregates) their status reports into a single resource-level status. This gives you a simple "Ready" or "Not Ready" answer that considers all registered adapters together.

The API aggregates individual adapter reports into resource-level conditions.

#### 2.3.1 Resource conditions

Every resource has these synthesized conditions:

| Condition | Meaning | When True |
|-----------|---------|-----------|
| **Available** | Resource is operational at any known good configuration | All registered adapters report `Available=True` (at any generation) |
| **Ready** | Resource is fully reconciled at current spec | All registered adapters report `Available=True` at the **current** `resource.spec.generation` |

#### 2.3.2 Adapter condition types

Adapters report three mandatory condition types:

| Type | Meaning |
|------|---------|
| **Available** | Adapter's work is complete and operational |
| **Applied** | Kubernetes/Maestro resources were created/applied |
| **Health** | Adapter framework executed without errors |

**Validation rules** (enforced by API):

- All three condition types must be present
- No duplicate condition types allowed
- `status` must be `True`, `False`, or `Unknown`
- `observed_generation` must be a valid integer

#### 2.3.3 Resource-level condition aggregation

The API synthesizes two resource-level conditions (`Available` and `Ready`) from adapter reports. Each condition is a complete state with both `status` and `observed_generation` working together:

**Available condition states:**

| Status | observed_generation | Meaning |
|--------|-------------------|---------|
| `True` | Min across all adapters with `Available=True` | All required adapters are operational, resource is running at generation N (last known good generation). N may be less than `resource.spec.generation` during reconciliation of new spec changes. |
| `False` | `0` | At least one required adapter has `Available!=True` or hasn't reported. Resource is not operational. |
| `False` | `1` | Initial state: no adapters have reported OR no adapters are required. |

**Why Available tracks minimum generation:** When a spec is updated (e.g., `generation=5` → `generation=6`), adapters reconcile at different speeds. If all adapters continue reporting `Available=True` (even at different generations), the resource-level Available remains `True` with `observed_generation=5` (the minimum generation across all Available=True adapters). This preserves the "last known good configuration" while adapters work through the new spec. If any adapter reports `Available=False` during reconciliation, the resource becomes `Available=False, observed_generation=0`.

**Ready condition states:**

| Status | observed_generation | Meaning |
|--------|-------------------|---------|
| `True` | Current `resource.spec.generation` | All required adapters report `Available=True` at the current generation. Resource is fully reconciled. |
| `False` | Current `resource.spec.generation` | At least one adapter hasn't caught up to the current generation OR has `Available!=True`. |

**Why Ready always uses current generation:** Ready always sets `observed_generation = resource.spec.generation`, regardless of status. This indicates whether adapters have finished reconciling the current spec.

#### 2.3.4 Special handling for `Available=Unknown`

The API handles `Available=Unknown` status reports differently based on whether the adapter has reported before:

**When `Available=Unknown` is reported for the first time** (no prior status from this adapter):
- ✅ Accepts and stores the adapter status with `Available=Unknown`
- Returns `201 Created` with the stored status object
- Resource-level conditions (`Available`, `Ready`) remain unchanged

**When `Available=Unknown` is reported again** (adapter has reported before):
- ❌ Discards the status (not stored)
- Returns `204 No Content` (no action taken)

**When `Available=True` or `Available=False` is reported** (any time):
- ✅ If the reported generation is newer than or equal to the existing stored generation:
  - Stored and triggers status aggregation
  - Updates resource-level `Available` and `Ready` conditions
  - Returns `201 Created` with the stored status object
- ❌ If the reported generation is older than the existing stored generation:
  - Discarded (stale update, not stored)
  - Returns `204 No Content`

This pattern allows adapters to signal initial progress without affecting resource readiness, while ensuring eventual convergence to a definitive state (`True` or `False`).

### 2.4 Generation

Generation is a version number that the API automatically increments every time a resource's desired state (`spec`) changes. It provides a way to track which version of the spec is being processed.

**IMPORTANT - No optimistic locking:** Unlike Kubernetes-style APIs, HyperFleet API does **not** implement optimistic locking with generation. You cannot provide generation in update requests, and the API does not verify generation matches before applying updates. The API uses a "last write wins" model where concurrent updates to the same resource may overwrite each other. Generation is purely a tracking mechanism for adapters to detect spec changes.

**How the API manages generation:**

- **Initial creation**: Sets `generation=1`
- **Spec updates**: Automatically increments `generation++` when comparing old vs. new spec
- **Non-spec updates**: Labels or status changes do **not** increment generation
- **Read-only for users**: Generation is managed entirely by the API, you cannot set it manually

**How generation is used in status aggregation:**

The API uses generation to determine if a resource is `Ready`:

```
Ready = True  when:  resource.generation == adapter.observed_generation
                     AND all registered adapters report Available=True

Ready = False when:  resource.generation > adapter.observed_generation
                     (spec changed but not all adapters have processed it yet)
```

---

## 3. Configuration Reference

This section highlights the **critical configuration** needed to operate the API.

For comprehensive guides on specific topics:
- **Environment variables**: See [Deployment Guide - Environment Variables](deployment.md#environment-variables)
- **Database setup**: See [Database Guide](database.md)
- **Authentication**: See [Authentication Guide](authentication.md)
- **Helm chart values**: See [Deployment Guide](deployment.md)

### 3.1 Adapter Requirements (REQUIRED)

The API will not start without these variables. **Note:** The adapter names shown below are examples - you must configure them to match the adapters actually deployed in your environment.

```bash
# Example configuration - adjust adapter names to match your deployment
HYPERFLEET_CLUSTER_ADAPTERS='["cluster-validation","dns","pullsecret","hypershift"]'
HYPERFLEET_NODEPOOL_ADAPTERS='["nodepool-validation","hypershift"]'
```

**Helm equivalent**:

```yaml
adapters:
  cluster:   # Example adapter names - adjust to match your deployment
    - cluster-validation
    - dns
    - pullsecret
    - hypershift
  nodepool:  # Example adapter names - adjust to match your deployment
    - nodepool-validation
    - hypershift
```

### 3.2 Database Configuration

The API requires PostgreSQL 13 or later.

**Helm deployment:**

Configure database connection using Helm values:

```yaml
database:
  external:
    enabled: true
    secretName: hyperfleet-db-prod  # Secret containing database credentials
```

The secret must contain these fields:

```
db.host         → PostgreSQL hostname
db.port         → PostgreSQL port
db.name         → Database name
db.user         → Database username
db.password     → Database password
db.rootcert     → (Optional) SSL root certificate
```

**SSL configuration:**

Use the `--db-sslmode` flag when running the binary directly:

```bash
./hyperfleet-api serve --db-sslmode=verify-full
```

| Mode | Description |
|------|-------------|
| `disable` | No SSL (development only) |
| `require` | SSL required, no cert verification |
| `verify-ca` | Verify server cert against CA |
| `verify-full` | Verify cert and hostname (recommended for production) |

### 3.3 Authentication Configuration

**What is JWT authentication?** JWT (JSON Web Token) is a secure way to verify that API requests come from authorized users or services. In production, the API validates tokens to ensure only authenticated clients can create or modify clusters.

**Helm deployment:**

```yaml
# Development (no authentication)
auth:
  enableJwt: false
  enableAuthz: false

# Production (with JWT authentication)
auth:
  enableJwt: true
  enableAuthz: true
  jwksUrl: https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/certs
```

**Direct binary execution:**

```bash
# Development (no authentication)
./hyperfleet-api serve --enable-jwt=false --enable-authz=false

# Production (with JWT authentication)
./hyperfleet-api serve \
  --enable-jwt=true \
  --enable-authz=true \
  --jwk-cert-url=https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/certs
```

See [Authentication Guide](authentication.md) for detailed JWT setup.

### 3.4 Server Binding

The API runs three independent servers on different ports:
- **API server** (built-in default: `localhost:8000`) - REST endpoints
- **Health server** (built-in default: `localhost:8080`) - Liveness/readiness probes
- **Metrics server** (built-in default: `localhost:9090`) - Prometheus metrics

**Helm deployment:**

The Helm chart overrides the built-in defaults to bind to all interfaces (required for Kubernetes):

```yaml
server:
  bindAddress: ":8000"          # Overrides localhost:8000
  healthBindAddress: ":8080"    # Overrides localhost:8080
  metricsBindAddress: ":9090"   # Overrides localhost:9090
```

**Direct binary execution:**

The built-in defaults (`localhost:*`) bind to loopback only. For production deployments or to make the API accessible from outside the host, bind to all interfaces:

```bash
./hyperfleet-api serve \
  --api-server-bindaddress=:8000 \
  --health-server-bindaddress=:8080 \
  --metrics-server-bindaddress=:9090
```

**Note:** Use `:PORT` format to bind to all interfaces (0.0.0.0), or `localhost:PORT` for local-only binding (127.0.0.1). 

### 3.5 Logging Configuration

```bash
LOG_LEVEL=info            # debug | info | warn | error
LOG_FORMAT=json           # json | text
LOG_OUTPUT=stdout         # stdout | stderr
```

Production should use `LOG_LEVEL=info` and `LOG_FORMAT=json` for structured logging.

### 3.6 Schema Validation

The API validates resource `spec` fields (e.g., cluster, nodepool) against an OpenAPI schema when configured. This allows different providers (GCP, AWS, Azure) to enforce different spec structures.

**Configuration:** `server.openapi_schema_path`

- **Environment variable:** `HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH=/etc/hyperfleet/schemas/openapi.yaml`
- **Config file:** `server.openapi_schema_path: /etc/hyperfleet/schemas/openapi.yaml`
- **Default:** `openapi/openapi.yaml` (provider-agnostic base schema)

**How validation works:**

The API uses a two-step process to validate specs:

1. **Resource type detection** — The validation middleware inspects the request URL path to determine the resource type:
   - Paths containing `/nodepools` → validated as `nodepool`
   - Paths containing `/clusters` → validated as `cluster`

2. **Schema lookup** — The validator maps each resource type to a specific OpenAPI schema component:
   - `cluster` → looks for `ClusterSpec` in the OpenAPI schema's `components.schemas`
   - `nodepool` → looks for `NodePoolSpec` in the OpenAPI schema's `components.schemas`

**Current limitations:**

- Resource types and schema mappings are currently hardcoded for `cluster` and `nodepool`
- Adding new resource types requires code changes in both the validation middleware and the schema validator
- This design may be generalized in the future to support dynamic resource type registration

**Behavior:**

- **Schema configured and valid**: Specs are validated against the OpenAPI schema. Invalid specs return `400 Bad Request`.
- **Schema missing or invalid**: API logs a warning and starts without validation. Specs are stored without schema validation.
- Startup is **non-blocking** — missing or invalid schema files do not prevent API startup

---

## 4. Deployment Checklist

For detailed Helm commands and configuration, see the [Deployment Guide](deployment.md).

Follow this checklist to ensure successful API deployment and operation.

### Phase 1: Database Preparation

Choose your database deployment strategy based on your environment:

**Option A: Development (Built-in PostgreSQL)**

- [ ] Skip database provisioning - built-in PostgreSQL will be created automatically
- [ ] **Note:** Built-in PostgreSQL is **not suitable for production** (single replica, no backups)

**Option B: Production (External Database)**

- [ ] Ensure PostgreSQL 13+ database server is running and accessible from Kubernetes cluster
- [ ] Create database and user with appropriate permissions:
  ```sql
  CREATE DATABASE hyperfleet;
  CREATE USER hyperfleet WITH PASSWORD '<strong-password>';
  GRANT ALL PRIVILEGES ON DATABASE hyperfleet TO hyperfleet;
  GRANT ALL ON SCHEMA public TO hyperfleet;
  ```

- [ ] Create Kubernetes secret with database credentials:
  ```bash
  kubectl create secret generic hyperfleet-db \
    --namespace hyperfleet-system \
    --from-literal=db.host=postgres.example.com \
    --from-literal=db.port=5432 \
    --from-literal=db.name=hyperfleet \
    --from-literal=db.user=hyperfleet \
    --from-literal=db.password=<strong-password>
  ```

- [ ] Verify secret was created correctly:
  ```bash
  kubectl get secret hyperfleet-db -n hyperfleet-system -o json | jq '.data | keys'
  ```
  Expected output: `["db.host", "db.name", "db.password", "db.port", "db.user"]`

- [ ] Test database connectivity:
  ```bash
  kubectl run pg-debug --rm -it --image=postgres:15-alpine \
    --restart=Never -n hyperfleet-system -- \
    psql -h <db-host> -U hyperfleet -d hyperfleet -c "SELECT 1"
  ```
  Expected output: `?column? | 1`

### Phase 2: Configuration Planning

**Define Adapter Requirements**

- [ ] List required adapters for each resource type (see [Adapter Registration](#22-adapter-registration))
  - **Example** cluster adapters: `cluster-validation`, `dns`, `pullsecret`, `hypershift`
  - **Example** nodepool adapters: `nodepool-validation`, `hypershift`
  - **Note:** These are example adapter names - configure to match your actual deployment
- [ ] Confirm all adapters are deployed or will be deployed alongside the API

**Prepare Helm Values File**

- [ ] Create `custom-values.yaml` based on your requirements
- [ ] Review and adjust configuration:
  - Adapter lists match your deployment
  - Database secret name matches (if using external database)
  - Resource limits appropriate for your cluster size
  - Authentication settings match your requirements

### Phase 3: Deployment

**Install HyperFleet API**

- [ ] Deploy using Helm:
  ```bash
  helm install hyperfleet-api ./charts/ \
    --namespace hyperfleet-system \
    --create-namespace \
    --values custom-values.yaml
  ```

- [ ] Verify deployment was created:
  ```bash
  kubectl get deployment -n hyperfleet-system hyperfleet-api
  ```

- [ ] Wait for pods to be ready:
  ```bash
  kubectl wait --for=condition=Ready pod -l app=hyperfleet-api -n hyperfleet-system --timeout=300s
  ```

**Verify Database Migration**

- [ ] Check init container logs to confirm migration completed:
  ```bash
  kubectl logs -n hyperfleet-system <pod-name> -c db-migrate
  ```
  Expected output: `Migration completed successfully`

- [ ] Verify database tables were created:
  ```bash
  kubectl run pg-debug --rm -it --image=postgres:15-alpine \
    --restart=Never -n hyperfleet-system -- \
    psql -h <db-host> -U hyperfleet -d hyperfleet -c "\dt"
  ```
  Expected tables: `adapter_statuses`, `clusters`, `labels`, `node_pools`

### Phase 4: Post-Deployment Validation

**Verify Service Health**

- [ ] Check health endpoint: `curl http://<hyperfleet-api-service>:8080/healthz`
- [ ] Check readiness endpoint: `curl http://<hyperfleet-api-service>:8080/readyz`
  - Expected: `{"status": "ok"}` for both endpoints
- [ ] Review pod logs for startup errors:
  ```bash
  kubectl logs -n hyperfleet-system -l app=hyperfleet-api
  ```

<details>
<summary><b>Run Smoke Tests (Optional)</b></summary>

- [ ] Create a test cluster:
  ```bash
  curl -X POST http://<hyperfleet-api-service>:8000/api/hyperfleet/v1/clusters \
    -H "Content-Type: application/json" \
    -d '{
      "kind": "Cluster",
      "name": "test-cluster",
      "spec": {},
      "labels": {"environment": "test"}
    }'
  ```
  Expected: `201 Created` with cluster object including `id` and `generation: 1`

- [ ] Save cluster ID and retrieve the cluster:
  ```bash
  CLUSTER_ID=<id-from-response>
  curl http://<hyperfleet-api-service>:8000/api/hyperfleet/v1/clusters/$CLUSTER_ID
  ```
  Expected: `200 OK` with cluster object

</details>

---

## Additional Resources

### Documentation

- [Deployment Guide](deployment.md) — Helm deployment, configuration, production setup
- [Operational Runbook](runbook.md) — Health checks, troubleshooting, recovery procedures
- [API Resources](api-resources.md) — Detailed endpoint reference, request/response formats
- [Metrics Documentation](metrics.md) — Complete Prometheus metrics catalog
- [Authentication Guide](authentication.md) — JWT setup and configuration
- [Database Guide](database.md) — Schema, migrations, connection pooling
- [Development Guide](development.md) — Local development, testing, code generation

---

## Appendix A: API Integration

This section covers how to integrate with the HyperFleet API, both as a **consumer** (creating/managing clusters) and as an **adapter developer** (reporting status).

For detailed documentation on specific integration topics:
- **API endpoints and request/response formats**: See [API Resources](api-resources.md)
- **Authentication setup**: See [Authentication Guide](authentication.md)
- **Metrics and monitoring**: See [Metrics Documentation](metrics.md)

### For API Consumers

#### Authentication

**Development (no auth):**

If `auth.enableJwt=false`, no authentication is required:

```bash
curl http://api-host:8000/api/hyperfleet/v1/clusters
```

**Production (JWT authentication):**

Make authenticated requests with a JWT token:

```bash
# Make authenticated request
curl http://api-host:8000/api/hyperfleet/v1/clusters \
  -H "Authorization: Bearer $JWT_TOKEN"
```

For details on JWT setup, see [Authentication Guide](authentication.md).

#### Rate Limiting Considerations

The API does not currently enforce rate limits, but clients should implement:

- **Exponential backoff** on retries (5xx errors, conflicts)
- **Polling intervals** of at least 5-10 seconds for status checks

### For Adapter Developers

Adapters are worker components that perform specific tasks (e.g., validation, DNS setup, infrastructure provisioning) and report their status back to the API.

#### Reporting Status Requirements

Every adapter status report must include:

- **adapter** (string) — If this adapter is required for cluster resources, must match a value in `HYPERFLEET_CLUSTER_ADAPTERS`. If required for nodepool resources, must match a value in `HYPERFLEET_NODEPOOL_ADAPTERS`
- **observed_generation** (integer) — The generation the adapter processed
- **observed_time** (RFC3339 timestamp) — When the adapter completed its work
- **conditions** (array) — Exactly three condition types:
   - `Available` — Is the work complete and operational?
   - `Applied` — Were Kubernetes resources created/configured?
   - `Health` — Did the adapter execute without errors?

**Optional field:**

- **data** (JSONB) — Optional adapter-specific information for debugging or operational dashboards:
   - Not used in status aggregation (API only reads `conditions`)
   - Can contain any valid JSON structure
   - Persisted in `adapter_statuses.data` column

<details>
<summary><b>Status Report Example</b></summary>

```json
{
  "adapter": "validation",
  "observed_generation": 5,
  "observed_time": "2025-01-15T10:30:00Z",
  "conditions": [
    {
      "type": "Available",
      "status": "True",
      "reason": "ValidationPassed",
      "message": "All cluster validations passed"
    },
    {
      "type": "Applied",
      "status": "True",
      "reason": "ResourcesCreated",
      "message": "Validation job created in namespace validation-system"
    },
    {
      "type": "Health",
      "status": "True",
      "reason": "ExecutionSucceeded",
      "message": "Adapter executed without errors"
    }
  ],
  "data": {
    "job_name": "validation-abc123",
    "validation_results": {
      "checks_passed": 15,
      "checks_failed": 0
    }
  }
}
```

</details>

#### Endpoint URLs

**Cluster status:**

```
POST /api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**NodePool status:**

```
POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses
```

---

## Appendix B: Troubleshooting

For comprehensive operational procedures, see the [Operational Runbook](runbook.md).

This section provides a **quick reference** for common API-specific issues and their solutions.

| Symptom | Likely Cause | Solution                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
|---------|--------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **API won't start: `HYPERFLEET_CLUSTER_ADAPTERS environment variable is required`** | Required environment variables not set | Verify Helm values: `helm get values hyperfleet-api -n hyperfleet-system`. If missing, configure adapter requirements in your Helm values file (see [Adapter Requirements](#31-adapter-requirements-required)) and upgrade the release.                                                                                                                                                                                                                                                                                                                                                        |
| **Adapters report status but resource remains `Ready=False`** | Adapter name mismatch, missing conditions, or generation mismatch | Check adapter names match registration: `kubectl exec -n hyperfleet-system deployment/hyperfleet-api -- env \| grep HYPERFLEET_CLUSTER_ADAPTERS` and compare with `curl http://<api-service>:8000/api/hyperfleet/v1/clusters/$CLUSTER_ID/statuses`. Verify all conditions present: `curl http://<api-service>:8000/api/hyperfleet/v1/clusters/$CLUSTER_ID/statuses \| jq '.items[] \| {adapter, conditions: [.conditions[].type]}'`. Check generation: `curl -s http://<api-service>:8000/api/hyperfleet/v1/clusters/$CLUSTER_ID/statuses \| jq ".items[] \| {adapter, observed_generation}"`. |
| **Pods stuck in init phase, migration fails: `permission denied for schema public`** | Database user lacks schema permissions | Grant permissions: `GRANT ALL ON SCHEMA public TO hyperfleet; GRANT ALL ON ALL TABLES IN SCHEMA public TO hyperfleet;`                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| **Pods stuck in init phase: `database does not exist`** | Database not created | Create database: `CREATE DATABASE hyperfleet;`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| **Pods stuck in init phase: `connection timeout`** | Database connection retry settings too low | Increase database connection retry settings using `--db-conn-retry-attempts` and `--db-conn-retry-interval` flags in the init container command                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| **High API latency, slow responses** | Resource limits, database slow queries, or connection pool exhausted | Check metrics: `curl http://<api-service>:9090/metrics \| grep hyperfleet_api_request_duration_seconds`. Check resources: `kubectl top pods -n hyperfleet-system`. Check slow queries: `kubectl logs -n hyperfleet-system deployment/hyperfleet-api \| grep "slow query"`. Resolution: Increase resource limits/replicas, add database indexes, or increase `--db-max-open-connections` (default: 50).                                                                                                                                                                                         |
| **400 Bad Request** | Resource spec doesn't match OpenAPI schema | Check loaded schema: `kubectl logs -n hyperfleet-system deployment/hyperfleet-api \| grep "OPENAPI_SCHEMA_PATH"`. Retrieve schema: `kubectl exec -n hyperfleet-system deployment/hyperfleet-api -- cat /etc/hyperfleet/schemas/openapi.yaml`. Validate and fix spec.                                                                                                                                                                                                                                                                                                                           |
| **401 Unauthorized** | Missing or invalid JWT token | Verify authentication is enabled (`auth.enableJwt=true`). If production, ensure valid JWT token is provided. Reference: [Authentication Guide](authentication.md).                                                                                                                                                                                                                                                                                                                                                                                                                             |
| **404 Not Found** | Resource doesn't exist | Verify resource ID is correct. Check if resource was deleted: `curl http://<api-service>:8000/api/hyperfleet/v1/clusters/$CLUSTER_ID`.                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| **409 Conflict** | Concurrent update or generation mismatch | Retry with exponential backoff. Ensure only one controller updates the same resource.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| **500 Internal Server Error** | Database error or unexpected panic | Check API logs: `kubectl logs -n hyperfleet-system -l app=hyperfleet-api --tail=100`. Verify database connectivity with `/readyz` endpoint.                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| **503 Service Unavailable** | Readiness probe failing | Check readiness: `curl http://<api-service>:8080/readyz`. Verify database connectivity and API initialization. Check logs for startup errors.                                                                                                                                                                                                                                                                                                                                                                                                                                                  |

---
