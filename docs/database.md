# Database

This document describes the database architecture used by HyperFleet API.

## Overview

HyperFleet API uses PostgreSQL with GORM ORM. The schema follows a simple relational model with polymorphic associations.

## Core Tables

### clusters
Primary resources for cluster management. Contains cluster metadata and JSONB spec field for provider-specific configuration.

### node_pools
Child resources owned by clusters, representing groups of compute nodes. Uses foreign key relationship with cascade delete.

### adapter_statuses
Polymorphic status records for both clusters and node pools. Stores adapter-reported conditions in JSONB format.

**Polymorphic pattern:**
- `owner_type` + `owner_id` allows one table to serve both clusters and node pools
- Enables efficient status lookups across resource types

### labels
Key-value pairs for resource categorization and search. Uses polymorphic association to support both clusters and node pools.

## Schema Relationships

```text
clusters (1) ──→ (N) node_pools
    │                    │
    │                    │
    └────────┬───────────┘
             │
             ├──→ adapter_statuses (polymorphic)
             └──→ labels (polymorphic)
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

### Soft Delete

Resources use GORM's soft delete pattern with `deleted_at` timestamp. Soft-deleted records are excluded from queries by default.

### Migration System

Uses GORM AutoMigrate:
- Non-destructive (never drops columns or tables)
- Additive (creates missing tables, columns, indexes)
- Run via `./bin/hyperfleet-api migrate`

### Migration Coordination

**Problem:** During rolling deployments, multiple pods attempt to run migrations simultaneously, causing race conditions and deployment failures.

**Solution:** PostgreSQL advisory locks ensure exclusive migration execution.

#### How It Works

```go
// Only one pod/process acquires the lock and runs migrations
// Others wait until the lock is released
db.MigrateWithLock(ctx, factory)
```

**Implementation:**
1. Pod sets statement timeout (5 minutes) to prevent indefinite blocking
2. Pod acquires advisory lock via `pg_advisory_xact_lock(hash("migrations"), hash("Migrations"))`
3. Lock holder runs migrations exclusively
4. Other pods block until lock is released or timeout is reached
5. Lock automatically released on transaction commit

**Key Features:**
- **Zero infrastructure overhead** - Uses native PostgreSQL locks
- **Automatic cleanup** - Locks released on transaction end or pod crash
- **Timeout protection** - 5-minute timeout prevents indefinite blocking if a pod hangs
- **Nested lock support** - Same lock can be acquired in nested contexts without deadlock
- **UUID-based ownership** - Only original acquirer can unlock

#### Testing Concurrent Migrations

Integration tests validate concurrent behavior:

```bash
make test-integration  # Runs TestConcurrentMigrations
```

**Test coverage:**
- `TestConcurrentMigrations` - Multiple pods running migrations simultaneously
- `TestAdvisoryLocksConcurrently` - Lock serialization under race conditions
- `TestAdvisoryLocksWithTransactions` - Lock + transaction interaction
- `TestAdvisoryLockBlocking` - Lock blocking behavior

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

The API uses an optimized transaction strategy to maximize connection pool efficiency and reduce latency under high adapter polling load.

### Write Operations (POST/PUT/PATCH/DELETE)

Write operations create full GORM transactions with ACID guarantees:
- Transaction begins before handler execution
- Automatic commit on success, rollback on error (via `MarkForRollback()`)
- Transaction ID tracked in logs for debugging

### Read Operations (GET)

Read operations skip transaction creation entirely for performance:
- Direct database session without BEGIN/COMMIT overhead
- No transaction ID consumption
- Reduced connection hold time and pool pressure

### Trade-offs

**List Operations**: COUNT and SELECT queries execute as separate autocommit statements (read operations don't use transactions). PostgreSQL's default READ COMMITTED isolation level means each statement gets a fresh snapshot:

- Under concurrent deletes, `total` count may slightly exceed actual `items` returned
- This is a cosmetic pagination issue, not a data integrity problem
- Occurs only during the ~1ms window between COUNT and SELECT
- Low probability in practice (requires delete between two consecutive queries)

**Why not use transactions for reads?** Creating transactions for every GET request would:
- Increase connection pool pressure under high adapter polling load
- Consume transaction IDs unnecessarily
- Add latency (BEGIN/COMMIT overhead)

**Why not use REPEATABLE READ?** The current inconsistency is acceptable for pagination UX. REPEATABLE READ would add overhead and doesn't align with the read-heavy workload optimization.

**Alternative**: Clients can use continuation tokens (Kubernetes pattern) instead of page/total pagination if strict consistency is required.

## Connection Pool Configuration

The API manages a Go `sql.DB` connection pool with the following tunable parameters, exposed as CLI flags:

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
