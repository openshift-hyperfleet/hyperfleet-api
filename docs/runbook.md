# HyperFleet Adapter Runbook

> **Audience:** On-call engineers managing the hyperfleet-adapter service.

---

## Table of Contents

1. [Service Overview](#service-overview)
2. [Health Checks](#health-checks)
3. [Failure Modes](#failure-modes)
   - [Adapter Fails to Start](#adapter-fails-to-start)
   - [Broker Connectivity Issues](#broker-connectivity-issues)
   - [Config Loading Failures](#config-loading-failures)
   - [Event Processing Failures](#event-processing-failures)
   - [HyperFleet API Failures](#hyperfleet-api-failures)
   - [Maestro Client Failures](#maestro-client-failures)
   - [Kubernetes Client Failures](#kubernetes-client-failures)
4. [Tracing / OpenTelemetry Issues](#tracing--opentelemetry-issues)
5. [Recovery Procedures](#recovery-procedures)
6. [Escalation Paths](#escalation-paths)

---

## Service Overview

The hyperfleet-adapter consumes CloudEvents from a message broker (Google Pub/Sub or RabbitMQ), evaluates preconditions, applies Kubernetes resources or Maestro ManifestWorks, and reports status back to the HyperFleet API.

**Ports:**
- `8080` — Health endpoints (`/healthz`, `/readyz`)
- `9090` — Prometheus metrics (`/metrics`)

**Startup sequence:**
1. Load adapter config and task config
2. Initialize OpenTelemetry tracing
3. Start health server and metrics server
4. Create HyperFleet API client
5. Create transport client (Maestro or Kubernetes)
6. Build executor
7. Create broker subscriber and subscribe to topic
8. Mark readiness (`/readyz` returns 200)

Any failure in steps 1–7 causes the process to exit with code 1.

---

## Health Checks

| Endpoint | Probe Type | Behavior |
|----------|-----------|----------|
| `/healthz` | Liveness | Always returns `200 OK` |
| `/readyz` | Readiness | Returns `200 OK` when config is loaded and broker is connected |

### Readiness checks

| Check | Meaning |
|-------|---------|
| `config` | Adapter and task configs loaded successfully |
| `broker` | Broker subscription established |

If `/readyz` returns `503`, inspect the response body for which check is failing:

```bash
kubectl exec <pod> -- curl -s localhost:8080/readyz | jq .
```

---

## Failure Modes

### Adapter Fails to Start

**Symptoms:** Pod is in `CrashLoopBackOff`. Exit code 1 in container logs.

**Common causes and resolution:**

| Log Pattern | Cause | Resolution |
|-------------|-------|------------|
| `"adapter config file path is required"` | Missing `--config` flag or `HYPERFLEET_ADAPTER_CONFIG` env | Check deployment args and ConfigMap mount |
| `"task config file path is required"` | Missing `--task-config` flag or `HYPERFLEET_TASK_CONFIG` env | Check deployment args and ConfigMap mount |
| `"failed to read adapter config file"` | ConfigMap not mounted or wrong path | Verify ConfigMap exists and volumeMount path matches |
| `"failed to parse adapter config YAML"` | Invalid YAML syntax | Validate config YAML offline with `yq` |
| `"unsupported apiVersion"` | Config version mismatch | Update config to match expected `apiVersion` |
| `"adapter version mismatch"` | `spec.adapter.version` doesn't match binary version | Update config or redeploy matching binary version |
| `"HyperFleet API base URL not configured"` | Missing `spec.clients.hyperfleetApi.baseUrl` | Set base URL in adapter config or `HYPERFLEET_API_BASE_URL` env |
| `"maestro config is required"` | Maestro transport selected but no config | Add `spec.clients.maestro` section to adapter config |
| `"maestro server address is required"` | Missing Maestro gRPC address | Set `grpcServerAddress` in maestro config |

**Steps:**
1. Check pod logs: `kubectl logs <pod> --previous`
2. Verify ConfigMaps exist: `kubectl get configmap -l app.kubernetes.io/name=hyperfleet-adapter`
3. Verify volume mounts: `kubectl describe pod <pod>` — look for mount paths
4. Validate config offline: `kubectl get configmap <name> -o yaml | yq .data`

---

### Broker Connectivity Issues

**Symptoms:** Pod starts but `/readyz` returns 503 with `broker: error`. Or pod crashes with broker-related errors.

| Log Pattern | Cause | Resolution |
|-------------|-------|------------|
| `"Missing required broker configuration"` | `subscription_id` or `topic` not set in adapter config | Check adapter config ConfigMap; set values directly in YAML (not via env vars for RabbitMQ) |
| `"Failed to create subscriber"` | Broker backend misconfiguration | Verify broker connection settings (Pub/Sub project, RabbitMQ URL) |
| `"Failed to subscribe to topic"` | Topic doesn't exist or permissions denied | Verify topic/exchange exists and adapter has access |
| `"Subscription error"` | Transient broker error | Usually recovers; monitor for frequency |
| `"Fatal subscription error, shutting down"` | Unrecoverable broker error | Check broker service health; adapter will restart via liveness probe |

**Steps:**
1. Check broker ConfigMap: `kubectl get configmap <release>-hyperfleet-adapter-broker -o yaml`
2. Check adapter config ConfigMap: `kubectl get configmap <release>-hyperfleet-adapter-config -o yaml`
3. For Google Pub/Sub: verify subscription exists and IAM permissions
4. For RabbitMQ:
   - Verify `broker.yaml` `exchange` matches the sentinel's `clients.broker.topic` — this is the most common misconfiguration
   - Verify `subscription_id` in the adapter config is unique per adapter instance (shared IDs cause competing consumers, not fan-out)
   - Check the queue exists and is bound to the exchange: `rabbitmqctl list_bindings`
   - Check the queue has consumers: `rabbitmqctl list_queues name messages consumers`
   - Verify TCP connectivity to RabbitMQ from the pod network
5. Check if broker service is healthy independently

---

### Config Loading Failures

**Symptoms:** Pod crashes immediately on startup with config-related errors.

| Log Pattern | Cause | Resolution |
|-------------|-------|------------|
| `"referenced file ... does not exist"` | Task config references a file not in the ConfigMap | Ensure all `ref:` files are included in the ConfigMap |
| `"CEL parse error"` | Invalid CEL expression in preconditions | Fix CEL syntax in task config |
| `"undefined template variable"` | Template references a variable not in params | Check param names match template variables |
| `"mutually exclusive"` | Conflicting config options (e.g. `build` and `buildRef`) | Use only one of the mutually exclusive options |
| `"manifest is required for maestro/kubernetes transport"` | Resource missing manifest definition | Add manifest to resource in task config |

**Steps:**
1. Check pod logs for the specific validation error
2. Pull the ConfigMap content and validate locally
3. Use the adapter's dry-run mode to test config changes before deploying:
   ```bash
   ./adapter serve --config adapter-config.yaml --task-config task-config.yaml --dry-run-event event.json
   ```

---

### Event Processing Failures

**Symptoms:** `hyperfleet_adapter_events_processed_total{status="failed"}` is increasing. Events are not being processed successfully.

All event failures are ACKed (not retried) to avoid infinite loops on non-transient errors.

| Log Pattern | Phase | Cause | Resolution |
|-------------|-------|-------|------------|
| `"Failed to parse event data"` | Params | Malformed CloudEvent payload | Check upstream event producer |
| `"failed to extract required parameter"` | Params | Missing field in event data | Verify event schema matches task config params |
| `"Precondition[...] evaluated: FAILED"` | Preconditions | API precondition check returned failure | Check HyperFleet API state for the resource |
| `"Precondition[...] evaluated: NOT_MET"` | Preconditions | Condition not satisfied (expected) | Normal flow — event skipped |
| `"failed to render manifest"` | Resources | Template rendering error | Fix Go template syntax in task config |
| `"failed to apply resource"` | Resources | Transport client error | See Maestro or K8s failure sections |
| `"PostAction[...] processed: FAILED"` | Post Actions | Status report API call failed | Check HyperFleet API connectivity |

**Steps:**
1. Check error metrics: `rate(hyperfleet_adapter_errors_total[5m])`
2. Identify the failing phase from `error_type` label
3. Check pod logs for the specific event ID and error details
4. For persistent failures, use dry-run to reproduce:
   ```bash
   ./adapter serve --config adapter-config.yaml --task-config task-config.yaml --dry-run-event failing-event.json
   ```

---

### HyperFleet API Failures

**Symptoms:** Post-action status updates fail. `hyperfleet_adapter_errors_total{error_type="post_actions"}` is increasing.

| Log Pattern | Cause | Resolution |
|-------------|-------|------------|
| `"HyperFleet API request returned retryable status"` | API returning 5xx, 408, or 429 | Check HyperFleet API health |
| `"API request failed: ... after N attempt(s)"` | All retries exhausted | API may be down or overloaded |
| `"context canceled"` | Request timed out | Increase `spec.clients.hyperfleetApi.timeout` or check API latency |

The adapter retries on 5xx, 408 (Request Timeout), and 429 (Too Many Requests) with configurable backoff (exponential, linear, or constant).

**Steps:**
1. Check HyperFleet API health: `kubectl get pods -l app=hyperfleet-api`
2. Check API response times from the adapter pod:
   ```bash
   kubectl exec <pod> -- curl -s -o /dev/null -w "%{http_code} %{time_total}s" http://hyperfleet-api:8000/healthz
   ```
3. Check for resource exhaustion on the API service
4. Review `retryAttempts` and `timeout` in adapter config

---

### Maestro Client Failures

**Symptoms:** Resource apply phase fails. Events fail with resource-related errors.

| Log Pattern | Cause | Resolution |
|-------------|-------|------------|
| `"consumer ... is not registered in Maestro"` | Target cluster consumer not registered | Register the consumer in Maestro before sending events |
| `"failed to create ManifestWork"` | Maestro API error | Check Maestro server health and logs |
| `"failed to parse CA certificate"` | Invalid TLS CA certificate | Verify cert files mounted at configured paths |
| `"failed to create Maestro work client"` | gRPC connection failure | Check Maestro gRPC endpoint connectivity |

**Steps:**
1. Verify Maestro server is running: `kubectl get pods -n maestro`
2. Test gRPC connectivity from the adapter pod:
   ```bash
   kubectl exec <pod> -- grpcurl -plaintext maestro-grpc.maestro.svc.cluster.local:8090 grpc.health.v1.Health/Check
   ```
3. For TLS issues, verify cert secret is mounted and certs are valid:
   ```bash
   kubectl exec <pod> -- openssl x509 -in /etc/maestro/certs/grpc/ca.crt -noout -dates
   ```
4. Verify consumer registration in Maestro for the target cluster

---

### Kubernetes Client Failures

**Symptoms:** Resource apply phase fails for Kubernetes transport tasks.

| Log Pattern | Cause | Resolution |
|-------------|-------|------------|
| `"failed to create kubernetes client"` | RBAC or kubeconfig issues | Check ServiceAccount and RBAC permissions |
| `K8sOperationError{Operation:"create",...}` | Resource creation failed | Check RBAC, resource quotas, namespace existence |
| `K8sOperationError{Operation:"update",...}` | Conflict on update | Usually transient; retried automatically |
| `"context canceled while waiting for resource deletion"` | Recreate timed out waiting for deletion | Resource may have finalizers preventing deletion |

Retryable K8s errors (automatically retried): timeouts, server unavailable, internal errors, rate limiting, network errors (connection refused, connection reset).
Non-retryable: forbidden, unauthorized, bad request, invalid, gone, method not supported, not acceptable.

**Steps:**
1. Check RBAC: `kubectl auth can-i --as=system:serviceaccount:<ns>:<sa> create <resource>`
2. Check resource quotas: `kubectl describe resourcequota -n <namespace>`
3. Check for finalizers blocking deletion: `kubectl get <resource> -o yaml | grep finalizers`

---

## Tracing / OpenTelemetry Issues

**Symptoms:** OTLP export errors in logs at startup or during event processing. Traces not appearing in the collector.

| Log Pattern | Cause | Resolution |
|-------------|-------|------------|
| `"failed to create OTLP exporter"` | Invalid endpoint or protocol | Verify `OTEL_EXPORTER_OTLP_ENDPOINT` is reachable and `OTEL_EXPORTER_OTLP_PROTOCOL` is `grpc` or `http/protobuf` |
| `"failed to export spans"` | Collector is down or unreachable | Check OTLP collector health; verify network connectivity from the pod |
| `"context deadline exceeded"` on spans | Slow or overloaded collector | Check collector resource usage; consider increasing export timeout |
| Traces going to stdout unexpectedly | No OTLP endpoint configured | Set `OTEL_EXPORTER_OTLP_ENDPOINT` to point to your collector |

**Default mismatch:** The binary defaults to tracing **enabled** (`HYPERFLEET_TRACING_ENABLED=true`), but the Helm chart defaults to tracing **disabled**. Running `adapter serve` locally without setting `HYPERFLEET_TRACING_ENABLED=false` will attempt OTLP export (falling back to stdout if no endpoint is set).

**Steps:**
1. Check if tracing is enabled: look for `"tracing enabled"` in startup logs
2. To disable tracing entirely: `HYPERFLEET_TRACING_ENABLED=false`
3. Verify collector connectivity:
   ```bash
   kubectl exec <pod> -- curl -s http://otel-collector:4317
   ```
4. Check Helm values: `tracing.enabled`, `tracing.otlpEndpoint`, `tracing.otlpProtocol`

---

## Recovery Procedures

### Restart the Adapter

```bash
kubectl rollout restart deployment/<release>-hyperfleet-adapter -n <namespace>
```

### Force Reprocess a Failed Event

Events are ACKed on failure and not automatically retried. To reprocess:
1. Identify the failed event from logs (look for `event_id`)
2. Republish the event to the broker topic. The event payload must conform to the [async API contract](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/components/broker/asyncapi.yaml).

   For Google Pub/Sub:
   ```bash
   gcloud pubsub topics publish <topic> --message '<event-json>'
   ```

   For RabbitMQ (the adapter's queue binding uses an empty routing key, so publish with `routing_key=""`):
   ```bash
   rabbitmqadmin publish exchange=<exchange> routing_key="" payload='<event-json>'
   ```
3. Monitor `hyperfleet_adapter_events_processed_total` for the reprocessed event

### Roll Back a Deployment

```bash
kubectl rollout undo deployment/<release>-hyperfleet-adapter -n <namespace>
```

### Verify Recovery

```bash
# Check pod health
kubectl get pods -l app.kubernetes.io/name=hyperfleet-adapter

# Check readiness
kubectl exec <pod> -- curl -s localhost:8080/readyz | jq .

# Check metrics
kubectl exec <pod> -- curl -s localhost:9090/metrics | grep hyperfleet_adapter_up

# Verify events are being processed (confirms the adapter is doing work, not just alive).
# If the counter is not incrementing, confirm there are messages in the broker
# before concluding the adapter is stuck.
kubectl exec <pod> -- curl -s localhost:9090/metrics | grep hyperfleet_adapter_events_processed_total
```

---

## Escalation Paths

| Severity | Condition | Action |
|----------|-----------|--------|
| **P1 - Critical** | Adapter down across all replicas; no events processing | Page on-call SRE. Check broker and API dependencies. |
| **P2 - High** | High failure rate (>10% events failing for 15+ min) | Notify team. Investigate error_type in metrics. |
| **P3 - Medium** | Single replica crash-looping; other replicas healthy | Investigate logs. This may be a config or resource issue. |
| **P4 - Low** | Intermittent errors, auto-recovering | Monitor. Create ticket if pattern persists. |

### Escalation contacts

| Level | Team | When |
|-------|------|------|
| L1 | On-call SRE | Initial triage, restart, basic recovery |
| L2 | HyperFleet Adapter team | Config issues, event processing logic, transport client failures |
| L3 | HyperFleet Platform team | Broker infrastructure, Maestro server, HyperFleet API issues |

### How to escalate

- **Email**: [openshift-hyperfleet@redhat.com](mailto:openshift-hyperfleet@redhat.com)
- **Jira**: Create a ticket in the [HYPERFLEET](https://issues.redhat.com/projects/HYPERFLEET) project with component `adapter`
- **GitHub**: Open an issue at [openshift-hyperfleet/hyperfleet-adapter](https://github.com/openshift-hyperfleet/hyperfleet-adapter/issues)
