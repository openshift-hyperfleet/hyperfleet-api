# Deployment Guide

This guide covers deploying HyperFleet API to a Kubernetes cluster via Helm chart.

For running the binary directly on your machine (development, debugging), see the **[Development Guide](development.md)**.

---

## ⚠️ Production Security Warning

**IMPORTANT**: Default Helm values are for development. Production requires:

| Setting | Default | Production | Risk if not changed |
|---------|---------|------------|---------------------|
| JWT auth | `false` | `true` | ⚠️ Unauthenticated access |
| TLS | `false` | `true` | ⚠️ Plaintext credentials |
| Database | Built-in pod | External managed | Data loss on restart |

See [API Operator Guide](api-operator-guide.md) for complete production configuration.

---

## Prerequisites

Before deploying, ensure you have:

- **Kubernetes cluster** (1.25+)
- **Helm 3** CLI
- **PostgreSQL database** — either:
  - An external managed instance (Cloud SQL, RDS, Azure Database) for production, or
  - The chart's built-in PostgreSQL pod for evaluation and testing
- **Container image** — a released hyperfleet-api image, a pre-built image from your registry, or build your own. See [development.md](./development.md) for more.

---

## Quick Start

The fastest path to a running deployment. This uses the chart's built-in PostgreSQL and no authentication — suitable for evaluation and testing.

**Three values are required** (they have no usable defaults):

| Value | What to set | Example |
|-------|-------------|---------|
| `image.registry` | Container registry domain | `quay.io` |
| `image.repository` | Organization and image name | `redhat-services-prod/hyperfleet-tenant/hyperfleet/hyperfleet-api` |
| `image.tag` | Image version | `v1.0.0` |

**Deploy:**

```bash
helm install hyperfleet-api oci://quay.io/redhat-services-prod/hyperfleet-tenant/hyperfleet/hyperfleet-api-chart:<tag> \
  --namespace hyperfleet-system \
  --create-namespace \
  --set image.registry=quay.io \
  --set image.repository=redhat-services-prod/hyperfleet-tenant/hyperfleet/hyperfleet-api \
  --set image.tag=<tag>
```

> **Note:** You may also choose to install from the ./charts folder, if you've cloned this repository locally.

**Verify:**

```bash
kubectl get pods --namespace hyperfleet-system
kubectl port-forward svc/hyperfleet-api 8000:8000 --namespace hyperfleet-system
curl http://localhost:8000/api/hyperfleet/v1/clusters
```

This creates a HyperFleet API deployment, a PostgreSQL StatefulSet, and the necessary Services, ConfigMaps, and Secrets.

---

## Production Deployment

For production, use an external managed database and store credentials in a Kubernetes Secret.

### Step 1: Create database secret

```bash
kubectl create secret generic hyperfleet-db-external \
  --namespace hyperfleet-system \
  --from-literal=db.host=<your-db-host> \
  --from-literal=db.port=5432 \
  --from-literal=db.name=hyperfleet \
  --from-literal=db.user=hyperfleet \
  --from-literal=db.password=<your-password>
```

### Step 2: Deploy with external database

```bash
helm install hyperfleet-api oci://quay.io/redhat-services-prod/hyperfleet-tenant/hyperfleet/hyperfleet-api-chart:<tag> \
  --namespace hyperfleet-system \
  --create-namespace \
  --set image.registry=quay.io \
  --set image.repository=redhat-services-prod/hyperfleet-tenant/hyperfleet/hyperfleet-api \
  --set image.tag=<tag> \
  --set database.postgresql.enabled=false \
  --set database.external.enabled=true \
  --set database.external.secretName=hyperfleet-db-external
```

The chart injects database credentials as environment variables using `secretKeyRef` — credentials are never exposed in ConfigMaps or pod specs.

<details>
<summary><b>How configuration flows in Kubernetes</b> (click to expand)</summary>

