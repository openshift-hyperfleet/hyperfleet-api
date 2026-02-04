# Operational Runbook

This runbook provides operational procedures for managing HyperFleet API in production environments.

## Service Overview

HyperFleet API is a REST service that manages HyperFleet cluster and nodepool resources. It exposes:

- **API Server**: Port 8000 - REST API endpoints
- **Health Server**: Port 8080 - Liveness (`/healthz`) and readiness (`/readyz`) probes
- **Metrics Server**: Port 9090 - Prometheus metrics (`/metrics`)

### Architecture Diagram

```text
                                 ┌─────────────────────────────────────┐
                                 │         hyperfleet-api Pod          │
                                 │                                     │
    ┌─────────────┐              │  ┌─────────────────────────────┐   │
    │   Clients   │──────────────┼─▶│     API Server (:8000)      │   │
    │  (OCM CLI)  │    REST API  │  │  /api/hyperfleet/v1/*       │   │
    └─────────────┘              │  └──────────────┬──────────────┘   │
                                 │                 │                   │
    ┌─────────────┐              │  ┌──────────────▼──────────────┐   │
    │ Kubernetes  │──────────────┼─▶│   Health Server (:8080)     │   │
    │   Probes    │   HTTP GET   │  │  /healthz  /readyz          │   │
    └─────────────┘              │  └──────────────┬──────────────┘   │
                                 │                 │                   │
    ┌─────────────┐              │  ┌──────────────▼──────────────┐   │
    │ Prometheus  │──────────────┼─▶│  Metrics Server (:9090)     │   │
    │             │   Scrape     │  │  /metrics                   │   │
    └─────────────┘              │  └─────────────────────────────┘   │
                                 │                 │                   │
                                 └─────────────────┼───────────────────┘
                                                   │
                                                   ▼
                                 ┌─────────────────────────────────────┐
                                 │            PostgreSQL               │
                                 │      (clusters, nodepools)          │
                                 └─────────────────────────────────────┘
```

## Health Check Interpretation

### Liveness Probe (`/healthz`)

The liveness probe indicates whether the application process is alive and responsive.

| Response | Status | Meaning |
|----------|--------|---------|
| `200 OK` | `{"status": "ok"}` | Process is alive and responsive |
| `503 Service Unavailable` | Error details | Process has a fatal error, Kubernetes will restart the pod |

**When liveness fails:**
- The pod will be restarted by Kubernetes
- Check logs for fatal errors or panics
- This should be rare; frequent restarts indicate a serious issue

### Readiness Probe (`/readyz`)

The readiness probe indicates whether the application is ready to receive traffic.

| Response | Status | Meaning |
|----------|--------|---------|
| `200 OK` | `{"status": "ok"}` | Ready to receive traffic |
| `503 Service Unavailable` | `{"status": "not_ready"}` | Still initializing or dependencies unavailable |
| `503 Service Unavailable` | `{"status": "shutting_down"}` | Graceful shutdown in progress |

**Readiness checks include:**
- Application initialization complete
- Database connection available and responding to pings
- Not in shutdown state

**When readiness fails:**
- Pod is removed from service endpoints (no traffic routed)
- Check database connectivity first
- Verify all required environment variables are set
- Check startup logs for initialization errors

## Common Operational Procedures

### Restarting the Service

#### Single Pod Restart

```bash
# Delete a specific pod (Kubernetes will recreate it)
kubectl delete pod <pod-name> -n hyperfleet-system

# Or rollout restart the entire deployment
kubectl rollout restart deployment/hyperfleet-api -n hyperfleet-system
```

#### Verify Restart Success

```bash
# Watch pods come up
kubectl get pods -n hyperfleet-system -w

# Check readiness
kubectl get pods -n hyperfleet-system -o jsonpath='{.items[*].status.conditions[?(@.type=="Ready")].status}'

# Verify health endpoints
kubectl port-forward svc/hyperfleet-api-health 8080:8080 -n hyperfleet-system &
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
```

### Scaling the Service

#### Manual Scaling

```bash
# Scale up
kubectl scale deployment/hyperfleet-api --replicas=5 -n hyperfleet-system

# Scale down
kubectl scale deployment/hyperfleet-api --replicas=2 -n hyperfleet-system
```

