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

### Default Image

The default container image is:
```text
quay.io/openshift-hyperfleet/hyperfleet-api:latest
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

HyperFleet API is configured via environment variables.

### Schema Validation

**`OPENAPI_SCHEMA_PATH`** - Path to OpenAPI specification for spec validation

The API validates cluster and nodepool `spec` fields against an OpenAPI schema. This allows different providers (GCP, AWS, Azure) to have different spec structures.

- Default: Uses `openapi/openapi.yaml` from the repository
- Custom: Set via `OPENAPI_SCHEMA_PATH` environment variable for provider-specific schemas

```bash
export OPENAPI_SCHEMA_PATH=/path/to/custom-schema.yaml
```

### Environment Variables

**Database:**
- `DB_HOST` - PostgreSQL hostname (default: `localhost`)
- `DB_PORT` - PostgreSQL port (default: `5432`)
- `DB_NAME` - Database name (default: `hyperfleet`)
- `DB_USER` - Database username (default: `hyperfleet`)
- `DB_PASSWORD` - Database password (required)
- `DB_SSLMODE` - SSL mode: `disable`, `require`, `verify-ca`, `verify-full` (default: `disable`)

**Authentication:**
- `AUTH_ENABLED` - Enable JWT authentication (default: `true`)
- `OCM_URL` - OpenShift Cluster Manager API URL (default: `https://api.openshift.com`)
- `JWT_ISSUER` - JWT token issuer URL (default: `https://sso.redhat.com/auth/realms/redhat-external`)
- `JWT_AUDIENCE` - JWT token audience (default: `https://api.openshift.com`)

**Server:**
- `PORT` - API server port (default: `8000`)
- `HEALTH_PORT` - Health endpoints port (default: `8080`)
- `METRICS_PORT` - Metrics endpoint port (default: `9090`)

**Logging:**
- `LOG_LEVEL` - Logging level: `debug`, `info`, `warn`, `error` (default: `info`)
- `LOG_FORMAT` - Log format: `json`, `text` (default: `json`)

**Adapter Requirements:**

Configure which adapters must be ready for resources to be marked as "Ready".

**Default values** (if not configured):
- Cluster: `["validation","dns","pullsecret","hypershift"]`
- NodePool: `["validation","hypershift"]`

**Option 1: Using structured values (Helm only, recommended)**
```yaml
# values.yaml
adapters:
  cluster:
    - validation
    - dns
    - pullsecret
    - hypershift
  nodepool:
    - validation
    - hypershift
```

**Option 2: Using environment variables in Helm**
```yaml
# values.yaml
env:
  - name: HYPERFLEET_CLUSTER_ADAPTERS
    value: '["validation","dns","pullsecret","hypershift"]'
  - name: HYPERFLEET_NODEPOOL_ADAPTERS
    value: '["validation","hypershift"]'
```

**Option 3: Direct environment variable (non-Helm)**
```bash
export HYPERFLEET_CLUSTER_ADAPTERS='["validation","dns","pullsecret","hypershift"]'
export HYPERFLEET_NODEPOOL_ADAPTERS='["validation","hypershift"]'
```

## Kubernetes Deployment

### Using Helm Chart

The project includes a Helm chart for Kubernetes deployment with configurable PostgreSQL support.

#### Development Deployment

Deploy with built-in PostgreSQL for development and testing:

```bash
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --create-namespace
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
  --set database.postgresql.enabled=false \
  --set database.external.enabled=true \
  --set database.external.secretName=hyperfleet-db-external
```

#### Custom Image Deployment

Deploy with custom container image:

```bash
helm install hyperfleet-api ./charts/ \
  --namespace hyperfleet-system \
  --set image.registry=quay.io \
  --set image.repository=myuser/hyperfleet-api \
  --set image.tag=v1.0.0
```

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
| `image.registry` | Container registry | `quay.io` |
| `image.repository` | Image repository | `openshift-hyperfleet/hyperfleet-api` |
| `image.tag` | Image tag | `latest` |
| `image.pullPolicy` | Image pull policy | `IfNotPresent` |
| `auth.enableJwt` | Enable JWT authentication | `true` |
| `database.postgresql.enabled` | Enable built-in PostgreSQL | `true` |
| `database.external.enabled` | Use external database | `false` |
| `database.external.secretName` | Secret containing database credentials | `hyperfleet-db-external` |
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

auth:
  enableJwt: true

database:
  postgresql:
    enabled: false
  external:
    enabled: true
    secretName: hyperfleet-db-external

# Optional: customize adapter requirements (YAML table format)
adapters:
  cluster:
    - validation
    - dns
  nodepool:
    - validation

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

For Prometheus Operator, enable ServiceMonitor via Helm values (`serviceMonitor.enabled=true`).

## Production Best Practices

- Use external managed database (Cloud SQL, RDS, Azure Database)
- Enable authentication with `auth.enableJwt=true`
- Set resource limits and use multiple replicas
- Use specific image tags instead of `latest`
- Enable monitoring and regular database backups
- Enable PodDisruptionBudget with `podDisruptionBudget.enabled=true` for high availability during node maintenance

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
  --set auth.enableJwt=false \
  --set database.postgresql.enabled=false \
  --set database.external.enabled=true

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