```
┌─────────────────────────────────────────────────────────────┐
│                       Helm Chart                            │
│                                                             │
│  values.yaml                                                │
│    ├─ server.port, logging.level, etc.                      │
│    └─ database.external.secretName                          │
└──────────────────┬──────────────────────────────────────────┘
                   │
                   ├─────────────────┬────────────────────────┐
                   ▼                 ▼                        ▼
         ┌──────────────────┐ ┌─────────────┐     ┌───────────────┐
         │    ConfigMap     │ │   Secret    │     │  Deployment   │
         │                  │ │             │     │               │
         │ Non-sensitive:   │ │ Sensitive:  │     │ Env vars:     │
         │ - server.host    │ │ - db.host   │     │ - HYPERFLEET  │
         │ - server.port    │ │ - db.user   │     │   _CONFIG     │
         │ - logging.level  │ │ - db.pass   │     │ - secretKeyRef│
         └──────┬───────────┘ └──────┬──────┘     └───────┬───────┘
                │                    │                    │
                └────────────────────┴────────────────────┘
                                     │
                                     ▼
                    ┌─────────────────────────────────────┐
                    │              Pod                    │
                    │                                     │
                    │  Volume Mounts:                     │
                    │  - /etc/hyperfleet/config.yaml      │
                    │    (from ConfigMap)                 │
                    │                                     │
                    │  Environment Variables:             │
                    │  - HYPERFLEET_CONFIG=               │
                    │    /etc/hyperfleet/config.yaml      │
                    │  - HYPERFLEET_DATABASE_HOST=        │
                    │    (from Secret via secretKeyRef)   │
                    │  - HYPERFLEET_DATABASE_PASSWORD=    │
                    │    (from Secret via secretKeyRef)   │
                    └─────────────┬───────────────────────┘
                                  │
                                  ▼
                    ┌─────────────────────────────────────┐
                    │         Application                 │
                    │                                     │
                    │  1. Load config from file           │
                    │     (/etc/hyperfleet/config.yaml)   │
                    │  2. Apply environment variables     │
                    │  3. Apply CLI flags (if any)        │
                    │                                     │
                    │  Priority: Flags > Env Vars >       │
                    │    ConfigMap > Defaults             │
                    └─────────────────────────────────────┘
```

</details>

---

## Configuring Authentication

JWT authentication is **disabled by default** in the Helm chart. To enable it, set the `config.server.jwt.*` properties, like so:

```bash
helm install hyperfleet-api oci://quay.io/redhat-services-prod/hyperfleet-tenant/hyperfleet/hyperfleet-api-chart:<tag> \
  --namespace hyperfleet-system \
  --set image.registry=quay.io \
  --set image.repository=redhat-services-prod/hyperfleet-tenant/hyperfleet/hyperfleet-api \
  --set image.tag=v1.0.0 \
  --set config.server.jwt.enabled=true \
  --set-json 'config.server.jwt.configs=[{"issuer_url":"https://your-idp.example.com/auth/realms/your-realm","jwk_cert_url":"https://your-idp.example.com/auth/realms/your-realm/protocol/openid-connect/certs"}]'
```

| Value | Required when JWT enabled | Description |
|-------|---------------------------|-------------|
| `config.server.jwt.enabled` | Yes | Set to `true` |
| `config.server.jwt.configs` | Yes | List of issuer configs. Each requires `issuer_url` and `jwk_cert_url` or `jwk_cert_file`. See link below for all optional fields. |

