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
    "phase": "NotReady",
    "observed_generation": 0,
    "last_transition_time": "2025-01-01T00:00:00Z",
    "last_updated_time": "2025-01-01T00:00:00Z",
    "conditions": []
  }
}
```

**Note**: Status is initially `NotReady` with empty conditions until adapters report status.

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
    "phase": "Ready",
    "observed_generation": 1,
    "last_transition_time": "2025-01-01T00:00:00Z",
    "last_updated_time": "2025-01-01T00:00:00Z",
    "conditions": [
      {
        "type": "ValidationSuccessful",
        "status": "True",
        "reason": "AllValidationsPassed",
        "message": "All validations passed",
        "observed_generation": 1,
        "created_time": "2025-01-01T00:00:00Z",
        "last_updated_time": "2025-01-01T00:00:00Z",
        "last_transition_time": "2025-01-01T00:00:00Z"
      },
      {
        "type": "DNSSuccessful",
        "status": "True",
        "reason": "DNSProvisioned",
        "message": "DNS successfully configured",
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

### Status Phases

- `NotReady` - Cluster is being provisioned or has failing conditions
- `Ready` - All adapter conditions report success
- `Failed` - Cluster provisioning or operation failed

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
    "phase": "NotReady",
    "observed_generation": 0,
    "last_transition_time": "2025-01-01T00:00:00Z",
    "last_updated_time": "2025-01-01T00:00:00Z",
    "conditions": []
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
    "phase": "Ready",
    "observed_generation": 1,
    "last_transition_time": "2025-01-01T00:00:00Z",
    "last_updated_time": "2025-01-01T00:00:00Z",
    "conditions": [...]
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

# AND query
curl -G http://localhost:8000/api/hyperfleet/v1/clusters \
  --data-urlencode "search=status.phase='Ready' and labels.environment='production'"

# OR query
curl -G http://localhost:8000/api/hyperfleet/v1/clusters \
  --data-urlencode "search=labels.environment='dev' or labels.environment='staging'"
```

**Supported fields:**
- `name` - Resource name
- `status.phase` - Status phase (NotReady, Ready, Failed)
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

- `phase` - Current resource phase (NotReady, Ready, Failed)
- `observed_generation` - Last spec generation processed (min across all adapters)
- `last_transition_time` - When phase last changed
- `last_updated_time` - Min of all adapter last_report_time (detects stale adapters)
- `conditions` - Array of resource conditions from adapters

### Condition Fields

**In AdapterStatus POST request (ConditionRequest):**
- `type` - Condition type (Available, Applied, Health)
- `status` - Condition status (True, False, Unknown)
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
