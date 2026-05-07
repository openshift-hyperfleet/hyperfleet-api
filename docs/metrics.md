# HyperFleet Adapter Metrics

> **Audience:** Developers integrating with adapter metrics and SREs building dashboards.

All metrics are exposed on the `/metrics` endpoint (port 9090) in Prometheus format. No additional configuration is needed.

The Helm chart includes a **ServiceMonitor** template for automatic discovery by the [Prometheus Operator](https://github.com/prometheus-operator/prometheus-operator). It is enabled by default (`serviceMonitor.enabled: true`) and scrapes the `/metrics` endpoint every 30s with `honorLabels: true` to preserve the adapter's `component` and `version` labels. The template is only rendered when the Prometheus Operator CRDs (`monitoring.coreos.com/v1/ServiceMonitor`) are available on the cluster; otherwise it is silently skipped. See the Helm `values.yaml` for configuration options (interval, scrapeTimeout, labels, namespaceSelector).

## Adapter Metrics

The adapter exposes Prometheus metrics following the [HyperFleet Metrics Standard](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/standards/metrics.md) with the `hyperfleet_adapter_` prefix.

All adapter metrics include `component` and `version` as constant labels.

### Baseline Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `hyperfleet_adapter_build_info` | Gauge | `component`, `version`, `commit` | Build information (always 1) |
| `hyperfleet_adapter_up` | Gauge | `component`, `version` | Whether the adapter is up and running (1=up, 0=shutting down) |

### Event Processing Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `hyperfleet_adapter_events_processed_total` | Counter | `component`, `version`, `status` | Total CloudEvents processed. Status: `success`, `failed`, `skipped` |
| `hyperfleet_adapter_event_processing_duration_seconds` | Histogram | `component`, `version` | End-to-end event processing duration |
| `hyperfleet_adapter_errors_total` | Counter | `component`, `version`, `error_type` | Total errors by execution phase |

#### Status Values

| Status | Description |
|--------|-------------|
| `success` | Event processed successfully with resources applied |
| `skipped` | Event processed successfully but resources skipped (preconditions not met) |
| `failed` | Event processing failed due to an error |

#### Error Types

The `error_type` label on `hyperfleet_adapter_errors_total` corresponds to the execution phase where the error occurred:

| Error Type | Description |
|------------|-------------|
| `param_extraction` | Failed to extract parameters from the event |
| `preconditions` | Precondition evaluation error (not the same as precondition not met) |
| `resources` | Failed to apply Kubernetes resources |
| `post_actions` | Failed to execute post-actions (e.g., status reporting) |

### Resource Deletion Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `hyperfleet_adapter_resources_deleted_total` | Counter | `component`, `version`, `adapter_name`, `resource_type`, `status` | Total resource deletion operations by Kubernetes kind |
| `hyperfleet_adapter_resource_deletion_duration_seconds` | Histogram | `component`, `version`, `adapter_name`, `resource_type` | Duration of resource deletion operations by Kubernetes kind |
| `hyperfleet_adapter_resource_deletions_in_progress` | Gauge | `component`, `version`, `adapter_name`, `resource_type` | Number of resource deletions currently in progress by Kubernetes kind |

**ConstLabels**: `component`, `version`, and `adapter_name` are set at metric registration and remain constant for the adapter instance.

**Label `resource_type`**: Kubernetes resource kind (e.g., `Namespace`, `ServiceAccount`, `Deployment`). Individual resource instances are tracked via logs/traces, not metrics, per HyperFleet observability architecture.

#### Deletion Status Values

| Status | Description |
|--------|-------------|
| `success` | Resource deleted successfully, or already deleted/not found (desired state achieved) |
| `error` | Deletion operation failed |

#### Histogram Buckets

The `event_processing_duration_seconds` histogram uses the following buckets (in seconds), as recommended by the [adapter metrics standard](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/components/adapter/framework/adapter-metrics.md):

```text
0.1, 0.5, 1, 2, 5, 10, 30, 60, 120
```

### Example PromQL Queries

Event processing success rate:

```promql
(
  sum(rate(hyperfleet_adapter_events_processed_total{status="success"}[5m]))
  /
  sum(rate(hyperfleet_adapter_events_processed_total[5m]))
) * 100
```

p95 event processing duration:

```promql
histogram_quantile(0.95,
  sum by (component, version, le) (
    rate(hyperfleet_adapter_event_processing_duration_seconds_bucket[5m])
  )
)
```

Error rate by phase:

```promql
sum by (error_type) (rate(hyperfleet_adapter_errors_total[5m]))
```

## Broker Metrics

The adapter automatically registers Prometheus metrics from the [hyperfleet-broker](https://github.com/openshift-hyperfleet/hyperfleet-broker) library.

### Available Metrics

| Metric | Type | Labels | Description |
|--------|------|--------|-------------|
| `hyperfleet_broker_messages_consumed_total` | Counter | `topic`, `component`, `version` | Total messages consumed from the broker |
| `hyperfleet_broker_errors_total` | Counter | `topic`, `error_type`, `component`, `version` | Total message processing errors |
| `hyperfleet_broker_message_duration_seconds` | Histogram | `topic`, `component`, `version` | Message processing duration |

These metrics use the `hyperfleet_broker_` prefix and include the adapter's `component` and `version` labels.

## Alerting and Monitoring

For recommended alerting rules, thresholds, and operational PromQL queries, see [alerts.md](alerts.md).
