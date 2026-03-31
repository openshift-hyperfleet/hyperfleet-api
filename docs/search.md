# API Search and Filtering

This document describes how to search and filter resources in the HyperFleet API using TSL (Tree Search Language) queries.

## Overview

The HyperFleet API supports search and filtering capabilities through the `search` query parameter. All list endpoints (`GET /clusters`, `GET /nodepools`, etc.) accept TSL (Tree Search Language) queries that allow you to filter results using field comparisons, logical operators, and complex nested conditions.

## TSL Language Reference

The HyperFleet API uses the [Tree Search Language (TSL)](https://github.com/yaacov/tree-search-language) library for parsing search queries.

### Supported Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `=` | Equal | `name='test'` |
| `!=` | Not equal | `name!='old'` |
| `<` | Less than | `generation<10` |
| `<=` | Less than or equal | `generation<=5` |
| `>` | Greater than | `generation>1` |
| `>=` | Greater than or equal | `generation>=1` |
| `in` | In list | `name in ('c1','c2')` |
| `and` | Logical AND | `a='1' and b='2'` |
| `or` | Logical OR | `a='1' or a='2'` |
| `not` | Logical NOT | `not name='test'` |

### Query Value Syntax

- **String values**: Must be enclosed in single quotes: `name='my-cluster'`
- **Numeric values**: No quotes required: `generation>5`
- **Lists**: Comma-separated values in parentheses: `id in ('019466a0-8f8e-7abc-9def-0123456789ab', '019466a1-2b3c-7def-8abc-456789abcdef')`

## Searchable Fields

### Clusters

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `id` | string | Cluster ID | `id='019466a0-8f8e-7abc-9def-0123456789ab'` |
| `name` | string | Cluster name | `name='my-cluster'` |
| `generation` | integer | Spec version counter | `generation>1` |
| `created_by` | string | Creator email | `created_by='user@example.com'` |
| `updated_by` | string | Last updater email | `updated_by='user@example.com'` |
| `labels.<key>` | string | Label value | `labels.environment='production'` |
| `status.conditions.<Type>` | string | Condition status | `status.conditions.Ready='True'` |
| `status.conditions.<Type>.<Subfield>` | varies | Condition subfield | `status.conditions.Ready.last_updated_time < '...'` |

```bash
# Find cluster by name
curl -G "http://localhost:8000/api/hyperfleet/v1/clusters" \
  --data-urlencode "search=name='my-cluster'"

# Find clusters by multiple names
curl -G "http://localhost:8000/api/hyperfleet/v1/clusters" \
  --data-urlencode "search=name in ('cluster1', 'cluster2', 'cluster3')"
```

### NodePools

NodePools support the same searchable fields as Clusters, plus:

| Field | Type | Description | Example |
|-------|------|-------------|---------|
| `owner_id` | string | Parent cluster ID | `owner_id='019466a0-8f8e-7abc-9def-0123456789ab'` |

```bash
# Find nodepools by parent cluster ID
curl -G "http://localhost:8000/api/hyperfleet/v1/nodepools" \
  --data-urlencode "search=owner_id='019466a0-8f8e-7abc-9def-0123456789ab'"

# Find ready nodepools
curl -G "http://localhost:8000/api/hyperfleet/v1/nodepools" \
  --data-urlencode "search=status.conditions.Ready='True'"

# Find nodepools by label
curl -G "http://localhost:8000/api/hyperfleet/v1/nodepools" \
  --data-urlencode "search=labels.role='worker'"
```

## Labels Queries

Use `labels.<key>` syntax to filter by label values:

```bash
# Find production clusters
curl -G "http://localhost:8000/api/hyperfleet/v1/clusters" \
  --data-urlencode "search=labels.environment='production'"

# Find clusters in a specific region
curl -G "http://localhost:8000/api/hyperfleet/v1/clusters" \
  --data-urlencode "search=labels.region='us-east'"
```

Label keys must contain only lowercase letters (a-z), digits (0-9), and underscores (_).

## Status Condition Queries

Query resources by status conditions: `status.conditions.<Type>='<Status>'`

Condition types must be PascalCase (`Ready`, `Available`) and status must be `True` or `False` for resource conditions.

**Note:** Only the `=` operator is supported for condition queries. Other operators (`!=`, `<`, `>`, `in`, etc.) will return an error. The `NOT` operator is not supported with condition queries (`status.conditions.<Type>` or `status.conditions.<Type>.<Subfield>`) and will return a `400 Bad Request` error. Use the inverse condition value instead (e.g., `status.conditions.Ready='False'` rather than `NOT status.conditions.Ready='True'`).

```bash
# Find available clusters
curl -G "http://localhost:8000/api/hyperfleet/v1/clusters" \
  --data-urlencode "search=status.conditions.Available='True'"

# Find clusters that are not ready
curl -G "http://localhost:8000/api/hyperfleet/v1/clusters" \
  --data-urlencode "search=status.conditions.Ready='False'"
```

## Condition Subfield Queries

Query resources by condition subfields such as timestamps and observed generation:

```text
status.conditions.<Type>.<Subfield> <op> '<Value>'
```

### Supported Subfields

| Subfield | Type | Description |
|----------|------|-------------|
| `last_updated_time` | TIMESTAMPTZ | When the condition was last updated |
| `last_transition_time` | TIMESTAMPTZ | When the condition status last changed |
| `observed_generation` | INTEGER | Last generation processed by the condition |

### Supported Operators

Condition subfields support comparison operators: `=`, `!=`, `<`, `<=`, `>`, `>=`.

> **Note**: `status.conditions.<Type>` (without subfield) only supports the `=` operator.
> The `NOT` operator is not supported with any condition expression — neither `status.conditions.<Type>` nor `status.conditions.<Type>.<Subfield>` (e.g., `status.conditions.Ready.last_updated_time`). Using `NOT` with these expressions returns a `400 Bad Request` error. Restructure queries using `AND`/`OR` or the inverse condition value instead.

```bash
# Find clusters where Ready condition hasn't been updated in the last hour
curl -G "http://localhost:8000/api/hyperfleet/v1/clusters" \
  --data-urlencode "search=status.conditions.Ready.last_updated_time < '2026-03-06T14:00:00Z'"

# Find stale-ready resources (Sentinel selective polling use case)
curl -G "http://localhost:8000/api/hyperfleet/v1/clusters" \
  --data-urlencode "search=status.conditions.Ready='True' AND status.conditions.Ready.last_updated_time < '2026-03-06T14:00:00Z'"

# Find clusters with observed_generation below a threshold (uses unquoted integer)
curl -G "http://localhost:8000/api/hyperfleet/v1/clusters" \
  --data-urlencode "search=status.conditions.Ready.observed_generation < 5"
```

Time subfields require RFC3339 format (e.g., `2026-01-01T00:00:00Z`). Integer subfields use unquoted numeric values.

## Complex Queries

Combine multiple conditions using `and`, `or`, `not`, and parentheses `()`:

```bash
# Find ready production clusters
curl -G "http://localhost:8000/api/hyperfleet/v1/clusters" \
  --data-urlencode "search=status.conditions.Ready='True' and labels.environment='production'"

# Find clusters in dev or staging
curl -G "http://localhost:8000/api/hyperfleet/v1/clusters" \
  --data-urlencode "search=labels.environment in ('dev', 'staging')"

# Find ready clusters in production or staging
curl -G "http://localhost:8000/api/hyperfleet/v1/clusters" \
  --data-urlencode "search=status.conditions.Ready='True' and (labels.environment='production' or labels.environment='staging')"

# Find clusters that are not in production
curl -G "http://localhost:8000/api/hyperfleet/v1/clusters" \
  --data-urlencode "search=not labels.environment='production'"
```

Operator precedence: `()` > comparisons > `not` > `and` > `or`

## Other Common Use Cases

```bash
# Find non-production clusters
curl -G "http://localhost:8000/api/hyperfleet/v1/clusters" \
  --data-urlencode "search=labels.environment in ('dev', 'staging', 'test')"

# Find clusters created by specific user
curl -G "http://localhost:8000/api/hyperfleet/v1/clusters" \
  --data-urlencode "search=created_by='user@example.com'"

# Find clusters by multiple IDs
curl -G "http://localhost:8000/api/hyperfleet/v1/clusters" \
  --data-urlencode "search=id in ('019466a0-8f8e-7abc-9def-0123456789ab', '019466a1-2b3c-7def-8abc-456789abcdef', '019466a2-4c5d-7ef0-9abc-123456789def')"
```

## Error Handling

Invalid queries return `400 Bad Request` with error details:

```json
{
  "type": "https://api.hyperfleet.io/errors/bad-request",
  "title": "Bad Request",
  "status": 400,
  "detail": "Failed to parse search query: invalid-query",
  "code": "HYPERFLEET-VAL-005",
  "timestamp": "2025-01-15T10:30:00Z"
}
```
