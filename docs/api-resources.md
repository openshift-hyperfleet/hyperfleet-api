# API Resources

This document provides detailed information about the HyperFleet API resources, including endpoints, request/response formats, and usage patterns.

## Cluster Management

### Endpoints

```text
GET    /api/hyperfleet/v1/clusters
POST   /api/hyperfleet/v1/clusters
GET    /api/hyperfleet/v1/clusters/{cluster_id}
GET    /api/hyperfleet/v1/clusters/{cluster_id}/statuses
PUT   /api/hyperfleet/v1/clusters/{cluster_id}/statuses
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
<details>
<summary>JSON response 201 created</summary>

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
        "type": "Reconciled",
        "status": "False",
        "reason": "ReconciledMissingAdapters",
        "message": "Required adapters have not yet reported status",
        "observed_generation": 1,
        "created_time": "2025-01-01T00:00:00Z",
        "last_updated_time": "2025-01-01T00:00:00Z",
        "last_transition_time": "2025-01-01T00:00:00Z"
      },
      {
        "type": "LastKnownReconciled",
        "status": "False",
        "reason": "AdaptersMissingReports",
        "message": "Required adapters have not yet reported status",
        "observed_generation": 1,
        "created_time": "2025-01-01T00:00:00Z",
        "last_updated_time": "2025-01-01T00:00:00Z",
        "last_transition_time": "2025-01-01T00:00:00Z"
      },
    ]
  }
}
```

</details>

**Note**: Status initially has `Reconciled=False` and `LastKnownReconciled=False` conditions until adapters report status.

### Get Cluster

**GET** `/api/hyperfleet/v1/clusters/{cluster_id}`

**Response (200 OK):**

<details>
<summary>JSON response</summary>

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
        "type": "Reconciled",
        "status": "True",
        "reason": "ReconciledAll",
        "message": "All required adapters reported Available=True or Finalized=True at the current generation",
        "observed_generation": 1,
        "created_time": "2025-01-01T00:00:00Z",
        "last_updated_time": "2025-01-01T00:00:00Z",
        "last_transition_time": "2025-01-01T00:00:00Z"
      },
      {
        "type": "LastKnownReconciled",
        "status": "True",
        "reason": "AllAdaptersReconciled",
        "message": "All required adapters report Available=True for the tracked generation",
        "observed_generation": 1,
        "created_time": "2025-01-01T00:00:00Z",
        "last_updated_time": "2025-01-01T00:00:00Z",
        "last_transition_time": "2025-01-01T00:00:00Z"
      },
    ]
  }
}
```

</details>

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

**PUT** `/api/hyperfleet/v1/clusters/{cluster_id}/statuses`

Adapters use this endpoint to report their status.

**Request Body:**

<details>
<summary>JSON response</summary>

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

</details>

**Response (201 Created):**

<details>
<summary>JSON response</summary>

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

</details>

**Note**: The API automatically sets `created_time`, `last_report_time`, and `last_transition_time` fields.

## NodePool Management

### Endpoints

```text
GET    /api/hyperfleet/v1/nodepools
GET    /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
POST   /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
GET    /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
GET    /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses
PUT   /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses
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

<details>
<summary>JSON response</summary>

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
        "type": "Reconciled",
        "status": "False",
        "reason": "ReconciledMissingAdapters",
        "message": "Required adapters have not yet reported status",
        "observed_generation": 1,
        "created_time": "2025-01-01T00:00:00Z",
        "last_updated_time": "2025-01-01T00:00:00Z",
        "last_transition_time": "2025-01-01T00:00:00Z"
      },
      {
        "type": "LastKnownReconciled",
        "status": "False",
        "reason": "AdaptersMissingReports",
        "message": "Required adapters have not yet reported status",
        "observed_generation": 1,
        "created_time": "2025-01-01T00:00:00Z",
        "last_updated_time": "2025-01-01T00:00:00Z",
        "last_transition_time": "2025-01-01T00:00:00Z"
      },
    ]
  }
}
```

</details>

### Get NodePool

**GET** `/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}`

**Response (200 OK):**

<details>
<summary>JSON response</summary>

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
        "type": "Reconciled",
        "status": "True",
        "reason": "ReconciledAll",
        "message": "All required adapters reported Available=True or Finalized=True at the current generation",
        "observed_generation": 1
      },
      {
        "type": "LastKnownReconciled",
        "status": "True",
        "reason": "AllAdaptersReconciled",
        "message": "All required adapters report Available=True for the tracked generation",
        "observed_generation": 1
      },
    ]
  }
}
```

</details>

### Report NodePool Status

**PUT** `/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses`

Same format as cluster status reporting (see above).

## Pagination and Search

### Pagination

All list endpoints support pagination:

```text
GET /api/hyperfleet/v1/clusters?page=1&pageSize=10
```

**Parameters:**

- `page` - Page number (default: 1)
- `pageSize` - Items per page (default: 20)

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

All list endpoints support filtering using TSL (Tree Search Language) query syntax. Example:

```bash
curl -G http://localhost:8000/api/hyperfleet/v1/clusters \
  --data-urlencode "search=status.conditions.Reconciled='True' and labels.environment='production'"
```

