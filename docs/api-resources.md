# API Resources

This document provides detailed information about the HyperFleet API resources, including endpoints, request/response formats, and usage patterns.

## Authentication Prerequisites

All API endpoints require a valid JWT bearer token when authentication is enabled (the default in production). Requests without a valid token receive `401 Unauthorized`. See [authentication.md](authentication.md) for configuration details, token format, and caller identity resolution.

Mutating requests (POST, PATCH, PUT, DELETE) additionally require a resolvable caller identity — either from a JWT claim or an identity header — which is recorded in audit fields (`created_by`, `updated_by`, `deleted_by`). Read requests (GET, LIST) are allowed without caller identity.

> **Note**: The API does not enforce role-based access control (RBAC). Any authenticated caller can invoke any endpoint, including destructive operations like force-delete. Access control should be enforced at the infrastructure layer (e.g., ingress policies, gateway authorization).

## Cluster Management

### Endpoints

```text
GET    /api/hyperfleet/v1/clusters
POST   /api/hyperfleet/v1/clusters
GET    /api/hyperfleet/v1/clusters/{cluster_id}
PATCH  /api/hyperfleet/v1/clusters/{cluster_id}
DELETE /api/hyperfleet/v1/clusters/{cluster_id}
POST   /api/hyperfleet/v1/clusters/{cluster_id}/force-delete
GET    /api/hyperfleet/v1/clusters/{cluster_id}/statuses
PUT    /api/hyperfleet/v1/clusters/{cluster_id}/statuses
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

### Patch Cluster

**PATCH** `/api/hyperfleet/v1/clusters/{cluster_id}`

Updates a cluster's `spec` and/or `labels`. Only the fields provided in the request body are modified; omitted fields are left unchanged. The `generation` counter increments when `spec` is updated.

**Request Body:**

```json
{
  "spec": {
    "region": "us-east-1",
    "instanceType": "m5.xlarge"
  },
  "labels": {
    "environment": "staging"
  }
}
```

**Response (200 OK):**

<details>
<summary>JSON response</summary>

```json
{
  "kind": "Cluster",
  "id": "2abc123...",
  "href": "/api/hyperfleet/v1/clusters/2abc123...",
  "name": "my-cluster",
  "generation": 2,
  "spec": {
    "region": "us-east-1",
    "instanceType": "m5.xlarge"
  },
  "labels": {
    "environment": "staging"
  },
  "created_time": "2025-01-01T00:00:00Z",
  "updated_time": "2025-01-01T12:00:00Z",
  "created_by": "user@example.com",
  "updated_by": "user@example.com",
  "status": {
    "conditions": [
      {
        "type": "Reconciled",
        "status": "False",
        "reason": "ReconciledMissingAdapters",
        "message": "Required adapters have not yet reported status",
        "observed_generation": 2,
        "created_time": "2025-01-01T00:00:00Z",
        "last_updated_time": "2025-01-01T12:00:00Z",
        "last_transition_time": "2025-01-01T12:00:00Z"
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
      }
    ]
  }
}
```

</details>

**Note**: After a spec update, `Reconciled` transitions to `False` until adapters report at the new generation. `LastKnownReconciled` retains the last known good state.

### Delete Cluster

**DELETE** `/api/hyperfleet/v1/clusters/{cluster_id}`

Soft-deletes a cluster. Sets `deleted_time` and `deleted_by`, increments `generation`, and cascades deletion to child nodepools according to the deletion policy: nodepools with required adapters are soft-deleted (their `deleted_time` and `deleted_by` are set and `generation` is incremented, entering **Finalizing**), while nodepools without required adapters are hard-deleted immediately. The cluster itself enters a **Finalizing** state — it remains in the database until adapters report `Finalized=True`, at which point it is hard-deleted automatically.

For more information, please take a look at the [delete lifecycle](#delete-lifecycle).

**Response (202 Accepted):**

<details>
<summary>JSON response</summary>

```json
{
  "kind": "Cluster",
  "id": "2abc123...",
  "href": "/api/hyperfleet/v1/clusters/2abc123...",
  "name": "my-cluster",
  "generation": 3,
  "spec": {},
  "labels": {},
  "created_time": "2025-01-01T00:00:00Z",
  "updated_time": "2025-01-01T14:00:00Z",
  "created_by": "user@example.com",
  "updated_by": "user@example.com",
  "deleted_time": "2025-01-01T14:00:00Z",
  "deleted_by": "user@example.com",
  "status": {
    "conditions": [...]
  }
}
```

</details>

Once a cluster is soft-deleted, creating or updating child nodepools returns `409 Conflict`.

### Force Delete Cluster

**POST** `/api/hyperfleet/v1/clusters/{cluster_id}/force-delete`

Permanently removes a cluster that is stuck in the Finalizing state. This bypasses the normal adapter finalization flow — use it only when adapters are unable to report `Finalized=True`. See [delete lifecycle](#delete-lifecycle) for more details.

The cluster, all its child nodepools, and all associated adapter statuses are hard-deleted immediately. The caller and reason are recorded in an audit log entry before deletion.

The cluster **must** already be soft-deleted (have a `deleted_time`). Calling force-delete on an active cluster returns `409 Conflict`.

**Request Body:**

```json
{
  "reason": "Adapter crashed and cannot finalize"
}
```

| Field    | Type   | Required | Constraints         |
|----------|--------|----------|---------------------|
| `reason` | string | Yes      | Non-empty, max 1024 |

**Response:** `204 No Content`

## NodePool Management

### Endpoints

```text
GET    /api/hyperfleet/v1/nodepools
GET    /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
POST   /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
GET    /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
PATCH  /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
DELETE /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
POST   /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/force-delete
GET    /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses
PUT    /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses
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

