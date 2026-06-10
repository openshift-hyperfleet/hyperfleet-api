# Deployment Guide

This guide covers two deployment modes:

- **[Kubernetes Deployment (Helm)](#kubernetes-deployment-helm)** — deploying to a cluster via Helm chart (partners, staging, production)
- **[Local Execution](#local-execution)** — running the binary directly on your machine (HF engineers, development, debugging)

---

## Kubernetes Deployment (Helm)

Deploy HyperFleet API to a Kubernetes cluster using the included Helm chart. Typical use cases: partner deployments, staging, production, engineer validation on a cluster.

### Container Image

#### Building Images

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

#### Image Registry Configuration

The `image.registry` value in [`charts/values.yaml`](../charts/values.yaml) defaults to `CHANGE_ME` — a placeholder that intentionally prevents accidental deployments with an incorrect registry. You **must** set this to your actual container registry before deploying.

| Environment | Image |
|-------------|-------|
| Development | `quay.io/<your-username>/hyperfleet-api:dev-<sha>` |
| Staging | `quay.io/openshift-hyperfleet/hyperfleet-api:v<version>` |
| Production | `quay.io/openshift-hyperfleet/hyperfleet-api:v<version>` |

Example `values.yaml` overrides:

```yaml
# Production/Staging (official image)
image:
  registry: quay.io
  repository: openshift-hyperfleet/hyperfleet-api
  tag: v1.2.3
  
# Personal development image
image:
  registry: quay.io
  repository: user/hyperfleet-api
  tag: dev-abc1234


```

#### Custom Registry

```bash
make image \
  IMAGE_REGISTRY=your-registry.io/yourorg \
  IMAGE_TAG=v1.0.0

podman push your-registry.io/yourorg/hyperfleet-api:v1.0.0
```

### Configuration in Kubernetes

The Helm chart manages configuration through:
- **ConfigMap** — generated from [`charts/values.yaml`](../charts/values.yaml) for non-sensitive settings
- **Secrets** — database credentials injected via `secretKeyRef`

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

**Example: Setting required adapters:**
```bash
--set 'config.adapters.required.cluster={validation,dns,pullsecret,hypershift}' \
--set 'config.adapters.required.nodepool={validation,hypershift}'
```

See [Configuration Guide](config.md) for the complete reference, and [`charts/values.yaml`](../charts/values.yaml) for all Helm-specific settings.

### Schema Validation via Helm

Partners can supply a custom OpenAPI schema for `spec` field validation:

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

When `validationSchema.enabled` is `true`, the chart creates a ConfigMap with the schema content, mounts it into the container, and sets `server.openapi_schema_path` in the generated config file to point to it.

Alternatively, reference an existing ConfigMap (must contain an `openapi.yaml` key):

```yaml
validationSchema:
  enabled: true
  existingConfigMap: my-validation-schema
```

### Deploying

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

#### Development Deployment (Using custom images)

Deploy with built-in PostgreSQL for development and testing (e.g., for engineer validation on a cluster):

```bash
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --create-namespace \
  --set image.registry=quay.io \
  --set image.repository=myuser/hyperfleet-api \
  --set image.tag=v1.0.0 \
  --set 'config.adapters.required.cluster={validation,dns,pullsecret,hypershift}' \
  --set 'config.adapters.required.nodepool={validation,hypershift}'
```

This creates:
- HyperFleet API deployment
- PostgreSQL StatefulSet
- Services for both components
- ConfigMaps and Secrets


**Note**: The `registry` should contain only the registry domain (e.g., `quay.io`, `docker.io`). The `repository` includes the organization and image name (e.g., `myuser/hyperfleet-api`).

#### Upgrade

```bash
helm upgrade hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --set image.tag=v1.1.0
```

#### Uninstall

```bash
helm uninstall hyperfleet-api --namespace hyperfleet-system
```

#### Custom Values File

Create a `values.yaml` file for repeatable deployments:

```yaml
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

```bash
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --values values.yaml
```

### Helm Values Reference

| Parameter | Description | Default |
|-----------|-------------|---------|
| `image.registry` | Container registry | `CHANGE_ME` (must be set explicitly) |
| `image.repository` | Image repository | `openshift-hyperfleet/hyperfleet-api` |
| `image.tag` | Image tag | `latest` |
| `image.pullPolicy` | Image pull policy | `Always` |
| `config.adapters.required.cluster` | Cluster adapters required for Reconciled state | `[]` |
| `config.adapters.required.nodepool` | Nodepool adapters required for Reconciled state | `[]` |
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

### Operations

#### Check Deployment Status

```bash
helm status hyperfleet-api --namespace hyperfleet-system
helm list --namespace hyperfleet-system
kubectl get pods --namespace hyperfleet-system
kubectl get svc --namespace hyperfleet-system
```

#### View Logs

```bash
kubectl logs -f deployment/hyperfleet-api --namespace hyperfleet-system
kubectl logs -f -l app=hyperfleet-api --namespace hyperfleet-system

# PostgreSQL logs (if using built-in)
kubectl logs -f statefulset/hyperfleet-postgresql --namespace hyperfleet-system
```

#### Troubleshooting

```bash
kubectl describe pod <pod-name> --namespace hyperfleet-system
kubectl get events --namespace hyperfleet-system --sort-by='.lastTimestamp'
kubectl exec -it deployment/hyperfleet-api --namespace hyperfleet-system -- /bin/sh
kubectl get secrets --namespace hyperfleet-system
kubectl get configmaps --namespace hyperfleet-system
```

### Health Checks

The deployment includes:
- Liveness probe: `GET /healthz` (port 8080) - Returns 200 if the process is alive
- Readiness probe: `GET /readyz` (port 8080) - Returns 200 when ready to receive traffic, 503 during startup/shutdown
- Metrics: `GET /metrics` (port 9090) - Prometheus metrics endpoint

### Scaling

```bash
# Manual scaling
kubectl scale deployment hyperfleet-api --replicas=3 --namespace hyperfleet-system

# Via Helm
helm upgrade hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --set replicaCount=3
```

Enable autoscaling via Helm values (`autoscaling.enabled=true`).

### Monitoring

Prometheus metrics available at `http://<service>:9090/metrics`.

#### Prometheus Operator Integration

```bash
# Enable ServiceMonitor
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --set image.registry=quay.io \
  --set serviceMonitor.enabled=true

# With custom Prometheus selector labels
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --set image.registry=quay.io \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.labels.release=prometheus

# ServiceMonitor in a different namespace
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --set image.registry=quay.io \
  --set serviceMonitor.enabled=true \
  --set serviceMonitor.namespace=monitoring
```

### Production Checklist

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

### Complete Example: GKE Deployment

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

---

## Local Execution

Run HyperFleet API directly on your machine without Helm or Kubernetes. Typical use cases: local development, debugging, integration testing.

### Prerequisites

- Go 1.25+, Podman, Make
- A running PostgreSQL instance (local container or external)

### Configuration

The application loads configuration in this priority order: **CLI flags > environment variables > config file > defaults**.

**Config file:** Copy the example and adjust as needed:

```bash
cp configs/config.yaml.example configs/config.yaml
```

The loader searches for a config file in this order:
1. `--config` flag (explicit path)
2. `HYPERFLEET_CONFIG` environment variable
3. `/etc/hyperfleet/config.yaml` (production default)
4. `./configs/config.yaml` (development default)

If none are found, the command fails with `failed to load configuration`.

**Environment variables:** Override any config value with the `HYPERFLEET_*` prefix:

```bash
export HYPERFLEET_DATABASE_HOST=localhost
export HYPERFLEET_DATABASE_PORT=5432
export HYPERFLEET_DATABASE_NAME=hyperfleet
export HYPERFLEET_DATABASE_USER=hyperfleet
export HYPERFLEET_DATABASE_PASSWORD=hyperfleet-dev-password
export HYPERFLEET_LOGGING_LEVEL=debug
export HYPERFLEET_SERVER_PORT=8000
```

See [Configuration Guide](config.md) for the complete reference and all available settings.

### Database Setup

**Option A: Local PostgreSQL container (quickest)**

```bash
make db/setup     # Creates a PostgreSQL container via Podman
make db/login     # Connect to the database for inspection
```

**Option B: External PostgreSQL**

Point the config or environment variables to your PostgreSQL instance:

```bash
export HYPERFLEET_DATABASE_HOST=my-postgres-host.example.com
export HYPERFLEET_DATABASE_PORT=5432
export HYPERFLEET_DATABASE_NAME=hyperfleet
export HYPERFLEET_DATABASE_USER=hyperfleet
export HYPERFLEET_DATABASE_PASSWORD=my-password
export HYPERFLEET_DATABASE_SSL_MODE=require   # for remote databases
```

### Running

```bash
# 1. Generate code (required after clone)
make generate-all

# 2. Build
make build

# 3. Run migrations
./bin/hyperfleet-api migrate

# 4. Start the server (no JWT auth)
make run-no-auth

# Or start with auth enabled:
./bin/hyperfleet-api serve
```

### Schema Validation (Local)

The API validates cluster and nodepool `spec` fields against an OpenAPI schema. Configure the schema path:

```bash
# Via flag
./bin/hyperfleet-api serve --server-openapi-schema-path ./openapi/openapi.yaml

# Via environment variable
export HYPERFLEET_SERVER_OPENAPI_SCHEMA_PATH=./openapi/openapi.yaml
```

The API **will fail to start** if the configured schema file is missing, unreadable, or invalid.

### Endpoints

Once running, the API is available at:

- **REST API**: `http://localhost:8000/api/hyperfleet/v1/`
- **OpenAPI spec**: `http://localhost:8000/api/hyperfleet/v1/openapi`
- **Swagger UI**: `http://localhost:8000/api/hyperfleet/v1/openapi.html`
- **Liveness probe**: `http://localhost:8080/healthz`
- **Readiness probe**: `http://localhost:8080/readyz`
- **Metrics**: `http://localhost:9090/metrics`

### CLI Subcommands

```bash
./bin/hyperfleet-api serve     # Start the HTTP server
./bin/hyperfleet-api migrate   # Run database migrations
./bin/hyperfleet-api version   # Print version, commit, and build date
```

---


## Related Documentation

- [Configuration Guide](config.md) - Complete configuration reference
- [Authentication](authentication.md) - Authentication configuration
- [Development Guide](development.md) - Local development setup and workflows