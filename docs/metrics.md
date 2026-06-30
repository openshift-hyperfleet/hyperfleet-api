# Metrics Documentation

This document describes all Prometheus metrics exposed by HyperFleet API, including their meanings, expected ranges, and example queries for common investigations.

## Metrics Endpoint

Metrics are exposed at:
- **Endpoint**: `/metrics`
- **Port**: 9090 (default, configurable via `--metrics-server-bindaddress`)
- **Format**: OpenMetrics/Prometheus text format

## Application Metrics

### Build Info

#### `hyperfleet_api_build_info`

**Type:** Gauge (always 1)

**Description:** Build information for the HyperFleet API component.

**Labels:**

| Label | Description | Example Values |
|-------|-------------|----------------|
| `component` | Component name | `api` |
| `version` | Application version (git sha) | `abc123`, `abc123-modified` |
| `commit` | Git commit SHA | `abc123` |
| `go_version` | Go runtime version | `go1.24.0` |

**Example output:**

```text
hyperfleet_api_build_info{component="api",version="abc123",commit="abc123",go_version="go1.24.0"} 1
```

### API Request Metrics

These metrics track all inbound HTTP requests to the API server.

#### `hyperfleet_api_requests_total`

**Type:** Counter

**Description:** Total number of HTTP requests served by the API.

**Labels:**

| Label | Description | Example Values |
|-------|-------------|----------------|
| `component` | Component name | `api` |
| `version` | Application version | `abc123` |
| `method` | HTTP method | `GET`, `POST`, `PUT`, `PATCH`, `DELETE` |
| `path` | Request path (with IDs replaced by `-`) | `/api/hyperfleet/v1/clusters/-` |
| `code` | HTTP response status code | `200`, `201`, `400`, `404`, `500` |

**Path normalization:** Object identifiers in paths are replaced with `-` to reduce cardinality. For example, `/api/hyperfleet/v1/clusters/abc123` becomes `/api/hyperfleet/v1/clusters/-`.

**Example output:**

```text
hyperfleet_api_requests_total{component="api",version="abc123",code="200",method="GET",path="/api/hyperfleet/v1/clusters"} 1523
hyperfleet_api_requests_total{component="api",version="abc123",code="200",method="GET",path="/api/hyperfleet/v1/clusters/-"} 8742
hyperfleet_api_requests_total{component="api",version="abc123",code="201",method="POST",path="/api/hyperfleet/v1/clusters"} 156
hyperfleet_api_requests_total{component="api",version="abc123",code="404",method="GET",path="/api/hyperfleet/v1/clusters/-"} 23
```

#### `hyperfleet_api_request_duration_seconds`

**Type:** Histogram

**Description:** Distribution of request processing times in seconds.

**Labels:** Same as `hyperfleet_api_requests_total`

**Buckets:** `0.005s`, `0.01s`, `0.025s`, `0.05s`, `0.1s`, `0.25s`, `0.5s`, `1s`, `2.5s`, `5s`, `10s`

**Derived metrics:**
- `hyperfleet_api_request_duration_seconds_sum` - Total time spent processing requests
- `hyperfleet_api_request_duration_seconds_count` - Number of requests measured
- `hyperfleet_api_request_duration_seconds_bucket` - Number of requests completed within each bucket

**Example output:**

```text
hyperfleet_api_request_duration_seconds_bucket{component="api",version="abc123",code="200",method="GET",path="/api/hyperfleet/v1/clusters",le="0.005"} 800
hyperfleet_api_request_duration_seconds_bucket{component="api",version="abc123",code="200",method="GET",path="/api/hyperfleet/v1/clusters",le="0.01"} 1000
hyperfleet_api_request_duration_seconds_bucket{component="api",version="abc123",code="200",method="GET",path="/api/hyperfleet/v1/clusters",le="0.025"} 1200
hyperfleet_api_request_duration_seconds_bucket{component="api",version="abc123",code="200",method="GET",path="/api/hyperfleet/v1/clusters",le="0.05"} 1350
hyperfleet_api_request_duration_seconds_bucket{component="api",version="abc123",code="200",method="GET",path="/api/hyperfleet/v1/clusters",le="0.1"} 1450
hyperfleet_api_request_duration_seconds_bucket{component="api",version="abc123",code="200",method="GET",path="/api/hyperfleet/v1/clusters",le="0.25"} 1490
hyperfleet_api_request_duration_seconds_bucket{component="api",version="abc123",code="200",method="GET",path="/api/hyperfleet/v1/clusters",le="0.5"} 1510
hyperfleet_api_request_duration_seconds_bucket{component="api",version="abc123",code="200",method="GET",path="/api/hyperfleet/v1/clusters",le="1"} 1520
hyperfleet_api_request_duration_seconds_bucket{component="api",version="abc123",code="200",method="GET",path="/api/hyperfleet/v1/clusters",le="2.5"} 1522
hyperfleet_api_request_duration_seconds_bucket{component="api",version="abc123",code="200",method="GET",path="/api/hyperfleet/v1/clusters",le="5"} 1523
hyperfleet_api_request_duration_seconds_bucket{component="api",version="abc123",code="200",method="GET",path="/api/hyperfleet/v1/clusters",le="10"} 1523
hyperfleet_api_request_duration_seconds_bucket{component="api",version="abc123",code="200",method="GET",path="/api/hyperfleet/v1/clusters",le="+Inf"} 1523
hyperfleet_api_request_duration_seconds_sum{component="api",version="abc123",code="200",method="GET",path="/api/hyperfleet/v1/clusters"} 45.23
hyperfleet_api_request_duration_seconds_count{component="api",version="abc123",code="200",method="GET",path="/api/hyperfleet/v1/clusters"} 1523
```