### Patch NodePool

**PATCH** `/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}`

Updates a nodepool's `spec` and/or `labels`. Same semantics as [Patch Cluster](#patch-cluster) — only provided fields are modified, and `generation` increments on spec changes.

**Request Body:**

```json
{
  "spec": {
    "machineType": "n2-standard-4",
    "replicas": 5
  }
}
```

**Response (200 OK):** Full nodepool resource with incremented `generation` and updated `updated_time`/`updated_by`.

### Delete NodePool

**DELETE** `/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}`

Soft-deletes a nodepool. Same lifecycle as [Delete Cluster](#delete-cluster) — sets `deleted_time` and `deleted_by`, enters the Finalizing state, and is hard-deleted when adapters report `Finalized=True`.

For more information, please refer to [delete lifecycle](#delete-lifecycle).

**Response (202 Accepted):** Full nodepool resource with `deleted_time` and `deleted_by` fields set.

### Force Delete NodePool

**POST** `/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/force-delete`

Same semantics as [Force Delete Cluster](#force-delete-cluster). The nodepool must already be soft-deleted.

**Request Body:**

```json
{
  "reason": "Adapter unable to finalize nodepool"
}
```

**Response:** `204 No Content`

## Delete Lifecycle

Resources follow a three-phase delete lifecycle:

```text
Active ──(DELETE)──▶ Finalizing ──(adapters report Finalized=True)──▶ Hard-Deleted
                         │
                         └──(POST /force-delete)──▶ Hard-Deleted
```

1. **Active** — Normal state. Resource is visible in list queries and can be updated.
2. **Finalizing** (soft-deleted) — `DELETE` sets `deleted_time` and `deleted_by`, increments `generation`. The resource stays in the database so adapters can observe the deletion and clean up external state. Soft-deleted records are excluded from list queries by default. Creating new child resources under a finalizing parent is rejected with `409 Conflict`.
3. **Hard-Deleted** — Permanently removed from the database. This happens automatically when all required adapters report `Finalized=True` at the current generation. If adapters are stuck, `POST .../force-delete` bypasses the adapter gating and hard-deletes immediately — but the resource must already be in Finalizing state; calling force-delete on an active resource returns `409 Conflict`. Repeated force-delete calls after hard-deletion return `404 Not Found`. Cluster force-delete cascades to all child NodePools and their adapter statuses. NodePool force-delete only removes the NodePool and its adapter statuses.

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

| Parameter  | Type           | Required | Default             | Constraints          |
|------------|----------------|----------|---------------------|----------------------|
| `search`   | string         | No       | -                   | TSL query syntax     |
| `page`     | integer (int32)| No       | `1`                 | Must be >= 1         |
| `pageSize` | integer (int32)| No       | `20`                | Must be between 1 and 100 |
| `orderBy`  | string         | No       | `created_time desc` | Field name(s) with optional direction (asc/desc) |

**Ordering behavior**:
- Include direction in `orderBy`: `?orderBy=name desc` or `?orderBy=name asc,created_time desc`
- Fields without direction default to ascending: `?orderBy=name` → sorts by `name asc`
- Default ordering when `orderBy` is omitted: `created_time desc`

**Note**: Violating constraints returns a `400 Bad Request` response with [RFC 9457 Problem Details](https://datatracker.ietf.org/doc/html/rfc9457) format.

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

When an OpenAPI schema is configured (see [deployment.md](deployment.md#configuring-schema-validation) for setup), the API validates cluster and nodepool `spec` fields on every create and update request. If no schema is configured, all specs are accepted without validation. When a schema is configured:

- `POST /clusters` and `POST /nodepools` validate `spec` against `ClusterSpec` or `NodePoolSpec` from the schema
- `PATCH /clusters/{id}` and `PATCH /nodepools/{id}` validate the merged result
- Invalid specs return a `400` with validation details in the error response

The schema is configured via `--server-openapi-schema-path` or the `validationSchema` section in the Helm chart. See [Validation Schema](../openapi/README.md#validation-schema) for details.

## Statuses Endpoint vs. Resource Endpoint

- `GET /clusters/{id}` returns the cluster with **aggregated** status conditions (`Reconciled`, `LastKnownReconciled`, and per-adapter conditions synthesized from adapter reports).
- `GET /clusters/{id}/statuses` returns the **raw adapter status records** — one per adapter that has reported. These are the individual reports, not the aggregated view.

The same distinction applies to nodepools.

## Error Responses

All error responses use the [RFC 9457](https://www.rfc-editor.org/rfc/rfc9457) Problem Details format with content type `application/problem+json`.

### Fields

| Field       | Type     | Always present | Description |
|-------------|----------|----------------|-------------|
| `type`      | string   | Yes            | URI reference identifying the problem type |
| `title`     | string   | Yes            | Short human-readable summary |
| `status`    | integer  | Yes            | HTTP status code |
| `detail`    | string   | Yes             | Human-readable explanation specific to this occurrence |
| `code`      | string   | No             | Machine-readable error code in `HYPERFLEET-CAT-NUM` format |
| `timestamp` | string   | No             | RFC 3339 timestamp of when the error occurred |
| `trace_id`  | string   | No             | Distributed trace ID for correlation (from `X-Request-Id` header) |
| `instance`  | string   | No             | URI reference for this specific occurrence |
| `errors`    | array    | No             | Field-level validation errors (see below) |

### Error Code Categories

Error codes follow the `HYPERFLEET-CAT-NUM` format:

| Category | Meaning |
|----------|---------|
| `VAL`    | Request validation failures |
| `AUT`    | Authentication errors |
| `NTF`    | Resource not found |
| `CNF`    | Resource conflicts |
| `LMT`    | Rate limiting |
| `INT`    | Internal server errors |
| `SVC`    | Upstream service errors |

### Example: Validation Error (400)

<details>
<summary>JSON response</summary>

```json
{
  "type": "https://api.hyperfleet.io/errors/validation-error",
  "title": "Validation failed",
  "status": 400,
  "detail": "Request body validation failed",
  "code": "HYPERFLEET-VAL-003",
  "timestamp": "2025-01-01T12:00:00Z",
  "trace_id": "abc123-def456",
  "instance": "/api/hyperfleet/v1/clusters",
  "errors": [
    {
      "field": "name",
      "message": "name is required"
    },
    {
      "field": "spec",
      "message": "spec must not be null",
      "constraint": "required"
    }
  ]
}
```

</details>

### Example: Not Found (404)

```json
{
  "type": "https://api.hyperfleet.io/errors/not-found",
  "title": "Not found",
  "status": 404,
  "detail": "Cluster with id='2abc123...' not found",
  "code": "HYPERFLEET-NTF-001",
  "timestamp": "2025-01-01T12:00:00Z"
}
```

### Example: Conflict (409)

```json
{
  "type": "https://api.hyperfleet.io/errors/conflict",
  "title": "Conflict",
  "status": 409,
  "detail": "Cannot create nodepool: parent cluster is being deleted",
  "code": "HYPERFLEET-CNF-001",
  "timestamp": "2025-01-01T12:00:00Z"
}
```

## Related Documentation

- [Example Usage](../README.md#example-usage) - Practical examples
- [Authentication](authentication.md) - API authentication
- [Database](database.md) - Database schema
