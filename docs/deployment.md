# Deployment Guide

This guide covers building container images and deploying HyperFleet API to Kubernetes using Helm.

## Container Image

### Building Images

Build and push container images:

```bash
# Build container image with default tag
make image

# Build with custom tag
make image IMAGE_TAG=v1.0.0

# Build and push to default registry
make image-push

# Build and push to personal Quay registry (for development)
QUAY_USER=myuser make image-dev
```

### Image Registry Configuration

The `image.registry` value defaults to `CHANGE_ME` - a placeholder that intentionally prevents accidental deployments with an incorrect registry. You **must** set this to your actual container registry before deploying.

#### Image Locations by Environment

| Environment | Image |
|-------------|-------|
| Development | `quay.io/<your-username>/hyperfleet-api:dev-<sha>` |
| Staging | `quay.io/openshift-hyperfleet/hyperfleet-api:v<version>` |
| Production | `quay.io/openshift-hyperfleet/hyperfleet-api:v<version>` |

#### Example values.yaml

Personal development image:
```yaml
image:
  registry: quay.io
  repository: user/hyperfleet-api
  tag: dev-abc1234
```

Production/Staging (official image):
```yaml
image:
  registry: quay.io
  repository: openshift-hyperfleet/hyperfleet-api
  tag: v1.2.3
```

### Custom Registry

To use a custom container registry:

```bash
# Build with custom registry
make image \
  IMAGE_REGISTRY=your-registry.io/yourorg \
  IMAGE_TAG=v1.0.0

# Push to custom registry
podman push your-registry.io/yourorg/hyperfleet-api:v1.0.0
```

## Configuration

HyperFleet API is configured via environment variables and configuration files.

### Configuration Methods

**Kubernetes deployments (recommended):**
- Non-sensitive config: ConfigMap (automatically created by Helm Chart from `values.yaml`)
- Sensitive data: Secrets with `secretKeyRef` (Kubernetes best practice, automatic via Helm Chart)

**Local development:**
- Configuration file: `./configs/config.yaml` or `--config` flag
- Environment variables: Direct values for quick testing

**See [Configuration Guide](config.md) for complete reference and priority rules.**

<details>
<summary><b>Configuration Flow in Kubernetes</b> (click to expand)</summary>

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

### Schema Validation

The API validates cluster and nodepool `spec` fields against an OpenAPI schema. This allows different providers (GCP, AWS, Azure) to have different spec structures.

- **Configuration:** `server.openapi_schema_path` (supports config file, env var, or CLI flag)
- **Default:** `openapi/openapi.yaml` (provider-agnostic base schema)

See [Configuration Guide](config.md) for all configuration options.

### Configuration

HyperFleet API configuration is managed through:
- **Helm Chart values** (`values.yaml`) for Kubernetes deployments
- **Configuration file** (`config.yaml`) for local development
- **Environment variables** for overrides

**For Kubernetes deployments**, the Helm Chart generates:
- **ConfigMap** from `values.yaml` for non-sensitive configuration
- **Secret mounts** for credentials (using `*_FILE` environment variables)

**Example: Setting required adapters (Helm):**
```bash
--set 'config.adapters.required.cluster={validation,dns,pullsecret,hypershift}' \
--set 'config.adapters.required.nodepool={validation,hypershift}'
```

**Example: Development override (environment variable):**
```bash
export HYPERFLEET_LOGGING_LEVEL=debug
```

**For complete configuration reference**, including all available settings, defaults, and validation rules, see:
- **[Configuration Guide](config.md)** - Complete reference for all configuration options
- **[Helm Chart values.yaml](../charts/values.yaml)** - Kubernetes-specific settings

## Kubernetes Deployment

### Using Helm Chart

The project includes a Helm chart for Kubernetes deployment with configurable PostgreSQL support.

#### Development Deployment

Deploy with built-in PostgreSQL for development and testing:

```bash
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --create-namespace \
  --set image.registry=quay.io \
  --set 'config.adapters.required.cluster={validation,dns,pullsecret,hypershift}' \
  --set 'config.adapters.required.nodepool={validation,hypershift}'
```

This creates:
- HyperFleet API deployment
- PostgreSQL StatefulSet
- Services for both components
- ConfigMaps and Secrets

#### Production Deployment

Deploy with external database (recommended for production):

##### Step 1: Create database secret

```bash
kubectl create secret generic hyperfleet-db-external \
  --namespace hyperfleet-system \
  --from-literal=db.host=<your-db-host> \
  --from-literal=db.port=5432 \
  --from-literal=db.name=hyperfleet \
  --from-literal=db.user=hyperfleet \
  --from-literal=db.password=<your-password>
```

##### Step 2: Deploy with external database

```bash
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --set image.registry=quay.io \
  --set database.postgresql.enabled=false \
  --set database.external.enabled=true \
  --set database.external.secretName=hyperfleet-db-external \
  --set 'config.adapters.required.cluster={validation,dns,pullsecret,hypershift}' \
  --set 'config.adapters.required.nodepool={validation,hypershift}'
```

**How it works:**
1. Helm Chart creates a ConfigMap with non-sensitive configuration
2. Your Secret (created in Step 1) contains database credentials
3. Helm Chart injects credentials as environment variables using `secretKeyRef`
4. Application reads credentials from environment variables
5. Credentials are never exposed in pod specs or ConfigMaps

This is the Kubernetes-native pattern for handling sensitive data securely.

#### Custom Image Deployment

Deploy with custom container image (e.g., `quay.io/myuser/hyperfleet-api:v1.0.0`):

