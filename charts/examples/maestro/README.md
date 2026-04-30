# Adapter example to create resources in a regional cluster

This `values.yaml` deploys an `adapter-task-config.yaml` that creates:

- A new namespace with the name of the cluster ID from the CloudEvent
- A service account, role and role bindings in that new namespace
- A Kubernetes Job with a status-reporter sidecar in that new namespace
- A nginx deployment in the same namespace as the adapter itself

## Overview

This example showcases:

- **Inline manifests**: Defines the Kubernetes Namespace resource directly in the adapter task config
- **External file references**: References external YAML files for Job, ServiceAccount, Role, RoleBinding, and Deployment
- **Preconditions**: Fetches cluster status from the Hyperfleet API before proceeding
- **Resource discovery**: Finds existing resources using label selectors
- **Status reporting**: Builds a status payload with CEL expressions and reports back to the Hyperfleet API
- **Job with sidecar**: Demonstrates a Job pattern with a status-reporter sidecar that monitors job completion and updates job conditions
- **Simulation modes**: Supports different test scenarios via `SIMULATE_RESULT` environment variable
- **RBAC configuration**: Demonstrates configuring additional RBAC resources in helm values

## Files

| File | Description |
|------|-------------|
| `values.yaml` | Helm values that configure the adapter, broker, image, environment variables, and RBAC permissions |
| `adapter-config.yaml` | Adapter deployment config (clients, broker, Kubernetes settings) |
| `adapter-task-config.yaml` | Task configuration with inline namespace manifest, external file references, params, preconditions, and post-processing |
| `adapter-task-resource-job.yaml` | Kubernetes Job template with a main container and status-reporter sidecar |
| `adapter-task-resource-job-serviceaccount.yaml` | ServiceAccount for the Job to use in the cluster namespace |
| `adapter-task-resource-job-role.yaml` | Role granting permissions for the status-reporter to update job status |
| `adapter-task-resource-job-rolebinding.yaml` | RoleBinding connecting the ServiceAccount to the Role |
| `adapter-task-resource-deployment.yaml` | Nginx deployment template created in the adapter's namespace |

## Key Features

### Inline vs External Manifests

This example uses both approaches:

**Inline manifest** for the Namespace:

```yaml
resources:
  - name: "clusterNamespace"
    manifest:
      apiVersion: v1
      kind: Namespace
      metadata:
        name: "{{ .clusterId }}"
```

**External file reference** for complex resources:

```yaml
resources:
  - name: "jobNamespace"
    manifest:
      ref: "/etc/adapter/job.yaml"
```

### Job with Status-Reporter Sidecar

The Job (`job.yaml`) includes two containers:

1. **Main container**: Runs the workload and writes results to a shared volume
2. **Status-reporter sidecar**: Monitors the main container, reads results, and updates the Job's status conditions

This pattern enables the adapter to track job completion through Kubernetes native conditions.

### Simulation Modes

The `SIMULATE_RESULT` environment variable controls test scenarios:

| Value | Behavior |
|-------|----------|
| `success` | Writes success result and exits cleanly |
| `failure` | Writes failure result and exits with error |
| `hang` | Sleeps indefinitely (tests timeout handling) |
| `crash` | Exits without writing results |
| `invalid-json` | Writes malformed JSON |
| `missing-status` | Writes JSON without required status field |

Configure in `values.yaml`:

```yaml
env:
  - name: SIMULATE_RESULT
    value: success
```

## Configuration

### RBAC Resources

The `values.yaml` configures RBAC permissions needed for resource management.
In this example is overly permissive since is creating deployments and jobs

```yaml
rbac:
  resources:
    - namespaces
    - serviceaccounts
    - configmaps
    - deployments
    - roles
    - rolebindings
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
helm install <name> ./charts -f charts/examples/values.yaml \
  --namespace <namespace> \
  --set image.registry=quay.io/<developer-registry> \
  --set broker.googlepubsub.projectId=<gcp-project> \
  --set broker.googlepubsub.subscriptionId=<gcp-subscription> \
  --set broker.googlepubsub.topic=<gcp-topic> \
  --set broker.googlepubsub.deadLetterTopic=<gcp-dlq-topic>
```

## How It Works

1. The adapter receives a CloudEvent with a cluster ID and generation
2. **Preconditions**: Fetches cluster status from the Hyperfleet API and captures the cluster name, generation, and ready condition
3. **Validation**: Checks that the cluster's Ready condition is "False" before proceeding
4. **Resource creation**: Creates resources in order:
   - Namespace named with the cluster ID
   - ServiceAccount in the new namespace
   - Role and RoleBinding for the status-reporter
   - Job with main container and status-reporter sidecar
   - Nginx deployment in the adapter's namespace
5. **Job execution**: The Job runs, writes results to a shared volume, and the status-reporter updates job conditions
6. **Post-processing**: Builds a status payload checking Applied, Available, and Health conditions
7. **Status reporting**: Reports the status back to the Hyperfleet API