#### Verify Scaling

```bash
# Check replica count
kubectl get deployment hyperfleet-api -n hyperfleet-system

# Verify all pods are ready
kubectl get pods -n hyperfleet-system -l app=hyperfleet-api
```

### Database Operations

#### Check Database Connectivity

```bash
# Check readiness probe (includes DB connectivity check)
kubectl port-forward svc/hyperfleet-api-health 8080:8080 -n hyperfleet-system &
curl http://localhost:8080/readyz

# If readiness returns "Database ping failed", use a debug pod to test connectivity
kubectl run pg-debug --rm -it --image=postgres:15-alpine --restart=Never -n hyperfleet-system -- \
  pg_isready -h <db-host> -p <db-port>
```

#### Database Connection Pool Issues

If you see `connection refused` or `too many connections` errors:

1. Check current connection count on database
2. Verify `DB_MAX_OPEN_CONNS` and `DB_MAX_IDLE_CONNS` settings
3. Consider scaling down replicas to reduce connection load
4. Check for connection leaks in recent deployments

#### Database Migrations

Migrations run automatically via an init container (`db-migrate`) before the main application starts. This happens on every deployment.

To manually run migrations (rarely needed):

```bash
# Run a one-off migration job
kubectl run hyperfleet-migrate --rm -it \
  --image=quay.io/openshift-hyperfleet/hyperfleet-api:latest \
  --restart=Never \
  -n hyperfleet-system \
  --overrides='{"spec":{"containers":[{"name":"hyperfleet-migrate","image":"quay.io/openshift-hyperfleet/hyperfleet-api:latest","command":["/app/hyperfleet-api","migrate"],"volumeMounts":[{"name":"secrets","mountPath":"/build/secrets","readOnly":true}]}],"volumes":[{"name":"secrets","secret":{"secretName":"hyperfleet-db-external"}}]}}' \
  -- /app/hyperfleet-api migrate
```

Or trigger a rollout restart to re-run the init container:

```bash
kubectl rollout restart deployment/hyperfleet-api -n hyperfleet-system
```

### Log Analysis

#### View Real-time Logs

```bash
# Single pod
kubectl logs -f deployment/hyperfleet-api -n hyperfleet-system

# All pods
kubectl logs -f -l app=hyperfleet-api -n hyperfleet-system --max-log-requests=10
```

#### Search for Errors

```bash
# Recent errors
kubectl logs deployment/hyperfleet-api -n hyperfleet-system --since=1h | grep -i error

# Structured log query (if using JSON logs)
kubectl logs deployment/hyperfleet-api -n hyperfleet-system --since=1h | jq 'select(.level == "error")'
```

## Troubleshooting Guide

### Pod Not Starting

**Symptoms:** Pod stuck in `Pending`, `ContainerCreating`, or `CrashLoopBackOff`

**Diagnosis:**
```bash
kubectl describe pod <pod-name> -n hyperfleet-system
kubectl get events -n hyperfleet-system --sort-by='.lastTimestamp' | tail -20
```

**Common causes:**
- **ImagePullBackOff**: Check image name, tag, and registry credentials
- **Insufficient resources**: Check node capacity and resource requests
- **ConfigMap/Secret not found**: Verify all required configs exist

### Pod Crashing on Startup

**Symptoms:** `CrashLoopBackOff` status, restarts > 0

**Diagnosis:**
```bash
# Check previous container logs
kubectl logs <pod-name> -n hyperfleet-system --previous

# Check events
kubectl describe pod <pod-name> -n hyperfleet-system
```

**Common causes:**
- Missing or invalid environment variables
- Database connection failure
- Invalid configuration file
- Port already in use (unlikely in Kubernetes)

### High Latency

**Symptoms:** Slow API responses, timeouts

**Diagnosis:**
```bash
# Check request duration metrics
curl -s http://<metrics-endpoint>:9090/metrics | grep api_inbound_request_duration

# Check pod resource usage
kubectl top pods -n hyperfleet-system
```

**Common causes:**
- Database query performance issues
- Insufficient CPU/memory resources
- Network latency to database
- High concurrent request load

### High Error Rate

**Symptoms:** Increased 5xx responses, error logs

