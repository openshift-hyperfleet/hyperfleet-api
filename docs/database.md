# Database

This document describes the database architecture used by HyperFleet API.

## Overview

HyperFleet API uses PostgreSQL with GORM ORM. The schema follows a simple relational model with polymorphic associations.

## Core Tables

### clusters
Primary resources for cluster management. It contains:
* cluster metadata,
* a JSONB `spec` field for provider-specific configuration,
* a JSONB `labels` field for key-value categorization,
* a JSONB `status_conditions` field for synthesized status,
* `deleted_time` for soft delete,
* and `deleted_by` for audit.

### node_pools
Child resources owned by clusters, representing groups of compute nodes. References clusters via `owner_id` with a `RESTRICT` foreign key. Shares the same core columns as clusters (`labels`, `status_conditions`, `deleted_time`, `deleted_by`) plus `owner_id` for the parent relationship.

### adapter_statuses
Polymorphic status records for both clusters and node pools. Stores adapter-reported conditions in JSONB format. No soft delete — rows are hard-deleted or replaced.

**Polymorphic pattern:**
- `resource_type` + `resource_id` allows one table to serve both clusters and node pools
- Unique constraint on `(resource_type, resource_id, adapter)` — one record per adapter per resource

### resources
Generic resource table used by the plugin system for extensible resource types (WifConfigs, Channels, Versions, etc.). Stores `kind`, `name`, `spec` (JSONB), `labels` (JSONB), and optional owner references (`owner_id`, `owner_kind`, `owner_href`) for parent-child relationships. Uses `deleted_time`/`deleted_by` for soft delete. Unique name constraints are scoped by `kind` and `owner_id`.

## Schema Relationships

```text
clusters (1) ──→ (N) node_pools
    │                    │
    │                    │
    └────────┬───────────┘
             │
             └──→ adapter_statuses (polymorphic via resource_type + resource_id)

resources (standalone, self-referencing parent-child via owner_id)
```

## Key Design Patterns

### JSONB Fields

Flexible schema storage for:
- **spec** - Provider-specific cluster/nodepool configurations
- **conditions** - Adapter status condition arrays
- **data** - Adapter metadata

**Benefits:**
- Support multiple cloud providers without schema changes
- Runtime validation against OpenAPI schema
- PostgreSQL JSON query capabilities

### Delete Lifecycle

Resources follow a three-phase delete lifecycle:

```text
Active ──(DELETE)──▶ Finalizing ──(adapters report Finalized=True)──▶ Hard-Deleted
                         │
                         └──(POST /force-delete)──▶ Hard-Deleted
```

1. **Active** — Normal state. Resource is visible in list queries and can be updated.
2. **Finalizing** (soft-deleted) — `DELETE` sets `deleted_time` and `deleted_by`, increments `generation`. The resource stays in the database so adapters can observe the deletion and clean up external state. Soft-deleted records are excluded from list queries by default. Creating new child resources under a finalizing parent is rejected with `409 Conflict`.
3. **Hard-Deleted** — Permanently removed from the database. This happens automatically when all required adapters report `Finalized=True` at the current generation. If adapters are stuck, `POST .../force-delete` bypasses the adapter gating and hard-deletes immediately — but the resource must already be in Finalizing state; calling force-delete on an active resource returns `409 Conflict`. Repeated force-delete calls after hard-deletion return `404 Not Found`. Cluster force-delete cascades to all child NodePools and their adapter statuses. NodePool force-delete only removes the NodePool and its adapter statuses.

Adapter statuses do not use soft delete — they are hard-deleted when their parent resource is hard-deleted.

### Delete Policies

Generic resources (the `resources` table) use delete policies to control child behavior when a parent is deleted. Each resource type declares its policy:

| Policy     | Behavior |
|------------|----------|
| `restrict` | Parent delete is rejected with `409 Conflict` if active children exist |
| `cascade`  | All children are soft-deleted (marked Finalizing) along with the parent |

Policies are enforced recursively — a cascade on a parent triggers policy checks on grandchildren. For clusters and nodepools, the cascade is built-in: deleting a cluster always cascades the soft-delete to all its nodepools.

Resources without required adapters skip the Finalizing phase entirely — they are hard-deleted immediately on `DELETE`.

