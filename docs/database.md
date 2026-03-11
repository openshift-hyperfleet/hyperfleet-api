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

## PgBouncer Connection Pooler

For production deployments, the Helm chart includes an optional [PgBouncer](https://www.pgbouncer.org/) sidecar that acts as a lightweight connection pooler between the API and PostgreSQL.

### Why PgBouncer?

Without pgbouncer, each API pod opens up to `--db-max-open-connections` (default 50) direct connections to PostgreSQL. At scale:

- 10 pods = 500 direct connections to a single PostgreSQL instance
- Connection setup overhead (TLS handshake, authentication) is paid per connection
- PostgreSQL `max_connections` becomes a bottleneck

PgBouncer in **transaction mode** multiplexes many client connections over a smaller pool of server connections. A server connection is only held for the duration of a single transaction, then returned to the pool.

### Enabling PgBouncer

```yaml
# values.yaml
database:
  pgbouncer:
    enabled: true
```

Or via Helm install/upgrade:

```bash
helm upgrade hyperfleet-api charts/ --set database.pgbouncer.enabled=true
```

### Architecture

When pgbouncer is enabled, the deployment changes:

```text
┌─────────────────────────────────────────┐
│  Pod                                    │
│                                         │
│  ┌──────────────┐     ┌──────────┐      │
│  │  hyperfleet   │────▶│ pgbouncer │─────┼──▶ PostgreSQL
│  │  API          │     │ :6432    │      │   (separate pod)
│  │  (localhost)  │     └──────────┘      │
│  └──────────────┘                       │
│                                         │
│  Init containers (migrate) ─────────────┼──▶ PostgreSQL
│  (direct connection, bypasses pgbouncer) │
└─────────────────────────────────────────┘
```

- **API container** connects to `localhost:6432` (pgbouncer)
- **Init containers** (migrations) connect directly to PostgreSQL — they run before the sidecar starts and need DDL operations that don't work well with transaction pooling
- Two Kubernetes Secrets are created: one with the direct PostgreSQL host, one pointing to `localhost:6432`

### Configuration

All pgbouncer settings are in `values.yaml` under `database.pgbouncer`:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `enabled` | `false` | Enable pgbouncer sidecar |
| `image` | `public.ecr.aws/bitnami/pgbouncer:1.25.1` | PgBouncer container image |
| `port` | `6432` | Port pgbouncer listens on |
| `poolMode` | `transaction` | Pool mode (`transaction` recommended for stateless APIs) |
| `defaultPoolSize` | `50` | Server connections per database per user |
| `maxClientConn` | `100` | Maximum client connections accepted |
| `minPoolSize` | `5` | Minimum server connections kept open |
| `serverIdleTimeout` | `600` | Close idle server connections after this many seconds |
| `serverLifetime` | `3600` | Close server connections after this many seconds regardless of activity |

### Pool Modes

- **`transaction`** (default, recommended): Server connection is assigned per transaction. Best for stateless CRUD APIs like HyperFleet. Allows high client concurrency with fewer server connections. HyperFleet uses GORM with simple queries and no prepared statements, so transaction mode is safe.
- **`session`**: Server connection held for the entire client session. Required if the application relies on session-level state (prepared statements, `SET` commands, advisory locks, temp tables). Provides less connection multiplexing than transaction mode.
- **`statement`**: Server connection per statement. Most aggressive pooling but breaks multi-statement transactions.

### Monitoring

PgBouncer logs connection stats every 60 seconds:

```text
LOG stats: 15 xacts/s, 30 queries/s, in 1234 B/s, out 5678 B/s, xact 2ms, query 1ms, wait 0us
```

Key metrics:
- **xacts/s**: Transactions per second
- **wait**: Time clients spend waiting for a server connection (should be near 0)
- **xact**: Average transaction duration

### Limitations

- PgBouncer sidecar is currently only supported with the built-in PostgreSQL deployment (`database.postgresql.enabled=true`). External database support requires manual pgbouncer configuration.
- Migrations bypass pgbouncer intentionally — DDL statements and `SET` commands are not compatible with transaction-mode pooling.

## Related Documentation

- [Development Guide](development.md) - Database setup and migrations
- [Deployment](deployment.md) - Database configuration and connection settings
- [API Resources](api-resources.md) - Resource data models
