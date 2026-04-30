# Adapter example to create a hello-world Job in a regional cluster

This `values.yaml` deploys an `adapter-task-config.yaml` that creates:

- A new namespace named `<clusterId>-k8s`
- A hello-world Kubernetes Job in that namespace

## Overview

This example showcases:

- **Inline manifests**: Defines the Kubernetes Namespace resource directly in the adapter task config
- **External file references**: References an external YAML file for the Job
- **Preconditions**: Fetches cluster status from the Hyperfleet API before proceeding
- **Resource discovery**: Finds existing resources using label selectors
- **Status reporting**: Builds a status payload with CEL expressions and reports back to the Hyperfleet API
- **Lifecycle delete**: Cleans up resources (namespace and job) when the cluster is marked for deletion

## Files

| File | Description |
|------|-------------|
| `values.yaml` | Helm values that configure the adapter, broker, image, and RBAC permissions |
| `adapter-config.yaml` | Adapter deployment config (clients, broker, Kubernetes settings) |
| `adapter-task-config.yaml` | Task configuration with inline namespace manifest, external job file reference, params, preconditions, and post-processing |
| `adapter-task-resource-job.yaml` | Kubernetes Job that prints "Hello, World!" and exits |

## Key Features

### Inline vs External Manifests

This example uses both approaches:

**Inline manifest** for the Namespace:

```yaml
resources:
  - name: "namespace"
    manifest:
      apiVersion: v1
      kind: Namespace
      metadata:
        name: "{{ .clusterId }}-k8s"
```

**External file reference** for the Job:

```yaml
resources:
  - name: "helloWorldJob"
    manifest:
      ref: "/etc/adapter/job.yaml"
```

### Job Completion Tracking

The `Available` condition checks the native Kubernetes Job `Complete` condition to determine
whether the job has finished successfully:

| Job state | Available status | Reason |
|-----------|-----------------|--------|
| Not yet applied | `False` | `JobPending` |
| Running | `False` | `JobRunning` |
| Completed | `True` | `JobComplete` |
| Failed | `False` | `JobFailed` |

## Configuration

### RBAC Resources

The `values.yaml` configures RBAC permissions needed for resource management:

```yaml
rbac:
  resources:
    - namespaces
    - jobs
    - jobs/status
    - pods
```

### Broker Configuration

Update the `broker.googlepubsub` section in `values.yaml` with your GCP Pub/Sub settings:

```yaml
broker:
  googlepubsub:
    projectId: CHANGE_ME
    subscriptionId: CHANGE_ME
    topic: CHANGE_ME
    deadLetterTopic: CHANGE_ME
```

### Image Configuration

Update the image registry in `values.yaml`:

```yaml
image:
  registry: CHANGE_ME
  repository: hyperfleet-adapter
  pullPolicy: Always
  tag: latest
```

## Usage

```bash
helm install <name> ./charts -f charts/examples/kubernetes/values.yaml \
  --namespace <namespace> \
  --set image.registry=quay.io/<developer-registry> \
  --set broker.googlepubsub.projectId=<gcp-project> \
  --set broker.googlepubsub.subscriptionId=<gcp-subscription> \
  --set broker.googlepubsub.topic=<gcp-topic> \
  --set broker.googlepubsub.deadLetterTopic=<gcp-dlq-topic>
```

## How It Works

1. The adapter receives a CloudEvent with a cluster ID
2. **Preconditions**: Fetches cluster status from the Hyperfleet API and captures the cluster name, generation, and deletion state
3. **Resource creation**: Creates resources in order:
   - Namespace named `<clusterId>-k8s`
   - Hello-world Job in that namespace
4. **Job execution**: The Job prints "Hello, World!" and exits successfully
5. **Post-processing**: Builds a status payload checking Applied, Available, Health, and Finalized conditions
6. **Status reporting**: Reports the status back to the Hyperfleet API