### Migration System

Migrations are:
- Non-destructive (never drops columns or tables)
- Additive (creates missing tables, columns, indexes)
- Run via `./bin/hyperfleet-api migrate`

### Migration Coordination

During rolling deployments, multiple pods may attempt to run migrations simultaneously. The API uses PostgreSQL advisory locks to ensure only one pod runs migrations at a time — other pods wait (up to 5 minutes) until the lock is released. Locks are automatically cleaned up on transaction commit or pod crash, so no manual intervention is needed.

## Database Setup

```bash
# Create PostgreSQL container
make db/setup

# Run migrations
./bin/hyperfleet-api migrate

# Connect to database
make db/login
```

See [development.md](development.md) for detailed setup instructions.

## Transaction Strategy

- **Write operations** (POST/PUT/PATCH/DELETE) run inside a database transaction with automatic commit on success and rollback on error.
- **Read operations** (GET) run without a transaction for lower latency and reduced connection pool pressure.

### Pagination note

Because list queries run without a transaction, the `total` count and the returned `items` are computed in separate statements. Under concurrent deletes, `total` may briefly exceed the actual number of items returned. This is a cosmetic pagination artifact, not a data integrity issue.

## Connection Pool Configuration

The connection pool is configured via CLI flags:

| Flag | Default | Description |
|------|---------|-------------|
| `--db-max-open-connections` | 50 | Maximum open connections to the database |
| `--db-max-idle-connections` | 10 | Maximum idle connections retained in the pool |
| `--db-conn-max-lifetime` | 5m | Maximum time a connection can be reused before being closed |
| `--db-conn-max-idle-time` | 1m | Maximum time a connection can sit idle before being closed |
| `--db-request-timeout` | 30s | Context deadline applied to each HTTP request's database transaction |
| `--db-conn-retry-attempts` | 10 | Retry attempts for initial database connection on startup |
| `--db-conn-retry-interval` | 3s | Wait time between connection retry attempts |

### Request Timeout

Every API request that touches the database gets a context deadline via `--db-request-timeout`. If a request cannot acquire a connection or complete its query within this window, it fails with a `500` and the connection is released. This prevents requests from hanging indefinitely when the pool is exhausted under load.

> **Note:** Under heavy load you may see `500` responses caused by the request timeout. This is expected backpressure behavior — the API deliberately fails fast rather than letting requests queue indefinitely. Clients (e.g., adapters) should treat these as transient errors and retry with backoff. If `500` rates are sustained, consider scaling the deployment or tuning `--db-max-open-connections` and `--db-request-timeout`.

### Connection Retries

On startup the API retries the database connection up to `--db-conn-retry-attempts` times. This handles sidecar startup races (e.g., pgbouncer may not be listening when the API container starts). Retries are logged at WARN level with attempt counts.

### Health Check Timeout

The readiness probe (`/readyz`) pings the database with a separate timeout controlled by `--health-db-ping-timeout` (default 2s). This ensures health checks respond quickly even when the main connection pool is under pressure, preventing Kubernetes from removing the pod from service endpoints due to slow readiness responses during load spikes. The default is set below the Kubernetes readiness probe `timeoutSeconds` (3s) so the Go-level timeout fires first and returns a proper 503 rather than a connection timeout.

## Sidecar Containers

The Helm chart supports two sidecar injection mechanisms:

| List | Where it renders | Starts when | Use case |
|------|------------------|-------------|----------|
| `nativeSidecars` | `initContainers` (with `restartPolicy: Always`) | Before all other init containers | Database proxies that must be available during `db-migrate` |
| `sidecars` | `containers` | After all init containers complete | Log shippers, monitoring agents, connection poolers that don't need to run during init |

Each entry in either list is a full Kubernetes container spec injected as-is into the deployment pod.

### Native Sidecars (`nativeSidecars`)

