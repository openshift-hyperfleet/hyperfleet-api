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

### Aggregation logic

Description of the aggregation logic for the resource status conditions

- An API that stores resources entities (clusters, nodepools)
- A sentinel that polls the API for changes and triggers messages
- Instances of "adapters":
  - Read the messages
  - Reconcile the state with the world
  - Report back to the API, using statuses "conditions"

Resources keep track of its status, which is affected by the reports from adapters

- Each resource keeps a `generation` property that gets increased on every change
- Adapters associated with a resource, report their state as an array of adapter conditions
  - Three of these conditions are always mandatory : `Available`, `Applied`, `Health`
  - If one of the mandatory conditions is missing, the report is discarded
  - A `observed_generation` field indicating the generation associated with the report
  - `observed_time` for when the adapter work was done
  - If the reported `observed_generation` is lower than the already stored `observed_generation` for that adapter, the report is discarded
- Each resource has a list of associated "adapters" used to compute the aggregated status.conditions
- Each resource "status.conditions" is array property composed of:
  - The `Available` condition of each adapter, named as `<adapter-name>Successful`
  - 2 aggregated conditions: `Ready` and `Available` computed from the array of `Available` resource statuses conditions
    - Only `Available` condition from adapters is used to compute aggregated conditions

The whole API spec is at: <https://raw.githubusercontent.com/openshift-hyperfleet/hyperfleet-api/refs/heads/main/openapi/openapi.yaml>

The aggregation logic for a resource (cluster/nodepool) works as follows.

**Notation:**

- `X` = report's `observed_generation`
- `G` = resource's current `generation`
- `statuses[]` = all stored adapter condition reports
- `lut` = `last_update_time`
- `ltt` = `last_transition_time`
- `obs_gen` = `observed_generation`
- `obs_time` = report's `observed_time`
- `—` = no change

---

#### Discard / Reject Rules

Checked before any aggregation. A discarded or rejected report causes no state change.

| Rule | Condition | Outcome |
|---|---|---|
| `obs_gen` too high | report `observed_generation` > resource `generation` | Discarded |
| Stale adapter report | report `observed_generation` < adapter's stored `observed_generation` | Discarded |
| Missing mandatory conditions | Missing any of `Available`, `Applied`, `Health`, or value not in `{True, False, Unknown}` | Discarded |
| Available=Unknown | Report is valid but `Available=Unknown` | Discarded |

---

#### Lifecycle Events

| Event | Condition | Target | → status | → obs_gen | → lut | → ltt |
|---|---|---|---|---|---|---|
| Creation | — | `Ready` | `False` | `1` | `now` | `now` |
| Creation | — | `Available` | `False` | `1` | `now` | `now` |
| Change (→G) | Was `Ready=True` | `Ready` | `False` | `G` | `now` | `now` |
| Change (→G) | Was `Ready=False` | `Ready` | `False` | `G` | `now` | `—` |
| Change (→G) | — | `Available` | unchanged | unchanged | `—` | `—` |

---

#### Adapter Report Aggregation Matrix

The **Ready** check and **Available** check are independent — both can apply to the same incoming report.

##### Report `Available=True` (obs_gen = X)

| Target | Current State | Required Condition | → status | → lut | → ltt | → obs_gen |
|---|---|---|---|---|---|---|
| `Ready` | `Ready=True` | `X==G` AND all `statuses[].obs_gen==G` AND all `statuses[].status==True` | unchanged | `min(statuses[].lut)` | `—` | `—` |
| `Ready` | `Ready=False` | `X==G` AND all `statuses[].obs_gen==G` AND all `statuses[].status==True` | **`True`** | `min(statuses[].lut)` | `obs_time` | `—` |
| `Ready` | any | Conditions above not met | `—` | `—` | `—` | `—` |
| `Available` | `Available=False` | all `statuses[].obs_gen==X` | **`True`** | `min(statuses[].lut)` | `obs_time` | `X` |
| `Available` | `Available=True` | all `statuses[].obs_gen==X` | unchanged | `min(statuses[].lut)` | `—` | `X` |
| `Available` | any | Conditions above not met | `—` | `—` | `—` | `—` |

##### Report `Available=False` (obs_gen = X)

| Target | Current State | Required Condition | → status | → lut | → ltt | → obs_gen |
|---|---|---|---|---|---|---|
| `Ready` | `Ready=False` | `X==G` | unchanged | `min(statuses[].lut)` | `—` | `—` |
| `Ready` | `Ready=True` | `X==G` | **`False`** | `obs_time` | `obs_time` | `—` |
| `Ready` | any | Conditions above not met | `—` | `—` | `—` | `—` |
| `Available` | `Available=False` | all `statuses[].obs_gen==X` | unchanged | `min(statuses[].lut)` | `—` | `X` |
| `Available` | `Available=True` | all `statuses[].obs_gen==X` | **`False`** | `obs_time` | `obs_time` | `X` |
| `Available` | any | Conditions above not met | `—` | `—` | `—` | `—` |

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

All list endpoints support filtering using TSL (Tree Search Language) query syntax. Example:

```bash
curl -G http://localhost:8000/api/hyperfleet/v1/clusters \
  --data-urlencode "search=status.conditions.Ready='True' and labels.environment='production'"
```

See **[search.md](search.md)** for complete documentation.

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
- `last_updated_time` - When this condition was last refreshed (API-managed). For **Available**, always the evaluation time. For **Ready**: when Ready=True, the minimum of `last_report_time` across all required adapters that report Available=True at the current generation; when Ready=False, the evaluation time (so consumers can detect staleness).
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

## Related Documentation

- [Example Usage](../README.md#example-usage) - Practical examples
- [Authentication](authentication.md) - API authentication
- [Database](database.md) - Database schema