See [Issuer configuration reference](authentication.md#issuer-configuration-reference) for the full field table, defaults, and [Caller identity for audit](authentication.md#caller-identity-for-audit) for identity header and claim details.

---

## Configuring Required Adapters

Adapters are external components (validation, DNS, pull-secret, HyperShift) that report status back to HyperFleet API. The `required` adapter lists define which adapters must report "ready" before a resource is considered **Reconciled**.

By default, no adapters are required (`[]`). For production, configure the adapters your deployment uses:

```bash
--set 'config.adapters.required.cluster={validation,dns,pullsecret,hypershift}' \
--set 'config.adapters.required.nodepool={validation,hypershift}'
```

Or in a values file:

```yaml
config:
  adapters:
    required:
      cluster:
        - validation
        - dns
        - pullsecret
        - hypershift
      nodepool:
        - validation
        - hypershift
```

---

## Configuring Schema Validation

The API can validate cluster and nodepool `spec` fields against a custom OpenAPI schema on every create/update request. This is **disabled by default**.

### Inline schema

Provide the schema content directly in your values file:

```yaml
validationSchema:
  enabled: true
  content: |
    openapi: 3.0.0
    info:
      title: My Validation Schema
      version: 1.0.0
    paths: {}
    components:
      schemas:
        ClusterSpec:
          type: object
          required: [region]
          properties:
            region:
              type: string
        NodePoolSpec:
          type: object
          required: [machine_type]
          properties:
            machine_type:
              type: string
```

### Existing ConfigMap

Reference a ConfigMap that already exists in the namespace (must contain an `openapi.yaml` key):

```yaml
validationSchema:
  enabled: true
  existingConfigMap: my-validation-schema
```

When enabled, the chart creates (or references) a ConfigMap with the schema, mounts it into the container, and configures the API to validate against it. The API **will fail to start** if the schema is invalid.

---

## Managing the Deployment

### Upgrade

```bash
helm upgrade hyperfleet-api oci://quay.io/redhat-services-prod/hyperfleet-tenant/hyperfleet/hyperfleet-api-chart:<tag> \
  --namespace hyperfleet-system \
  --set image.tag=v1.1.0
```

During upgrade, in case schema changes have occurred in the new version, a DB migration will be handled automatically. See [Migration](./database.md#migration-system).

### Uninstall

```bash
helm uninstall hyperfleet-api --namespace hyperfleet-system
```

## Helm Values

### Key Configuration Options

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.registry` | Container registry | `CHANGE_ME` (must be set explicitly) |
| `image.repository` | Image repository | `openshift-hyperfleet/hyperfleet-api` |
| `image.tag` | Image tag | `latest` |
| `image.pullPolicy` | Image pull policy | `Always` |
| `config.adapters.required.cluster` | Cluster adapters required for Ready state | `[]` |
| `config.adapters.required.nodepool` | Nodepool adapters required for Ready state | `[]` |
| `config.server.jwt.enabled` | Enable JWT authentication | `false` (Helm default; app default is `true`) |
| `config.server.tls.enabled` | Enable TLS on the API listener | `false` |
| `config.database.ssl.mode` | SSL mode for database connection | `disable` |
| `database.postgresql.enabled` | Enable built-in PostgreSQL | `true` |
| `database.external.enabled` | Use external database | `false` |
| `database.external.secretName` | Secret containing database credentials | `hyperfleet-db-external` |
| `serviceMonitor.enabled` | Enable Prometheus Operator ServiceMonitor | `false` |
| `serviceMonitor.interval` | Metrics scrape interval | `30s` |
| `serviceMonitor.scrapeTimeout` | Metrics scrape timeout | `10s` |
| `serviceMonitor.labels` | Additional labels for Prometheus selector | `{}` |
| `serviceMonitor.namespace` | Namespace for ServiceMonitor (if different) | `""` |
| `replicaCount` | Number of API replicas | `1` |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `512Mi` |
| `podDisruptionBudget.enabled` | Enable PodDisruptionBudget | `false` |
| `podDisruptionBudget.minAvailable` | Minimum available pods during disruption | `1` |
| `podDisruptionBudget.maxUnavailable` | Maximum unavailable pods during disruption | - |

### Custom Values File

For repeatable deployments, create a `values.yaml` file:

```yaml
image:
  registry: quay.io
  repository: redhat-services-prod/hyperfleet-tenant/hyperfleet/hyperfleet-api
  tag: <version>

config:
  server:
    jwt:
      enabled: true
      configs:
        - issuer_url: https://your-idp.example.com/auth/realms/your-realm
          jwk_cert_url: https://your-idp.example.com/auth/realms/your-realm/protocol/openid-connect/certs
          # See "Issuer configuration reference" in authentication.md

  adapters:
    required:
      cluster:
        - validation
        - dns
        - pullsecret
        - hypershift
      nodepool:
        - validation
        - hypershift

database:
  postgresql:
    enabled: false
  external:
    enabled: true
    secretName: hyperfleet-db-external

replicaCount: 3

resources:
  limits:
    cpu: 1000m
    memory: 1Gi
  requests:
    cpu: 500m
    memory: 512Mi
```

```bash
helm install hyperfleet-api oci://quay.io/redhat-services-prod/hyperfleet-tenant/hyperfleet/hyperfleet-api-chart:<tag> \
  --namespace hyperfleet-system \
  --values values.yaml
```

---

## Helm Values Reference

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.registry` | Container registry | `CHANGE_ME` (must be set) |
| `image.repository` | Image repository | `CHANGE_ME` (must be set) |
| `image.tag` | Image tag | `""` (must be set) |
| `image.pullPolicy` | Image pull policy | `Always` |
| `config.server.jwt.enabled` | Enable JWT authentication | `false` |
| `config.adapters.required.cluster` | Cluster adapters required for Reconciled state | `[]` |
| `config.adapters.required.nodepool` | Nodepool adapters required for Reconciled state | `[]` |
| `database.postgresql.enabled` | Enable built-in PostgreSQL | `true` |
| `database.external.enabled` | Use external database | `false` |
| `database.external.secretName` | Secret containing database credentials | `""` |
| `validationSchema.enabled` | Enable spec validation schema | `false` |
| `replicaCount` | Number of API replicas | `1` |
| `resources.limits.cpu` | CPU limit | `500m` |
| `resources.limits.memory` | Memory limit | `512Mi` |
| `podDisruptionBudget.enabled` | Enable PodDisruptionBudget | `false` |
| `podDisruptionBudget.minAvailable` | Minimum available pods during disruption | `1` |
| `serviceMonitor.enabled` | Enable Prometheus Operator ServiceMonitor | `false` |
| `serviceMonitor.interval` | Metrics scrape interval | `30s` |
| `serviceMonitor.scrapeTimeout` | Metrics scrape timeout | `10s` |
| `serviceMonitor.labels` | Additional labels for Prometheus selector | `{}` |
| `serviceMonitor.namespace` | Namespace for ServiceMonitor (if different) | `""` |

See [Configuration Guide](config.md) for the complete application configuration reference and [`charts/values.yaml`](../charts/values.yaml) for all Helm-specific settings.

---

## Operations

### Check Deployment Status

```bash
helm status hyperfleet-api --namespace hyperfleet-system
helm list --namespace hyperfleet-system
kubectl get pods --namespace hyperfleet-system
kubectl get svc --namespace hyperfleet-system
```

### View Logs

```bash
kubectl logs -f deployment/hyperfleet-api --namespace hyperfleet-system
kubectl logs -f -l app=hyperfleet-api --namespace hyperfleet-system

# PostgreSQL logs (if using built-in)
kubectl logs -f statefulset/hyperfleet-postgresql --namespace hyperfleet-system
```

### Troubleshooting

```bash
kubectl describe pod <pod-name> --namespace hyperfleet-system
kubectl get events --namespace hyperfleet-system --sort-by='.lastTimestamp'
kubectl exec -it deployment/hyperfleet-api --namespace hyperfleet-system -- /bin/sh
kubectl get secrets --namespace hyperfleet-system
kubectl get configmaps --namespace hyperfleet-system
```

### Health Checks

The deployment includes:

- Liveness probe: `GET /healthz` (port 8080) — returns 200 if the process is alive
- Readiness probe: `GET /readyz` (port 8080) — returns 200 when ready to receive traffic, 503 during startup/shutdown
- Metrics: `GET /metrics` (port 9090) — Prometheus metrics endpoint

### Scaling

```bash
# Manual scaling
kubectl scale deployment hyperfleet-api --replicas=3 --namespace hyperfleet-system

# Via Helm
helm upgrade hyperfleet-api oci://quay.io/redhat-services-prod/hyperfleet-tenant/hyperfleet/hyperfleet-api-chart:<tag> \
  --namespace hyperfleet-system \
  --set replicaCount=3
```

Enable autoscaling via Helm values (`autoscaling.enabled=true`).

### Monitoring

Prometheus metrics are available at `http://<service>:9090/metrics`.

#### Prometheus Operator Integration

```bash
# Enable ServiceMonitor
helm install hyperfleet-api oci://quay.io/redhat-services-prod/hyperfleet-tenant/hyperfleet/hyperfleet-api-chart:<tag> \
  --namespace hyperfleet-system \
  --set image.registry=quay.io \
  --set image.repository=redhat-services-prod/hyperfleet-tenant/hyperfleet/hyperfleet-api \
  --set image.tag=<tag> \
  --set serviceMonitor.enabled=true

# With custom Prometheus selector labels
--set serviceMonitor.labels.release=prometheus

# ServiceMonitor in a different namespace
--set serviceMonitor.namespace=monitoring
```

---

## Production Checklist

Before deploying to production, ensure:

- [ ] **Image**: Specific version tag set (not `latest` or empty)
- [ ] **Database**: External managed database configured (Cloud SQL, RDS, Azure Database)
- [ ] **Secrets**: Database credentials stored in a Secret (not ConfigMap)
- [ ] **Authentication**: JWT enabled with issuer and JWK URL configured
- [ ] **Adapters**: Required adapters specified for cluster and nodepool
- [ ] **Config file permissions**: Config files (`--config` / `HYPERFLEET_CONFIG`) must be operator-trusted — see [below](#configuration-file-security)
- [ ] **Resources**: CPU/memory limits and requests set
- [ ] **Replicas**: Multiple replicas configured (`replicaCount >= 2`)
- [ ] **Disruption**: PodDisruptionBudget enabled (`podDisruptionBudget.enabled=true`)
- [ ] **Monitoring**: ServiceMonitor enabled if using Prometheus Operator

## Production Best Practices

- **Environment**: Use default (ProductionEnv) for production deployments; never set `HYPERFLEET_ENV=development`
- **Database**: Use external managed database (Cloud SQL, RDS, Azure Database) with automated backups
- **Secrets**: Store all sensitive data in Kubernetes Secrets, never in ConfigMap or values.yaml
- **Authentication**: Enable JWT authentication with `config.server.jwt.enabled=true`
- **Identity**: Enable identity extraction based on JWT claims or HTTP headers as appropriate
- **Logging**: Use JSON format (`config.logging.format=json`) and set level to `info` for production
- **Tracing**: Enable distributed tracing for observability in production environments
- **Resources**: Set CPU/memory limits and use multiple replicas for high availability
- **Images**: Use specific image tags (semantic versioning) instead of `latest`
- **Disruption**: Enable PodDisruptionBudget for zero-downtime during cluster maintenance
- **Health**: Configure health probes with appropriate timeouts for your workload

### Configuration File Security

The configuration file path — set via `--config` or `HYPERFLEET_CONFIG` — is a trust boundary. The API validates configuration **content** on startup (unknown fields are rejected, required values are enforced, TLS/JWT/timeout settings are checked) and will refuse to start with an invalid configuration. However, **path and permission safety is the operator's responsibility**. The API reads whatever file the process can access at the given path without checking permissions or ownership.

Ensure configuration files are:

- Owned by the service account running the API (e.g., `root:root` or a dedicated user)
- Mode `0600` (owner read/write only) or `0640` if group-readable access is needed
- Never world-writable

In Helm deployments, the chart mounts the configuration as a ConfigMap volume at `/etc/hyperfleet/config.yaml` with default Kubernetes permissions, which satisfies these requirements. This guidance applies primarily to bare-metal or VM deployments where config files are managed directly on disk.

---

## Complete Example: GKE Deployment

```bash
# 1. Build and push image
export QUAY_USER=myuser
podman login quay.io
make image-dev

# 2. Get GKE credentials
gcloud container clusters get-credentials my-cluster \
  --zone=us-central1-a \
  --project=my-project

# 3. Create namespace
kubectl create namespace hyperfleet-system
kubectl config set-context --current --namespace=hyperfleet-system

# 4. Create database secret
kubectl create secret generic hyperfleet-db-external \
  --from-literal=db.host=10.10.10.10 \
  --from-literal=db.port=5432 \
  --from-literal=db.name=hyperfleet \
  --from-literal=db.user=hyperfleet \
  --from-literal=db.password=secretpassword

# 5. Deploy with Helm
helm install hyperfleet-api ./charts/ \
  --set image.registry=quay.io \
  --set image.repository=myuser/hyperfleet-api \
  --set image.tag=dev-abc123 \
  --set config.server.jwt.enabled=false \
  --set database.postgresql.enabled=false \
  --set database.external.enabled=true \
  --set database.external.secretName=hyperfleet-db-external \
  --set 'config.adapters.required.cluster={validation,dns,pullsecret,hypershift}' \
  --set 'config.adapters.required.nodepool={validation,hypershift}'

# 6. Verify deployment
kubectl get pods
kubectl logs -f deployment/hyperfleet-api

# 7. Access API (port-forward for testing)
kubectl port-forward svc/hyperfleet-api 8000:8000
curl http://localhost:8000/api/hyperfleet/v1/clusters
```

---

## Related Documentation

- [Configuration Guide](config.md) — Complete configuration reference
- [Authentication](authentication.md) — Authentication configuration
- [Development Guide](development.md) — Local execution, development setup, and workflows
