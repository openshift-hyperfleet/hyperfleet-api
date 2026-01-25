# API Resources

This document provides detailed information about the HyperFleet API resources, including endpoints, request/response formats, and usage patterns.

## Cluster Management

### Endpoints

```text
GET    /api/hyperfleet/v1/clusters
POST   /api/hyperfleet/v1/clusters
GET    /api/hyperfleet/v1/clusters/{cluster_id}
GET    /api/hyperfleet/v1/clusters/{cluster_id}/statuses
POST   /api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

### Create Cluster

**POST** `/api/hyperfleet/v1/clusters`

**Request Body:**

```json
{
  "kind": "Cluster",
  "name": "my-cluster",
  "spec": {},
  "labels": {
    "environment": "production"
  }
}
```

**Response (201 Created):**

```json
{
  "kind": "Cluster",
  "id": "2abc123...",
  "href": "/api/hyperfleet/v1/clusters/2abc123...",
  "name": "my-cluster",
  "generation": 1,
  "spec": {},
  "labels": {
    "environment": "production"
  },
  "created_time": "2025-01-01T00:00:00Z",
  "updated_time": "2025-01-01T00:00:00Z",
  "created_by": "user@example.com",
  "updated_by": "user@example.com",
  "status": {
    "conditions": [
      {
        "type": "Available",
        "status": "False",
        "reason": "AwaitingAdapters",
        "message": "Waiting for adapters to report status",
        "observed_generation": 0,
        "created_time": "2025-01-01T00:00:00Z",
        "last_updated_time": "2025-01-01T00:00:00Z",
        "last_transition_time": "2025-01-01T00:00:00Z"
      },
      {
        "type": "Ready",
        "status": "False",
        "reason": "AwaitingAdapters",
        "message": "Waiting for adapters to report status",
        "observed_generation": 0,
        "created_time": "2025-01-01T00:00:00Z",
        "last_updated_time": "2025-01-01T00:00:00Z",
        "last_transition_time": "2025-01-01T00:00:00Z"
      }
    ]
  }
}
```

**Note**: Status initially has `Available=False` and `Ready=False` conditions until adapters report status.

### Get Cluster

**GET** `/api/hyperfleet/v1/clusters/{cluster_id}`

**Response (200 OK):**

```json
{
  "kind": "Cluster",
  "id": "2abc123...",
  "href": "/api/hyperfleet/v1/clusters/2abc123...",
  "name": "my-cluster",
  "generation": 1,
  "spec": {},
  "labels": {
    "environment": "production"
  },
  "created_time": "2025-01-01T00:00:00Z",
  "updated_time": "2025-01-01T00:00:00Z",
  "created_by": "user@example.com",
  "updated_by": "user@example.com",
  "status": {
    "conditions": [
      {
        "type": "Available",
        "status": "True",
        "reason": "ResourceAvailable",
        "message": "Cluster is accessible",
        "observed_generation": 1,
        "created_time": "2025-01-01T00:00:00Z",
        "last_updated_time": "2025-01-01T00:00:00Z",
        "last_transition_time": "2025-01-01T00:00:00Z"
      },
      {
        "type": "Ready",
        "status": "True",
        "reason": "ResourceReady",
        "message": "All adapters report ready at current generation",
        "observed_generation": 1,
        "created_time": "2025-01-01T00:00:00Z",
        "last_updated_time": "2025-01-01T00:00:00Z",
        "last_transition_time": "2025-01-01T00:00:00Z"
      }
    ]
  }
}
```

### List Clusters

**GET** `/api/hyperfleet/v1/clusters?page=1&pageSize=10`

**Response (200 OK):**

```json
{
  "kind": "ClusterList",
  "page": 1,
  "size": 10,
  "total": 100,
  "items": [
    {
      "kind": "Cluster",
      "id": "2abc123...",
      "name": "my-cluster",
      ...
    }
  ]
}
```

### Report Cluster Status

**POST** `/api/hyperfleet/v1/clusters/{cluster_id}/statuses`

Adapters use this endpoint to report their status.

**Request Body:**

```json
{
  "adapter": "validator",
  "observed_generation": 1,
  "observed_time": "2025-01-01T10:00:00Z",
  "conditions": [
    {
      "type": "Available",
      "status": "True",
      "reason": "AllValidationsPassed",
      "message": "All validations passed"
    },
    {
      "type": "Applied",
      "status": "True",
      "reason": "ValidationJobApplied",
      "message": "Validation job applied successfully"
    },
    {
      "type": "Health",
      "status": "True",
      "reason": "OperationsCompleted",
      "message": "All adapter operations completed successfully"
    }
  ],
  "data": {
    "job_name": "validator-job-abc123",
    "attempt": 1
  }
}
```

**Response (201 Created):**

```json
{
  "adapter": "validator",
  "observed_generation": 1,
  "conditions": [
    {
      "type": "Available",
      "status": "True",
      "reason": "AllValidationsPassed",
      "message": "All validations passed",
      "last_transition_time": "2025-01-01T10:00:00Z"
    },
    {
      "type": "Applied",
      "status": "True",
      "reason": "ValidationJobApplied",
      "message": "Validation job applied successfully",
      "last_transition_time": "2025-01-01T10:00:00Z"
    },
    {
      "type": "Health",
      "status": "True",
      "reason": "OperationsCompleted",
      "message": "All adapter operations completed successfully",
      "last_transition_time": "2025-01-01T10:00:00Z"
    }
  ],
  "data": {
    "job_name": "validator-job-abc123",
    "attempt": 1
  },
  "created_time": "2025-01-01T10:00:00Z",
  "last_report_time": "2025-01-01T10:00:00Z"
}
```

**Note**: The API automatically sets `created_time`, `last_report_time`, and `last_transition_time` fields.

### Status Conditions

The status uses Kubernetes-style conditions instead of a single phase field:

- **Ready** - Whether all adapters report successfully at the current generation
  - `True`: All required adapters report `Available=True` at current spec generation
  - `False`: One or more adapters report Available=False at current generation
    - After every spec change, `Ready` becomes `False` since adapters take some time to report at current spec generation
    - Default value when creating the cluster, when no adapters have reported yet any value

- **Available** - Aggregated adapter result for a common `observed_generation`
  - `True`: All required adapters report Available=True for the same observed_generation
  - `False`: At least one adapter reports Available=False when all adapters report the same observed_generation
    - Default value when creating the cluster, when no adapters have reported yet any value

`Available` keeps its value unchanged in case adapters report from a different `observed_generation` or there is already a mix of `observed_generation` statuses

- e.g. `Available=True` for `observed_generation==1`
  - One adapter reports `Available=False` for `observed_generation=1` `Available` transitions to `False`
  - One adapter reports `Available=False` for `observed_generation=2` `Available` keeps its `True` status

## NodePool Management

### Endpoints

```text
GET    /api/hyperfleet/v1/nodepools
GET    /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
POST   /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
GET    /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
GET    /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses
POST   /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses
```

### Create NodePool

**POST** `/api/hyperfleet/v1/clusters/{cluster_id}/nodepools`

**Request Body:**

```json
{
  "kind": "NodePool",
  "name": "worker-pool",
  "spec": {},
  "labels": {
    "role": "worker"
  }
}
```

**Response (201 Created):**

```json
{
  "kind": "NodePool",
  "id": "2def456...",
  "href": "/api/hyperfleet/v1/nodepools/2def456...",
  "name": "worker-pool",
  "owner_references": {
    "kind": "Cluster",
    "id": "2abc123..."
  },
  "generation": 1,
  "spec": {},
  "labels": {
    "role": "worker"
  },
  "created_time": "2025-01-01T00:00:00Z",
  "updated_time": "2025-01-01T00:00:00Z",
  "created_by": "user@example.com",
  "updated_by": "user@example.com",
  "status": {
    "conditions": [
      {
        "type": "Available",
        "status": "False",
        "reason": "AwaitingAdapters",
        "message": "Waiting for adapters to report status",
        "observed_generation": 0,
        "created_time": "2025-01-01T00:00:00Z",
        "last_updated_time": "2025-01-01T00:00:00Z",
        "last_transition_time": "2025-01-01T00:00:00Z"
      },
      {
        "type": "Ready",
        "status": "False",
        "reason": "AwaitingAdapters",
        "message": "Waiting for adapters to report status",
        "observed_generation": 0,
        "created_time": "2025-01-01T00:00:00Z",
        "last_updated_time": "2025-01-01T00:00:00Z",
        "last_transition_time": "2025-01-01T00:00:00Z"
      }
    ]
  }
}
```

### Get NodePool

**GET** `/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}`

**Response (200 OK):**

```json
{
  "kind": "NodePool",
  "id": "2def456...",
  "href": "/api/hyperfleet/v1/nodepools/2def456...",
  "name": "worker-pool",
  "owner_references": {
    "kind": "Cluster",
    "id": "2abc123..."
  },
  "generation": 1,
  "spec": {},
  "labels": {
    "role": "worker"
  },
  "created_time": "2025-01-01T00:00:00Z",
  "updated_time": "2025-01-01T00:00:00Z",
  "created_by": "user@example.com",
  "updated_by": "user@example.com",
  "status": {
    "conditions": [
      {
        "type": "Available",
        "status": "True",
        "reason": "ResourceAvailable",
        "message": "NodePool is accessible",
        "observed_generation": 1
      },
      {
        "type": "Ready",
        "status": "True",
        "reason": "ResourceReady",
        "message": "All adapters report ready at current generation",
        "observed_generation": 1
      }
    ]
  }
}
```

### Report NodePool Status

**POST** `/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses`

Same format as cluster status reporting (see above).

## Pagination and Search

### Pagination

All list endpoints support pagination:

```text
GET /api/hyperfleet/v1/clusters?page=1&pageSize=10
```

**Parameters:**

- `page` - Page number (default: 1)
- `pageSize` - Items per page (default: 100)

**Response:**

```json
{
  "kind": "ClusterList",
  "page": 1,
  "size": 10,
  "total": 100,
  "items": [...]
}
```

### Search

Search using TSL (Tree Search Language) query syntax:

```bash
# Simple equality
curl -G http://localhost:8000/api/hyperfleet/v1/clusters \
  --data-urlencode "search=name='my-cluster'"

