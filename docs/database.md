# Database

This document describes the database architecture used by HyperFleet API.

## Overview

HyperFleet API uses PostgreSQL with GORM ORM. The schema follows a simple relational model with polymorphic associations.

## Core Tables

### resources

Unified resource table for all entity types (Cluster, NodePool, Channel, Version, WifConfig, etc.). Stores `kind`, `name`, `spec` (JSONB), `tenancy` (JSONB), and optional owner references (`owner_id`, `owner_kind`, `owner_href`) for parent-child relationships. Labels are stored in the separate `resource_labels` table; conditions in `resource_conditions`. Uses `deleted_time`/`deleted_by` for soft delete. Unique name constraints are tenant-scoped: root resources (`owner_id IS NULL`) are unique on `(kind, name, tenancy)`, so two tenants can own a resource with the same name; child resources (`owner_id IS NOT NULL`) are unique on `(kind, owner_id, name)` — tenant isolation for children follows transitively from their owner (the globally-unique root row that carries the tenancy), so their own index does not include `tenancy`. Both constraints apply only to active rows (via `WHERE deleted_time IS NULL`), so soft-deleted resources do not block name reuse.

### adapter_statuses

Status records for resources. Stores adapter-reported conditions in JSONB format. No soft delete — rows are hard-deleted or replaced.

**Polymorphic pattern:**

- `resource_type` + `resource_id` identifies the owning resource
- Unique constraint on `(resource_type, resource_id, adapter)` — one record per adapter per resource

## Schema Relationships

```text
resources (self-referencing parent-child via owner_id)
    ├──→ resource_conditions (via resource_id, composite PK: resource_id + type)
    ├──→ resource_labels (via resource_id, composite PK: resource_id + key)
    ├──→ resource_references (via source_id/target_id)
    └──→ adapter_statuses (via resource_type + resource_id)
```

## Key Design Patterns

### JSONB Fields

Flexible schema storage for:

- **spec** - Provider-specific resource configurations
- **tenancy** - Tenancy dimensions carried by the resource (not null, defaults to `{}`); indexed with a GIN index using `jsonb_path_ops` to serve containment queries. DAO reads, lists, and deletes filter on `tenancy @> ?` against the caller's resolved tenant dimensions, so this column drives per-request access control, not just uniqueness
- **conditions** - Adapter status condition arrays
- **data** - Adapter metadata

**Benefits:**

- Support multiple cloud providers without schema changes
- Runtime validation against OpenAPI schema
- PostgreSQL JSON query capabilities

Adapter statuses do not use soft delete — they are hard-deleted when their parent resource is hard-deleted.

### Tenant Scoping

When tenant enforcement is enabled, the caller's resolved tenancy is applied as a JSONB containment predicate (`tenancy @> ?`) on reads, lists, updates, and deletes (updates and deletes go through the same scoped `GetForUpdate` lookup, so a cross-tenant target is not found). Matching is by **containment, not equality**: a caller is authorized for a resource when the caller's tenancy map is a subset of the resource's. An org-scoped caller (`{org: acme}`) therefore sees every resource under that org, including those with extra dimensions (`{org: acme, project: platform}`), while a caller scoped to `{org: acme, project: p1}` does not see `{org: acme, project: p2}`.

#### Where `tenancy` comes from

On create by a non-system caller, the API fills the resource's `tenancy` column from the caller's resolved dimensions. Clients cannot set it: any `tenancy` sent in the request body is ignored, and the column can never change after creation. So a resource permanently carries the tenancy of whoever created it.

System identities cannot create resources at all — they may write only `status`/`conditions`, so a create (or any other spec mutation) from a system identity is rejected with `403 Forbidden` before any `tenancy` is assigned. The creator-dimension rule therefore applies only to non-system creates.

#### Fail-closed on zero dimensions

A non-system caller that resolves to *no* dimensions is rejected with `403` in the middleware, before any query runs. The DAO has a second safeguard in case such a request ever slips through: instead of running an unscoped query, it applies a `1 = 0` predicate that matches nothing.

This backstop exists because matching is by containment and the empty map `{}` is a subset of *every* row. Without it, a zero-dimension caller's predicate (`tenancy @> '{}'`) would match every resource — an accidental "see everything." Failing closed turns that into "see nothing."

#### System callers and pre-existing rows

System callers (e.g. Sentinel, adapters) skip scoping entirely — no `tenancy` predicate is added, so they see all resources.

> **Security:** because `System=true` removes the tenancy predicate, the system header is only trustworthy when it originates from the Envoy + Authorino gateway. The gateway must authenticate and allowlist system clients, strip any client-supplied system header, and block direct routes to the API pod (see [ADR-0020](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/adrs/0020-envoy-authorino-api-gateway.md)). Without that boundary, a spoofed system header — or direct pod access — would expose cross-tenant resources. Only enable tenant enforcement behind such a gateway.

Rows created *before* enforcement was enabled carry `tenancy = '{}'`. Since a scoped caller is only authorized when its (non-empty) tenancy is a subset of the row's, no scoped caller can be a subset of `{}` — so these legacy rows are visible only to system/unscoped callers. Because `tenancy` is immutable after creation and there is no backfill or re-stamp operation, this is permanent: a legacy `{}` resource can never be brought into a tenant's scope. Enable tenant enforcement before creating tenant-owned resources.

#### Uniqueness vs. visibility

These use `tenancy` in two different ways, and it matters:

- **Uniqueness** — the root-resource index `(kind, name, tenancy)` compares `tenancy` by **exact equality**.
- **Visibility** — access scoping compares `tenancy` by **containment** (subset).

Because uniqueness is by exact equality, two callers scoped to different tenancy documents can each create a resource with the same `kind` and `name` without colliding — their rows differ in the `tenancy` column.

See [Tenant isolation](authentication.md#tenant-isolation) for the request-path behavior and trust model.

### Delete Policies

Resources use delete policies to control child behavior when a parent is deleted. Each resource type declares its policy in its entity descriptor:

| Policy     | Behavior |
|------------|----------|
| `restrict` | Parent delete is rejected with `409 Conflict` if active children exist |
| `cascade`  | All children are soft-deleted (marked Finalizing) along with the parent |

Policies are enforced recursively — a cascade on a parent triggers policy checks on children. For example, deleting a Cluster cascades to all its NodePools — those with required adapters are soft-deleted (entering Finalizing), while those without are hard-deleted immediately.

Resources without required adapters skip the Finalizing phase entirely — they are hard-deleted immediately on `DELETE`.

### Migration System

Migrations are:

- Non-destructive (never drops columns or tables)
- Additive (creates missing tables, columns, indexes)
- Run via `./bin/hyperfleet-api migrate`
- Fix-forward only — a bad migration is corrected by a new migration, never a rollback

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
| ------ | --------- | ------------- |
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