**Diagnosis:**
```bash
# Check error count by path and code
curl -s http://<metrics-endpoint>:9090/metrics | grep api_inbound_request_count

# Review error logs
kubectl logs deployment/hyperfleet-api -n hyperfleet-system --since=15m | grep -i error
```

**Common causes:**
- Database connection issues
- Invalid request data
- Upstream service failures
- Resource exhaustion

### Database Connection Errors

**Symptoms:** `connection refused`, `no such host`, `connection reset`

**Diagnosis:**
```bash
# Check readiness probe (includes DB check)
kubectl port-forward svc/hyperfleet-api-health 8080:8080 -n hyperfleet-system &
curl http://localhost:8080/readyz

# Test connectivity using a debug pod
kubectl run pg-debug --rm -it --image=postgres:15-alpine --restart=Never -n hyperfleet-system -- \
  pg_isready -h <db-host> -p <db-port>

# Check database secret
kubectl get secret hyperfleet-db -n hyperfleet-system -o yaml
```

**Resolution:**
1. Verify database host and port are correct
2. Check network policies allow egress to database
3. Verify database credentials are valid
4. Check database is running and accepting connections
5. Verify SSL settings match database requirements

### Memory Issues

**Symptoms:** OOMKilled, high memory usage

**Diagnosis:**
```bash
# Check memory usage
kubectl top pods -n hyperfleet-system

# Check for OOMKilled events
kubectl get events -n hyperfleet-system | grep -i oom
```

**Resolution:**
1. Increase memory limits in deployment
2. Check for memory leaks (increasing memory over time)
3. Review query patterns that may load large datasets

## Recovery Procedures

### Complete Service Recovery

If the service is completely down:

1. **Check namespace exists:**
   ```bash
   kubectl get namespace hyperfleet-system
   ```

2. **Check deployment exists:**
   ```bash
   kubectl get deployment hyperfleet-api -n hyperfleet-system
   ```

3. **Force recreate all pods:**
   ```bash
   kubectl rollout restart deployment/hyperfleet-api -n hyperfleet-system
   ```

4. **Verify recovery:**
   ```bash
   kubectl rollout status deployment/hyperfleet-api -n hyperfleet-system
   ```

### Database Recovery

If database is unavailable:

1. **Verify database status** (external DB or PostgreSQL pod)
2. **Check connectivity** from API pods
3. **If using built-in PostgreSQL:**
   ```bash
   kubectl rollout restart statefulset/hyperfleet-postgresql -n hyperfleet-system
   ```
4. **Wait for readiness probes to pass** before routing traffic

### Rollback to Previous Version

```bash
# View rollout history
kubectl rollout history deployment/hyperfleet-api -n hyperfleet-system

# Rollback to previous version
kubectl rollout undo deployment/hyperfleet-api -n hyperfleet-system

# Rollback to specific revision
kubectl rollout undo deployment/hyperfleet-api -n hyperfleet-system --to-revision=2
```

## Escalation Paths

### Severity Levels

| Level | Description | Response Time | Example |
|-------|-------------|---------------|---------|
| **P1 - Critical** | Complete service outage | Immediate | All pods crashing, database unavailable |
| **P2 - High** | Degraded service | 30 minutes | High error rate, significant latency |
| **P3 - Medium** | Minor impact | 4 hours | Single pod issues, non-critical errors |
| **P4 - Low** | No user impact | Next business day | Log noise, documentation issues |

### Escalation Contacts

For all HyperFleet issues, escalate via the team Slack channel:

- **Channel**: [#hcm-hyperfleet-team](https://redhat.enterprise.slack.com/archives/C0916E39DQV)

### When to Escalate

- **Escalate immediately** if:
  - Complete service outage affecting users
  - Data integrity issues suspected
  - Security incident detected
  - Unable to diagnose issue within 30 minutes

- **Escalate within 1 hour** if:
  - Partial outage or degraded performance
  - Issue requires access you don't have
  - Root cause is unclear after initial investigation

## Related Documentation

- [Deployment Guide](deployment.md) - Deployment and configuration
- [Metrics Documentation](metrics.md) - Prometheus metrics reference
- [Development Guide](development.md) - Local development setup