### Reconciliation Observability Metrics

These metrics track resources pending reconciliation — both normal lifecycle (create/update) and deletion lifecycle. The `is_delete` label distinguishes the two.

#### `hyperfleet_api_reconciliation_started_total`

**Type:** Counter

**Description:** Total number of resources that entered the unreconciled state (Reconciled condition transitioned to False). Incremented only after the transition is persisted to the database.

**Labels:**

| Label | Description | Example Values |
|-------|-------------|----------------|
| `resource_type` | Type of resource | `cluster`, `nodepool` |
| `is_delete` | Whether this is a deletion reconciliation | `true`, `false` |
| `component` | Component name (const) | `api` |
| `version` | Application version (const) | `abc123` |

**Example output:**

```text
hyperfleet_api_reconciliation_started_total{component="api",is_delete="false",resource_type="cluster",version="abc123"} 42
hyperfleet_api_reconciliation_started_total{component="api",is_delete="true",resource_type="nodepool",version="abc123"} 12
```

#### `hyperfleet_api_resource_pending_reconciliation`

**Type:** Gauge (Collector)

**Description:** Number of resources currently pending reconciliation (Reconciled=False). Computed on each Prometheus scrape via a SQL query against the database.

**Labels:** Same as `hyperfleet_api_reconciliation_started_total`

**Behavior at zero:** When no resources are pending for a given `(resource_type, is_delete)` combination, the series is absent rather than emitting 0. The `> 0` alert expressions handle this correctly.

#### `hyperfleet_api_resource_pending_reconciliation_stuck`

**Type:** Gauge (Collector)

**Description:** Number of resources pending reconciliation beyond the stuck threshold (default 10 minutes). Identifies resources whose Reconciled condition has been False for longer than the configured threshold.

**Labels:** Same as `hyperfleet_api_reconciliation_started_total`

**Configuration:** The stuck threshold is configurable via `--metrics-reconciliation-stuck-threshold` (default `10m`).

#### `hyperfleet_api_resource_pending_reconciliation_stuck_duration_seconds`

**Type:** Gauge (Collector)

**Description:** Maximum duration in seconds that any resource has been stuck pending reconciliation, per `(resource_type, is_delete)` combination.

**Labels:** Same as `hyperfleet_api_reconciliation_started_total`

**Example output:**

```text
hyperfleet_api_resource_pending_reconciliation{component="api",is_delete="false",resource_type="cluster",version="abc123"} 5
hyperfleet_api_resource_pending_reconciliation_stuck{component="api",is_delete="false",resource_type="cluster",version="abc123"} 2
hyperfleet_api_resource_pending_reconciliation_stuck_duration_seconds{component="api",is_delete="false",resource_type="cluster",version="abc123"} 1847.3
```

### Reconciliation Alerts

Two alerts are available via the PrometheusRule (requires `monitoring.prometheusRule.enabled=true` in Helm values):

| Alert | Severity | Condition | Description |
|-------|----------|-----------|-------------|
| `HyperFleetResourceReconciliationStuckWarning` | Warning | `stuck > 0` for 5m | Resources stuck pending reconciliation beyond threshold + 5m |
| `HyperFleetResourceReconciliationStuckCritical` | Critical | `stuck_duration_seconds > 1800` for 5m | Elapsed stuck duration exceeds 30m timeout (survives Prometheus restarts) |