Kubernetes 1.28+ supports [native sidecar containers](https://kubernetes.io/docs/concepts/workloads/pods/sidecar-containers/) — init containers declared with `restartPolicy: Always`. They start before other init containers and keep running throughout the pod lifecycle.

Use `nativeSidecars` for database proxies (Cloud SQL Auth Proxy, AlloyDB Auth Proxy) that must be reachable when the `db-migrate` init container runs. Without this, the migration deadlocks: the init container waits for the proxy, but regular sidecars only start after all init containers finish.

### Example: Cloud SQL Auth Proxy (Native Sidecar)

```yaml
# values.yaml
nativeSidecars:
  - name: cloud-sql-proxy
    restartPolicy: Always
    image: gcr.io/cloud-sql-connectors/cloud-sql-proxy:2.14.3
    args:
      - "--auto-iam-authn"
      - "--structured-logs"
      - "--port=5432"
      - "PROJECT:REGION:INSTANCE"
    securityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop: [ALL]
      readOnlyRootFilesystem: true
      runAsNonRoot: true
      seccompProfile:
        type: RuntimeDefault
    resources:
      requests:
        cpu: 100m
        memory: 64Mi
      limits:
        cpu: 200m
        memory: 128Mi
```

With this setup, both the `db-migrate` init container and the runtime API container connect through the proxy — no `extraEnv` override needed since the proxy listens on `localhost:5432`. The `database.external.secretName` secret must set `db.host` to `localhost` and `db.port` to `5432` so both containers route through the proxy.

### Regular Sidecars (`sidecars`)

Use the `sidecars` list for containers that don't need to be available during init. The example below shows a PgBouncer connection pooler:

```yaml
# values.yaml
sidecars:
  - name: pgbouncer
    image: public.ecr.aws/bitnami/pgbouncer:1.25.1
    securityContext:
      allowPrivilegeEscalation: false
      capabilities:
        drop: [ALL]
      readOnlyRootFilesystem: true
      seccompProfile:
        type: RuntimeDefault
    ports:
      - name: pgbouncer
        containerPort: 6432
        protocol: TCP
    env:
      - name: POSTGRESQL_HOST
        value: my-postgresql-host
      - name: POSTGRESQL_PORT
        value: "5432"
      - name: POSTGRESQL_DATABASE
        value: hyperfleet
      - name: POSTGRESQL_USERNAME
        value: hyperfleet
      - name: POSTGRESQL_PASSWORD
        valueFrom:
          secretKeyRef:
            name: my-db-secret
            key: db.password
      - name: PGBOUNCER_PORT
        value: "6432"
      - name: PGBOUNCER_POOL_MODE
        value: transaction
    resources:
      limits:
        cpu: 200m
        memory: 128Mi
      requests:
        cpu: 50m
        memory: 64Mi
```

When using a regular sidecar proxy, the `db-migrate` init container connects directly to the database (regular sidecars aren't running yet). Route runtime traffic through the proxy with `extraEnv`:

```yaml
extraEnv:
  - name: HYPERFLEET_DATABASE_HOST
    value: "localhost"
  - name: HYPERFLEET_DATABASE_PORT
    value: "6432"  # proxy port
```

Use `extraVolumes` and `extraVolumeMounts` for any volumes the sidecar requires (e.g., temp dirs, config dirs).

### Architecture

```text
┌────────────────────────────────────────────────────────┐
│  Pod                                                   │
│                                                        │
│  nativeSidecars (restartPolicy: Always)                  │
│  ┌──────────────────┐                                  │
│  │  cloud-sql-proxy  │◄─── starts first, stays running │
│  └────────┬─────────┘                                  │
│           │                                            │
│  initContainers                                        │
│  ┌──────────────────┐                                  │
│  │  db-migrate       │──▶ proxy ──▶ Database           │
│  └──────────────────┘                                  │
│                                                        │
│  containers                                            │
│  ┌──────────────────┐                                  │
│  │  hyperfleet-api   │──▶ proxy ──▶ Database           │
│  └──────────────────┘                                  │
└────────────────────────────────────────────────────────┘
```

### Common Proxy Choices

- **Cloud SQL Auth Proxy**: Required for GCP Cloud SQL. Use `nativeSidecars` so migrations can reach the database through the proxy.
- **PgBouncer**: Lightweight connection pooler. Use `transaction` pool mode for stateless APIs. Can go in either `nativeSidecars` or `sidecars` depending on whether migrations need it.

## Related Documentation

- [Development Guide](development.md) - Database setup and migrations
- [Deployment](deployment.md) - Database configuration and connection settings
- [API Resources](api-resources.md) - Resource data models
