# Adapter Examples

This directory contains example configurations for deploying the HyperFleet Adapter using different transport clients.

## Examples

| Directory | Transport | Description |
|-----------|-----------|-------------|
| [`kubernetes/`](./kubernetes/) | Kubernetes only | Creates resources directly in the local cluster using the Kubernetes client |
| [`maestro/`](./maestro/) | Maestro only | Deploys resources to a remote cluster via Maestro using ManifestWork |

---

### `kubernetes/` — Direct Kubernetes Resource Management

Creates the following resources directly in the local cluster via the Kubernetes transport client:

- A Namespace named after the cluster ID from the CloudEvent
- A ServiceAccount, Role, and RoleBinding in that namespace
- A Kubernetes Job with a status-reporter sidecar
- An Nginx Deployment in the adapter's own namespace

**Key features demonstrated:**

- Inline manifests and external file references (`ref:`)
- Preconditions with Hyperfleet API calls and CEL expressions
- Resource discovery by name and label selectors
- Job with a status-reporter sidecar pattern
- Simulation modes via `SIMULATE_RESULT` environment variable
- Status reporting back to the Hyperfleet API

See [`kubernetes/README.md`](./kubernetes/README.md) for full details.

---

### `maestro/` — Maestro Transport (ManifestWork)

Deploys resources to a remote cluster through Maestro (Open Cluster Management) using a ManifestWork:

- A ManifestWork delivered to the target cluster via Maestro gRPC transport
- The ManifestWork contains a Namespace and a ConfigMap on the remote cluster
- Nested resource discovery within the ManifestWork result

**Key features demonstrated:**

- Maestro transport client configuration (gRPC + HTTP)
- ManifestWork template with external file reference (`ref:`)
- Resource discovery by name and by label selectors (`nested_discoveries`)
- Post-processing with CEL expressions on nested ManifestWork status
- Status reporting back to the Hyperfleet API

See [`maestro/README.md`](./maestro/README.md) for full details.

---

## Common Configuration

All examples share the same broker and image placeholders that must be updated before deployment.

### Broker

```yaml
broker:
  googlepubsub:
    projectId: CHANGE_ME
    subscriptionId: CHANGE_ME
    topic: CHANGE_ME
    deadLetterTopic: CHANGE_ME
```

### Image

```yaml
image:
  registry: CHANGE_ME
  repository: hyperfleet-adapter
  pullPolicy: Always
  tag: latest
```

## Usage

```bash
helm install <name> ./charts -f charts/examples/<example>/values.yaml \
  --namespace <namespace> \
  --set image.registry=quay.io/<developer-registry> \
  --set broker.googlepubsub.projectId=<gcp-project> \
  --set broker.googlepubsub.subscriptionId=<gcp-subscription> \
  --set broker.googlepubsub.topic=<gcp-topic> \
  --set broker.googlepubsub.deadLetterTopic=<gcp-dlq-topic>
```
