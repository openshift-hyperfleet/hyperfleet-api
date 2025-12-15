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
- Run via `./hyperfleet-api migrate`

## Database Setup

```bash
# Create PostgreSQL container
make db/setup

# Run migrations
./hyperfleet-api migrate

# Connect to database
make db/login
```

See [development.md](development.md) for detailed setup instructions.

## Related Documentation

- [Development Guide](development.md) - Database setup and migrations
- [Deployment](deployment.md) - Database configuration and connection settings
- [API Resources](api-resources.md) - Resource data models