**Note:** The critical alert uses the collector-reported `stuck_duration_seconds` gauge rather than Prometheus `for` duration, so it fires immediately after a Prometheus restart if a resource has been stuck beyond the timeout. Both alerts cover deletion and normal (create/update) reconciliation. The `is_delete` label allows separate alerting rules if needed.

## Go Runtime Metrics

The following metrics are automatically exposed by the Prometheus Go client library.

### Process Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `process_cpu_seconds_total` | Counter | Total user and system CPU time spent in seconds |
| `process_max_fds` | Gauge | Maximum number of open file descriptors |
| `process_open_fds` | Gauge | Number of open file descriptors |
| `process_resident_memory_bytes` | Gauge | Resident memory size in bytes |
| `process_start_time_seconds` | Gauge | Start time of the process since unix epoch |
| `process_virtual_memory_bytes` | Gauge | Virtual memory size in bytes |

### Go Runtime Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `go_gc_duration_seconds` | Summary | A summary of pause durations during GC cycles |
| `go_goroutines` | Gauge | Number of goroutines currently existing |
| `go_memstats_alloc_bytes` | Gauge | Bytes allocated and still in use |
| `go_memstats_alloc_bytes_total` | Counter | Total bytes allocated (even if freed) |
| `go_memstats_heap_alloc_bytes` | Gauge | Heap bytes allocated and still in use |
| `go_memstats_heap_idle_bytes` | Gauge | Heap bytes waiting to be used |
| `go_memstats_heap_inuse_bytes` | Gauge | Heap bytes in use |
| `go_memstats_heap_objects` | Gauge | Number of allocated objects |
| `go_memstats_heap_sys_bytes` | Gauge | Heap bytes obtained from system |
| `go_memstats_sys_bytes` | Gauge | Total bytes obtained from system |
| `go_threads` | Gauge | Number of OS threads created |

## Expected Ranges and Alerting Thresholds

### Request Rate

| Condition | Threshold | Severity | Description |
|-----------|-----------|----------|-------------|
| Normal | < 1000 req/s | - | Normal operating range |
| Warning | > 1000 req/s | Warning | High load, monitor closely |
| Critical | > 5000 req/s | Critical | Capacity limit approaching |

### Error Rate

| Condition | Threshold | Severity | Description |
|-----------|-----------|----------|-------------|
| Normal | < 1% | - | Normal error rate |
| Warning | 1-5% | Warning | Elevated errors, investigate |
| Critical | > 5% | Critical | High error rate, immediate action |

### Latency (P99)

| Condition | Threshold | Severity | Description |
|-----------|-----------|----------|-------------|
| Normal | < 500ms | - | Good response times |
| Warning | 500ms - 2s | Warning | Degraded performance |
| Critical | > 2s | Critical | Unacceptable latency |

### Memory Usage

| Condition | Threshold | Severity | Description |
|-----------|-----------|----------|-------------|
| Normal | < 70% of limit | - | Healthy memory usage |
| Warning | 70-85% of limit | Warning | Memory pressure |
| Critical | > 85% of limit | Critical | OOM risk |

### Goroutines

| Condition | Threshold | Severity | Description |
|-----------|-----------|----------|-------------|
| Normal | < 1000 | - | Normal goroutine count |
| Warning | 1000-5000 | Warning | High goroutine count |
| Critical | > 5000 | Critical | Possible goroutine leak |

## Example PromQL Queries

### Request Rate

```promql
# Total request rate (requests per second)
sum(rate(hyperfleet_api_requests_total[5m]))

# Request rate by pod/instance
sum(rate(hyperfleet_api_requests_total[5m])) by (instance)

# Request rate by endpoint
sum(rate(hyperfleet_api_requests_total[5m])) by (path)

# Request rate by status code
sum(rate(hyperfleet_api_requests_total[5m])) by (code)

# Request rate by method
sum(rate(hyperfleet_api_requests_total[5m])) by (method)
```

### Error Rate

```promql
# Overall error rate (5xx responses)
sum(rate(hyperfleet_api_requests_total{code=~"5.."}[5m])) /
sum(rate(hyperfleet_api_requests_total[5m])) * 100

# Error rate by endpoint
sum(rate(hyperfleet_api_requests_total{code=~"5.."}[5m])) by (path) /
sum(rate(hyperfleet_api_requests_total[5m])) by (path) * 100

# Client error rate (4xx responses)
sum(rate(hyperfleet_api_requests_total{code=~"4.."}[5m])) /
sum(rate(hyperfleet_api_requests_total[5m])) * 100
```

### Latency