See **[search.md](search.md)** for complete documentation.

## Field Descriptions

### Common Fields

- `kind` - Resource type (Cluster, NodePool)
- `id` - Unique resource identifier (auto-generated, RFC4122 UUID v7 format: 36-character lowercase with hyphens)
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
  - **Reconciled** - Whether all adapters have reconciled at the current spec generation
  - **LastKnownReconciled** - Whether resource is running at any known good configuration
  - Additional conditions from adapters (with `observed_generation`, timestamps)

### Condition Fields

**In AdapterStatus PUT request (ConditionRequest):**

- `type` - Condition type (Available, Applied, Health)
- `status` - Condition status (True, False)
- `reason` - Machine-readable reason code
- `message` - Human-readable message

**In Cluster/NodePool status (ResourceCondition):**

- All above fields plus:
- `observed_generation` - Generation this condition reflects
- `created_time` - When condition was first created (API-managed)
- `last_updated_time` - API-managed. For per-adapter conditions, taken from `AdapterStatus.last_report_time`. For aggregated conditions (`Reconciled`, `LastKnownReconciled`), computed as the oldest valid adapter report time within the relevant generation bucket — not the latest report time
- `last_transition_time` - When status last changed (API-managed)

## Parameter Restrictions

### Query Parameters

All list endpoints accept the following query parameters:

| Parameter  | Type           | Required | Default        | Constraints          |
|------------|----------------|----------|----------------|----------------------|
| `search`   | string         | No       | -              | TSL query syntax     |
| `page`     | integer (int32)| No       | `1`            | -                    |
| `pageSize` | integer (int32)| No       | `20`           | -                    |
| `orderBy`  | string         | No       | `created_time` | -                    |
| `order`    | string         | No       | -              | Must be `asc` or `desc` |

### Path Parameters

| Parameter     | Type   | Required |
|---------------|--------|----------|
| `cluster_id`  | string | Yes      |
| `nodepool_id` | string | Yes      |

### Request Body Constraints

#### Cluster Name (`ClusterCreateRequest.name`)

| Constraint  | Value                                  |
|-------------|----------------------------------------|
| Required    | Yes                                    |
| Type        | string                                 |
| Min length  | 3                                      |
| Max length  | 53                                     |
| Pattern     | `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`     |

Must be lowercase alphanumeric, may contain hyphens, and must start and end with an alphanumeric character.

#### NodePool Name (`NodePoolCreateRequest.name`)

| Constraint  | Value                                  |
|-------------|----------------------------------------|
| Required    | Yes                                    |
| Type        | string                                 |
| Min length  | 3                                      |
| Max length  | 15                                     |
| Pattern     | `^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`     |

Same naming rules as cluster, but with a shorter maximum length.

#### Adapter Status (`AdapterStatusCreateRequest`)

| Field                | Type           | Required |
|----------------------|----------------|----------|
| `adapter`            | string         | Yes      |
| `observed_generation`| integer (int32)| Yes      |
| `observed_time`      | string (date-time) | Yes  |
| `conditions`         | array of `ConditionRequest` | Yes |
| `metadata`           | object         | No       |
| `data`               | object         | No       |

#### Condition Request (`ConditionRequest`)

| Field     | Type   | Required | Constraints                          |
|-----------|--------|----------|--------------------------------------|
| `type`    | string | Yes      | -                                    |
| `status`  | string | Yes      | Must be `True`, `False`, or `Unknown` |
| `reason`  | string | No       | -                                    |
| `message` | string | No       | -                                    |

### Enum Values

- **AdapterConditionStatus** (used in adapter status reports): `True`, `False`, `Unknown`
- **ResourceConditionStatus** (used in cluster/nodepool conditions): `True`, `False`
- **OrderDirection**: `asc`, `desc`

## Spec Validation

When an OpenAPI schema is configured (see [deployment.md](deployment.md#schema-validation) for setup), the API validates cluster and nodepool `spec` fields on every create and update request. If no schema is configured, all specs are accepted without validation. When a schema is configured:

- `POST /clusters` and `POST /nodepools` validate `spec` against `ClusterSpec` or `NodePoolSpec` from the schema
- `PATCH /clusters/{id}` and `PATCH /nodepools/{id}` validate the merged result
- Invalid specs return a `400` with validation details in the error response

The schema is configured via `--server-openapi-schema-path` or the `validationSchema` section in the Helm chart. See [Validation Schema](../openapi/README.md#validation-schema) for details.

## Statuses Endpoint vs. Resource Endpoint

- `GET /clusters/{id}` returns the cluster with **aggregated** status conditions (`Reconciled`, `LastKnownReconciled`, and per-adapter conditions synthesized from adapter reports).
- `GET /clusters/{id}/statuses` returns the **raw adapter status records** — one per adapter that has reported. These are the individual reports, not the aggregated view.

The same distinction applies to nodepools.

## Related Documentation

- [Example Usage](../README.md#example-usage) - Practical examples
- [Authentication](authentication.md) - API authentication
- [Database](database.md) - Database schema