# AND query with condition-based status
curl -G http://localhost:8000/api/hyperfleet/v1/clusters \
  --data-urlencode "search=status.conditions.Ready='True' and labels.environment='production'"

# OR query
curl -G http://localhost:8000/api/hyperfleet/v1/clusters \
  --data-urlencode "search=labels.environment='dev' or labels.environment='staging'"

# Query for available resources
curl -G http://localhost:8000/api/hyperfleet/v1/clusters \
  --data-urlencode "search=status.conditions.Available='True'"
```

**Supported fields:**

- `name` - Resource name
- `status.conditions.<Type>` - Condition status (True, False). Examples:
  - `status.conditions.Ready='True'` - Resources that are ready
  - `status.conditions.Available='True'` - Resources that are available
- `labels.<key>` - Label values

**Supported operators:**

- `=` - Equality
- `in` - In list
- `and` - Logical AND
- `or` - Logical OR

## Field Descriptions

### Common Fields

- `kind` - Resource type (Cluster, NodePool)
- `id` - Unique resource identifier (auto-generated, format: `2<base62>`)
- `href` - Resource URI
- `name` - Resource name (user-defined)
- `generation` - Spec version counter (incremented on spec updates)
- `spec` - Provider-specific configuration (JSONB, validated against OpenAPI schema)
- `labels` - Key-value pairs for categorization and search
- `created_time` - When resource was created (API-managed)
- `updated_time` - When resource was last updated (API-managed)
- `created_by` - User who created the resource (email)
- `updated_by` - User who last updated the resource (email)

### Status Fields

The status object contains synthesized conditions computed from adapter reports:

- `conditions` - Array of resource conditions, including:
  - **Available** - Whether resource is running at any known good configuration
  - **Ready** - Whether all adapters have processed current spec generation
  - Additional conditions from adapters (with `observed_generation`, timestamps)

### Condition Fields

**In AdapterStatus POST request (ConditionRequest):**

- `type` - Condition type (Available, Applied, Health)
- `status` - Condition status (True, False)
- `reason` - Machine-readable reason code
- `message` - Human-readable message

**In Cluster/NodePool status (ResourceCondition):**

- All above fields plus:
- `observed_generation` - Generation this condition reflects
- `created_time` - When condition was first created (API-managed)
- `last_updated_time` - When adapter last reported (API-managed, from AdapterStatus.last_report_time)
- `last_transition_time` - When status last changed (API-managed)

## Related Documentation

- [Example Usage](../README.md#example-usage) - Practical examples
- [Authentication](authentication.md) - API authentication
- [Database](database.md) - Database schema