```promql
# Average request duration (last 10 minutes)
rate(hyperfleet_api_request_duration_seconds_sum[10m]) /
rate(hyperfleet_api_request_duration_seconds_count[10m])

# Average request duration by endpoint
sum(rate(hyperfleet_api_request_duration_seconds_sum[5m])) by (path) /
sum(rate(hyperfleet_api_request_duration_seconds_count[5m])) by (path)

# P50 latency (approximate using histogram)
histogram_quantile(0.5, sum(rate(hyperfleet_api_request_duration_seconds_bucket[5m])) by (le))

# P90 latency
histogram_quantile(0.9, sum(rate(hyperfleet_api_request_duration_seconds_bucket[5m])) by (le))

# P99 latency
histogram_quantile(0.99, sum(rate(hyperfleet_api_request_duration_seconds_bucket[5m])) by (le))

# P99 latency by endpoint
histogram_quantile(0.99, sum(rate(hyperfleet_api_request_duration_seconds_bucket[5m])) by (le, path))
```

### Resource Usage

```promql
# Memory usage in MB
process_resident_memory_bytes / 1024 / 1024

# Memory usage trend (increase over 1 hour)
delta(process_resident_memory_bytes[1h]) / 1024 / 1024

# Goroutine count
go_goroutines

# Goroutine trend
delta(go_goroutines[1h])

# CPU usage rate
rate(process_cpu_seconds_total[5m])

# File descriptor usage percentage
process_open_fds / process_max_fds * 100
```

### Reconciliation Observability

```promql
# Resources entering unreconciled state per minute
sum by (resource_type, is_delete) (rate(hyperfleet_api_reconciliation_started_total[5m])) * 60

# Resources currently stuck pending reconciliation
hyperfleet_api_resource_pending_reconciliation_stuck

# Stuck resources — deletion only
hyperfleet_api_resource_pending_reconciliation_stuck{is_delete="true"}

# Maximum stuck duration by type
hyperfleet_api_resource_pending_reconciliation_stuck_duration_seconds

# All pending resources by type
hyperfleet_api_resource_pending_reconciliation
```

### Common Investigation Queries

```promql
# Slowest endpoints (average latency)
topk(10,
  sum(rate(hyperfleet_api_request_duration_seconds_sum[5m])) by (path) /
  sum(rate(hyperfleet_api_request_duration_seconds_count[5m])) by (path)
)

# Most requested endpoints
topk(10, sum(rate(hyperfleet_api_requests_total[5m])) by (path))

# Endpoints with highest error rate
topk(10,
  sum(rate(hyperfleet_api_requests_total{code=~"5.."}[5m])) by (path) /
  sum(rate(hyperfleet_api_requests_total[5m])) by (path)
)

# Percentage of requests taking longer than 1 second
1 - (sum(rate(hyperfleet_api_request_duration_seconds_bucket{le="1"}[5m])) /
sum(rate(hyperfleet_api_request_duration_seconds_count[5m])))
```

## Prometheus Operator Integration

If using Prometheus Operator, enable the ServiceMonitor in Helm values:

```yaml
serviceMonitor:
  enabled: true
  interval: 30s
  scrapeTimeout: 10s
  labels:
    release: prometheus  # Match your Prometheus selector
```

See [Deployment Guide](deployment.md#prometheus-operator-integration) for details.

## Google Managed Prometheus (GMP) Integration

If running on GKE with Google Managed Prometheus, enable the PodMonitoring resource in Helm values:

```yaml
monitoring:
  podMonitoring:
    enabled: true
    interval: 30s
```

This creates a `monitoring.googleapis.com/v1/PodMonitoring` resource that configures the GMP collector agent to scrape the `/metrics` endpoint directly from pods. The `serviceMonitor` and `podMonitoring` options are independent and can be enabled simultaneously.

## Grafana Dashboard

Example dashboard JSON for HyperFleet API monitoring is available in the architecture repository. Key panels to include:

1. **Request Rate** - Total requests per second over time
2. **Error Rate** - Percentage of 5xx responses
3. **Latency Distribution** - P50, P90, P99 latencies
4. **Request Duration Heatmap** - Visual distribution of request times
5. **Top Endpoints** - Most frequently accessed paths
6. **Memory Usage** - Resident memory over time
7. **Goroutines** - Goroutine count over time

## Related Documentation

- [Operational Runbook](runbook.md) - Troubleshooting and operational procedures
- [Deployment Guide](deployment.md) - Deployment and ServiceMonitor configuration
- [Development Guide](development.md) - Local development setup