```bash
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --set image.registry=quay.io \
  --set image.repository=myuser/hyperfleet-api \
  --set image.tag=v1.0.0 \
  --set 'config.adapters.required.cluster={validation,dns,pullsecret,hypershift}' \
  --set 'config.adapters.required.nodepool={validation,hypershift}'
```

**Note**: The `registry` should contain only the registry domain (e.g., `quay.io`, `docker.io`). The `repository` includes the organization and image name (e.g., `myuser/hyperfleet-api`).

#### Upgrade Deployment

Upgrade to a new version:

```bash
helm upgrade hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --set image.tag=v1.1.0
```

#### Uninstall

Remove the deployment:

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
| `config.server.jwt.enabled` | Enable JWT authentication | `true` |
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

Create a `values.yaml` file:

```yaml
# values.yaml
image:
  registry: quay.io
  repository: myuser/hyperfleet-api
  tag: v1.0.0

config:
  server:
    jwt:
      enabled: true

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

Deploy with custom values:
```bash
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --values values.yaml
```

## Helm Operations

### Check Deployment Status

```bash
# Get deployment status
helm status hyperfleet-api --namespace hyperfleet-system

# List all releases
helm list --namespace hyperfleet-system

# Check pods
kubectl get pods --namespace hyperfleet-system

# Check services
kubectl get svc --namespace hyperfleet-system
```

### View Logs

```bash
# View API logs
kubectl logs -f deployment/hyperfleet-api --namespace hyperfleet-system

# View logs from all pods
kubectl logs -f -l app=hyperfleet-api --namespace hyperfleet-system

# View PostgreSQL logs (if using built-in)
kubectl logs -f statefulset/hyperfleet-postgresql --namespace hyperfleet-system
```

### Troubleshooting

```bash
# Describe pod for events and status
kubectl describe pod <pod-name> --namespace hyperfleet-system

# Check deployment events
kubectl get events --namespace hyperfleet-system --sort-by='.lastTimestamp'

# Exec into pod for debugging
kubectl exec -it deployment/hyperfleet-api --namespace hyperfleet-system -- /bin/sh

# Check secrets
kubectl get secrets --namespace hyperfleet-system

# Verify ConfigMaps
kubectl get configmaps --namespace hyperfleet-system
```

## Health Checks

The deployment includes:
- Liveness probe: `GET /healthz` (port 8080) - Returns 200 if the process is alive
- Readiness probe: `GET /readyz` (port 8080) - Returns 200 when ready to receive traffic, 503 during startup/shutdown
- Metrics: `GET /metrics` (port 9090) - Prometheus metrics endpoint

## Scaling

Scale replicas:
```bash
# Manual scaling
kubectl scale deployment hyperfleet-api --replicas=3 --namespace hyperfleet-system

# Via Helm
helm upgrade hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --set replicaCount=3
```

Enable autoscaling via Helm values (`autoscaling.enabled=true`).

## Monitoring

Prometheus metrics available at `http://<service>:9090/metrics`.

### Prometheus Operator Integration

For clusters with Prometheus Operator, enable the ServiceMonitor to automatically discover and scrape metrics:

```bash
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --set image.registry=quay.io \
  --set serviceMonitor.enabled=true
```

If your Prometheus requires specific labels for service discovery, add them:

```bash
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --set image.registry=quay.io \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.labels.release=prometheus
```

To create the ServiceMonitor in a different namespace (e.g., `monitoring`):

```bash
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --set image.registry=quay.io \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.namespace=monitoring
```

## Production Deployment Checklist

Before deploying to production, ensure:

- [ ] **Database**: External managed database configured (Cloud SQL, RDS, Azure Database)
- [ ] **Secrets**: Database credentials stored in Secret (not ConfigMap)
- [ ] **Authentication**: JWT enabled (`config.server.jwt.enabled=true`)
- [ ] **Adapters**: Required adapters specified for cluster and nodepool
- [ ] **Resources**: CPU/memory limits and requests set
- [ ] **Replicas**: Multiple replicas configured (`replicaCount >= 2`)
- [ ] **Image**: Specific version tag (not `latest`)
- [ ] **Disruption**: PodDisruptionBudget enabled (`podDisruptionBudget.enabled=true`)
- [ ] **Monitoring**: ServiceMonitor enabled if using Prometheus Operator
- [ ] **TLS**: HTTPS enabled for API endpoint (optional)

## Production Best Practices

- Use external managed database (Cloud SQL, RDS, Azure Database) with automated backups
- Store all sensitive data in Kubernetes Secrets, never in ConfigMap or values.yaml
- Enable authentication with `config.server.jwt.enabled=true`
- Set resource limits and use multiple replicas for high availability
- Use specific image tags (semantic versioning) instead of `latest`
- Enable PodDisruptionBudget for zero-downtime during cluster maintenance
- Configure health probes with appropriate timeouts for your workload

## Complete Deployment Example

### GKE Deployment

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

# 4. Create database secret (for production)
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
  --set 'config.adapters.required.cluster={validation,dns,pullsecret,hypershift}' \
  --set 'config.adapters.required.nodepool={validation,hypershift}'

# 6. Verify deployment
kubectl get pods
kubectl logs -f deployment/hyperfleet-api

# 7. Access API (port-forward for testing)
kubectl port-forward svc/hyperfleet-api 8000:8000
curl http://localhost:8000/api/hyperfleet/v1/clusters
```

## Related Documentation

- [Development Guide](development.md) - Local development setup
- [Authentication](authentication.md) - Authentication configuration
