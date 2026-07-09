# HyperFleet Adapter Authoring Guide

> **Audience:** Adapter authors writing task configurations for HyperFleet cluster lifecycle events.

---

## 1. Introduction

An **adapter** is an event-driven worker that reacts to cluster lifecycle events, creates Kubernetes resources, and reports status back to the HyperFleet API. You don't write Go code to build an adapter — you write **YAML configuration** that the adapter framework binary executes.

Your custom logic lives in the Kubernetes objects created by adapters, the status conditions of these Kubernetes objects will be reported back by the adapter to the HyperFleet API offering external visibility to the managed objects.

This document uses "clusters" as an example of the entities modified by the user through the HyperFleet API, but the concepts apply to other entities, like node pools.

The flow of events goes like this:

customer updates cluster -> event -> adapter task -> k8s object performs work -> adapter reports status

### What you produce

Every adapter requires configuring 3 main elements:

| Concern |  Purpose |
|-------|---------|
| Adapter Config | Deployment settings: API client config, broker subscription, timeouts, retries |
| Adapter Task Config | Business logic: what to extract, check, create, and report |
| Broker config |  Broker configuration: Configures broker system (pubsub, rabbitmq) |

The `AdapterConfig` is pretty straightforward, it defines the name of the adapter as well as client configs to interact with HyperFleet API, Kubernetes or Maestro.

Your main authoring effort goes into the `AdapterTaskConfig` which configures the tasks to execute for every object changed by the customer

### When you need a new adapter

Create a new adapter when you need to:

- Provision a new type of infrastructure per cluster (namespace, RBAC, DNS, certificates)
- Validate cluster prerequisites before provisioning
- Manage resources on a remote cluster via Maestro/ManifestWork

### Development workflow

```
Write task config → Dry-run locally → Inspect trace → Iterate → Deploy
```

The framework includes a **dry-run mode** that simulates the full execution pipeline without any infrastructure. You can validate your configuration before touching a real cluster.

---

## 2. Concepts

### Event-driven execution

When a cluster is created or updated, the **Sentinel** detects the change and publishes a lightweight CloudEvent to a message broker. Your adapter receives this event and executes a four-phase pipeline:

```mermaid
flowchart LR
    E[CloudEvent] --> P1[1. Extract Params]
    P1 --> P2[2. Check Preconditions]
    P2 -->|met| P3[3. Apply Resources]
    P2 -->|not met| P4
    P3 --> P4[4. Report Status]
```

Post-actions (phase 4) **always execute**, even when preconditions are not met or resources fail. This ensures the adapter always reports its state back to the API.

### Generation-based reconciliation

Every spec change increments a cluster's `generation` counter. Adapters report `observed_generation` with their status. When `generation > observed_generation`, the Sentinel publishes an event to trigger reconciliation.

```mermaid
sequenceDiagram
    participant User
    participant API
    participant Sentinel
    participant Adapter

    User->>API: PATCH /clusters/{id} (spec change)
    API->>API: generation++ (now N+1)
    Sentinel->>API: Poll: generation=N+1, observed=N
    Sentinel->>Adapter: CloudEvent {id, generation: N+1}
    Adapter->>API: GET /clusters/{id}
    Adapter->>Adapter: Create/update resources
    Adapter->>API: PUT status {observed_generation: N+1}
    API->>API: All adapters at N+1 → Reconciled=True
```

### Anemic events

Information carried by the events is the minimum to identify the changed object by the adapters.
For example for a NodePool, it will require both the Id of the NodePool and the Id of the Cluster it belongs.

The format of the event is CloudEvents.

```json
{
  "data": {
    "id": "abc123",
    "kind": "Cluster",
    "href": "/api/hyperfleet/v1/clusters/abc123",
    "generation": 5
  }
}
```

The adapter fetches the full resource from the API during preconditions phase.
This keeps the event schema stable and ensures the adapter always works with fresh data.

### Configuration languages

Three languages appear in adapter configs, each for a different purpose:

| Language | Syntax | Use for |
|----------|--------|---------|
| Go Templates | `{{ .clusterId }}` | String interpolation in URLs, manifest fields, direct values |
| CEL | `expression: "..."` | Logic evaluation in preconditions, status conditions, computed values |
| JSONPath | `field: "path"` | Simple field extraction from API responses |

**Rule of thumb:** Use Go Templates for inserting values into strings. Use CEL when you need conditionals, array filtering, or type-safe logic. Use `field:` for straightforward extraction from API responses.

---

## 3. Configuration Structure

### File skeleton

```yaml
params: []            # Phase 1: Extract variables from event and environment
preconditions: []     # Phase 2: Validate state via API calls
resources: []         # Phase 3: Create/update Kubernetes resources
post:                 # Phase 4: Report status
  payloads: []        #   Build status JSON
  post_actions: []    #   Send status to API
```

### Execution flow and error handling

```mermaid
flowchart TD
    START([CloudEvent received]) --> PARAMS[Phase 1: Extract Params]
    PARAMS -->|required param missing| FAIL_PARAMS[Set adapter.executionError]
    FAIL_PARAMS --> POST
    PARAMS -->|success| PRECOND[Phase 2: Preconditions]
    PRECOND -->|API call fails| FAIL_PRECOND[Set adapter.executionError]
    FAIL_PRECOND --> POST
    PRECOND -->|conditions not met| SKIP[Set adapter.resourcesSkipped=true]
    SKIP --> POST
    PRECOND -->|conditions met| RESOURCES[Phase 3: Apply Resources]
    RESOURCES -->|resource fails| FAIL_RES[Set adapter.executionError + resourceErrors]
    FAIL_RES --> POST
    RESOURCES -->|success| POST
    POST[Phase 4: Build Payload & Report Status]
    POST --> DONE([Done])
```

The `adapter.*` context is populated automatically and available in your post-action CEL expressions:

| Variable | Type | Description |
|----------|------|-------------|
| `adapter.executionStatus` | string | `"success"` or `"failed"` |
| `adapter.resourcesSkipped` | bool | `true` if preconditions were not met, or if any resource's `lifecycle.create.when` evaluated to `false` |
| `adapter.skipReason` | string | Why resources were skipped |
| `adapter.executionError.phase` | string | Phase where the first error occurred |
| `adapter.executionError.step` | string | Specific step that first failed |
| `adapter.executionError.message` | string | First error details |
| `adapter.resourceErrors` | map | Per-resource errors from the resources phase (keyed by resource name) |
| `adapter.resourceErrors.<name>.phase` | string | Phase for that resource's error |
| `adapter.resourceErrors.<name>.step` | string | Resource name that failed |
| `adapter.resourceErrors.<name>.message` | string | Error details for that resource |

---

## 4. Parameter Extraction

Parameters are variables extracted from the incoming CloudEvent and the runtime environment. They become available as Go Template variables (`{{ .paramName }}`) and CEL variables throughout the rest of the config.

```yaml
params:
  # From the CloudEvent data
  - name: "clusterId"
    source: "event.id"
    type: "string"
    required: true

  - name: "generation"
    source: "event.generation"
    type: "int"
    required: true

  # From environment variables (set in Helm values or deployment)
  - name: "region"
    source: "env.REGION"
    type: "string"
    default: "us-east-1"
```

### Sources

| Prefix | Source | Example |
|--------|--------|---------|
| `event.` | CloudEvent data fields | `event.id`, `event.generation`, `event.kind` |
| `env.` | Environment variables | `env.REGION`, `env.NAMESPACE` |
| `secret.` | Kubernetes Secret | `secret.my-ns.my-secret.api-key` |
| `configmap.` | Kubernetes ConfigMap | `configmap.my-ns.my-config.setting` |

### Types and conversion

| Type | Accepts |
|------|---------|
| `string` | Any value (default) |
| `int`, `int64` | Integers, numeric strings, floats (truncated) |
| `float`, `float64` | Numeric values |
| `bool` | `true/false`, `yes/no`, `on/off`, `1/0` |

If type conversion fails on a **required** param, execution stops. On an optional param, the `default` value is used.

### Common parameters

Most adapters need at least `clusterId` and `generation` from the event. These are the minimum to identify what cluster changed and at what generation.

---

## 5. Preconditions

A precondition is used to decide if the Resource phase executes or not.

Preconditions validate cluster state before the adapter creates resources. They run **sequentially** — each precondition can use data captured by previous ones.

Usually an API call to the HyperFleet API will be the first action of the Precondition phase, to extract values from the current state of the cluster.

The state of the cluster contains information about all adapters in the form of `conditions[]`, so it can be used to make one adapter depend on the results of another adapter.

### Making an API call

```yaml
preconditions:
  - name: "clusterStatus"
    api_call:
      method: "GET"
      url: "/api/hyperfleet/v1/clusters/{{ .clusterId }}"
      timeout: 10s
      retry_attempts: 3
      retry_backoff: "exponential"    # also: linear, constant
```

URLs are **relative** — the base URL comes from the `AdapterConfig` `clients.hyperfleet_api.base_url` setting. You only write the path.

### Capturing fields

After the API call, capture values from the response for use in later phases. Two extraction modes are available (`field` or `expression`)— use one per capture, not both:

```yaml
    capture:
      # Simple field extraction (dot notation or JSONPath)
      - name: "clusterName"
        field: "name"

      # CEL expression for computed values
      - name: "reconciledStatus"
        expression: |
          status.conditions.filter(c, c.type == "Reconciled").size() > 0
            ? status.conditions.filter(c, c.type == "Reconciled")[0].status
            : "False"

      # JSONPath with filter
      - name: "lzNamespaceStatus"
        field: "{.items[?(@.adapter=='landing-zone')].data.namespace.status}"
```

> **Scope:** Capture expressions can only see the current API response. They cannot reference params or other captured values.

### Evaluating conditions

After captures, evaluate conditions to decide whether to proceed. Two syntaxes are available:

**Structured conditions** — declarative, readable for simple checks:

```yaml
    conditions:
      - field: "reconciledStatus"
        operator: "equals"
        value: "False"
```

**CEL expression** — for complex logic:

```yaml
    expression: |
      reconciledStatus == "False" && clusterStatus.spec.nodeCount > 0
```

> **Scope:** Conditions see the **full execution context**: all params, all captured fields, and the full API response accessible via the precondition name (e.g., `clusterStatus.status.conditions`).

### Supported operators

| Operator | Description |
|----------|-------------|
| `equals` | Exact match |
| `notEquals` | Not equal |
| `in` | Value is in array |
| `notIn` | Value is not in array |
| `contains` | String contains substring |
| `greaterThan` | Numeric greater than |
| `lessThan` | Numeric less than |
| `exists` | Field exists (no value needed) |
| `notExists` | Field does not exist (no value needed) |
| `greaterThanOrEqual` | Numeric greater than or equal |
| `lessThanOrEqual` | Numeric less than or equal |

### Chaining preconditions

Preconditions execute in order. Data flows forward — a captured field from precondition 1 is available in precondition 2's conditions:

```yaml
preconditions:
  - name: "getCluster"
    api_call:
      url: "/api/hyperfleet/v1/clusters/{{ .clusterId }}"
    capture:
      - name: "clusterName"
        field: "name"

  - name: "getStatuses"
    api_call:
      url: "/api/hyperfleet/v1/clusters/{{ .clusterId }}/statuses"
    capture:
      - name: "lzReconciled"
        field: "{.items[?(@.adapter=='landing-zone')].data.namespace.status}"
    conditions:
      - field: "lzReconciled"
        operator: "equals"
        value: "Active"
```

When a condition is **not met**, the adapter skips the resources phase but still runs post-actions. The `adapter.resourcesSkipped` flag is set to `true` and `adapter.skipReason` describes why.

### Time-based stability preconditions

#### Why use time-based preconditions?

Adapter preconditions typically need to handle two scenarios:

1. **Initial deployment** — Deploy resources when the cluster is NOT Reconciled
2. **Self-healing** — Detect and recreate accidentally deleted resources when the cluster IS Reconciled

A condition-only precondition (e.g., "only run when cluster is NOT Reconciled") handles scenario 1 but breaks scenario 2:

```yaml
# Condition-only pattern - INCOMPLETE
preconditions:
  - name: "getCluster"
    api_call:
      url: "/api/hyperfleet/v1/clusters/{{ .clusterId }}"
    capture:
      - name: "reconciledStatus"
        expression: |
          status.conditions.filter(c, c.type == "Reconciled").size() > 0
            ? status.conditions.filter(c, c.type == "Reconciled")[0].status
            : "False"
    conditions:
      - field: "reconciledStatus"
        operator: "equals"
        value: "False"   # Only runs resource phase when NOT Reconciled
```

**Problem:** If a resource is accidentally deleted while the cluster is Reconciled, the adapter skips the resource operation phase because the precondition is `False`. The adapter still runs and reports status, but it cannot detect or recreate the deleted resource because it never executes the resource phase.

**Solution:** Add a time-based stability check to enable both scenarios:

- Run resource phase when cluster is **NOT Reconciled**
- Run resource phase when cluster is **Reconciled AND stable for >5 minutes** (periodic self-healing)

#### Understanding `last_transition_time` vs `last_updated_time`

To implement time-based stability checks, you need to know how long a cluster has been in its current state. Each condition provides two timestamp fields:

| Field | Updates when | Use for |
|-------|-------------|---------|
| **`last_transition_time`** | Condition status **changes** (True→False or False→True) | **Stability windows** — "cluster has been Reconciled for N minutes" |
| **`last_updated_time`** | Adapter **reports status** (every PUT, even if unchanged) | **Liveness checks** — "adapter reported recently" |

**Critical:** For stability windows, always use `last_transition_time`. The `last_updated_time` field has special aggregation behavior that makes it unsuitable for measuring state duration.

#### Correct pattern: Time-based stability precondition

```yaml
preconditions:
  - name: "checkClusterState"
    api_call:
      url: "/api/hyperfleet/v1/clusters/{{ .clusterId }}"
    capture:
      - name: "clusterNotReconciled"
        expression: |
          status.conditions.filter(c, c.type == "Reconciled").size() > 0
            ? status.conditions.filter(c, c.type == "Reconciled")[0].status != "True"
            : true
      - name: "clusterReconciledTTL"
        expression: |
          (timestamp(now()) - timestamp(
            status.conditions.filter(c, c.type == "Reconciled").size() > 0
              ? status.conditions.filter(c, c.type == "Reconciled")[0].last_transition_time
              : now()
          )).getSeconds() > 300

  - name: "validationCheck"
    # Precondition passes if cluster is NOT Reconciled OR if cluster is Reconciled and stable for >300 seconds since last transition (enables self-healing)
    expression: |
      clusterNotReconciled || clusterReconciledTTL
```

**What this does:**

- `clusterNotReconciled` → Captures whether the cluster is NOT Reconciled (true when Reconciled condition is missing or not "True")
- `clusterReconciledTTL` → Captures whether the cluster has been Reconciled for >5 minutes (300 seconds) since the last status transition
- `validationCheck` → Evaluates both conditions: run resource phase when cluster is NOT Reconciled OR when cluster has been Reconciled and stable for >5 minutes (self-healing)

**Important notes:**

- The `now()` function returns the current time in RFC3339 format as a string.
- Use `timestamp()` to convert RFC3339 strings to timestamp types for arithmetic operations.
- This pattern enables both initial deployment and periodic self-healing checks.

---

## 6. Resources

Resources define the Kubernetes objects your adapter creates on the management cluster. They execute **sequentially** in the order listed — a namespace defined first is available for resources defined after it.

**Important**: Include an annotation `hyperfleet.io/generation: {{ .generation }}` to the kubernetes resources to create. This will be used by adapters to know if the object is in current generation or must be updated.

### Inline manifests

```yaml
resources:
  - name: "clusterNamespace"
    transport:
      client: "kubernetes"
    manifest:
      apiVersion: v1
      kind: Namespace
      metadata:
        name: "{{ .clusterId }}"
        labels:
          hyperfleet.io/cluster-id: "{{ .clusterId }}"
          hyperfleet.io/managed-by: "{{ .adapter.name }}"
          hyperfleet.io/resource-type: "namespace"
        annotations:
          hyperfleet.io/generation: "{{ .generation }}"
    discovery:
      by_name: "{{ .clusterId }}"
```

Inline manifests are parsed as YAML before template rendering, so they support `{{ .var }}` substitution in values but **not** structural directives (`{{ if }}`, `{{ range }}`). To use structural Go templates inline, use a YAML block scalar (`|`):

```yaml
resources:
  - name: "clusterConfig"
    transport:
      client: "kubernetes"
    manifest: |
      apiVersion: v1
      kind: ConfigMap
      metadata:
        name: "{{ .clusterId }}-config"
      data:
        cluster_id: "{{ .clusterId }}"
      {{ if eq .platformType "gcp" }}
        platform_tier: "cloud"
      {{ else }}
        platform_tier: "onprem"
      {{ end }}
    discovery:
      by_name: "{{ .clusterId }}-config"
```

The `|` block scalar tells YAML to treat the content as a raw string, which preserves Go template directives for rendering at execution time.

### External manifest files

For larger manifests, reference an external YAML file:

```yaml
resources:
  - name: "validationJob"
    transport:
      client: "kubernetes"
    manifest:
      ref: "/etc/adapter/job.yaml"
    discovery:
      namespace: "{{ .clusterId }}"
      by_selectors:
        label_selector:
          hyperfleet.io/cluster-id: "{{ .clusterId }}"
          hyperfleet.io/resource-type: "job"
```

Note that the location of the referenced file is a path of the adapter pod, so it has to be mounted from a ConfigMap in the adapter deployment.

The referenced file is a Go template and has access to all params and captured fields.

### Resource lifecycle

The framework determines the operation automatically:

| Operation | When | Behavior |
|-----------|------|----------|
| `create` | Resource doesn't exist | Apply the manifest |
| `update` | Resource exists, generation changed | Patch the resource |
| `skip` | Resource exists and generation unchanged, **or** resource doesn't exist and `lifecycle.create.when` evaluates to `false` | No-op; the latter case also sets `adapter.resourcesSkipped` to `true` |
| `recreate` | `recreate_on_change: true` is set | Delete then create |
| `delete` | `lifecycle.delete.when` expression evaluates to `true` | Delete the resource; remaining resources still processed |

### Discovery

After applying a resource, the framework **discovers** it to read its server-populated state (status, uid, resourceVersion). This state is then available in post-action CEL expressions via `resources.<name>`.

Two discovery modes:

```yaml
# By name (direct lookup)
discovery:
  by_name: "{{ .clusterId }}"

# By label selector
discovery:
  namespace: "{{ .clusterId }}"       # omit or "*" for cluster-scoped
  by_selectors:
    label_selector:
      hyperfleet.io/cluster-id: "{{ .clusterId }}"
      hyperfleet.io/resource-type: "namespace"
```

### Labeling conventions

Always label your resources for discovery and traceability:

| Label | Purpose |
|-------|---------|
| `hyperfleet.io/cluster-id` | Associate resource with a cluster |
| `hyperfleet.io/managed-by` | Adapter that owns this resource |
| `hyperfleet.io/resource-type` | Resource category for discovery |
| `hyperfleet.io/generation` | Generation that created/updated this resource (annotation) |

### Transport types

Different transport types are available for resources:

- Kubernetes: makes use of an active Kubernetes configuration to create k8s objects using the Kubernetes API
  - The credentials can be specified using a custom KubeConfigPath in the `AdapterConfig`
  - Or using in-cluster configuration to deploy to the same cluster the Adapter is running
- Maestro: connects to a maestro server to send manifestworks which can contain many resources as manifests

#### Kubernetes (direct)

The default. Resources are applied directly to the management cluster's API server.

```yaml
resources:
  - name: "myResource"
    transport:
      client: "kubernetes"
    manifest:
      # ... standard K8s manifest
```

#### Maestro (remote clusters via ManifestWork)

For resources that need to land on a remote spoke cluster managed through Open Cluster Management / Maestro. The manifest is a `ManifestWork` that wraps the actual resources.

<details>

<summary>Maestro adapter-task-config example</summary>

```yaml
resources:
  - name: "clusterSetup"
    transport:
      client: "maestro"
      maestro:
        target_cluster: "{{ .placementClusterName }}"
    manifest:
      apiVersion: work.open-cluster-management.io/v1
      kind: ManifestWork
      metadata:
        name: "manifestwork-{{ .clusterId }}"
        labels:
          hyperfleet.io/cluster-id: "{{ .clusterId }}"
      spec:
        workload:
          manifests:
            - apiVersion: v1
              kind: Namespace
              metadata:
                name: "{{ .clusterId }}"
                labels:
                  hyperfleet.io/cluster-id: "{{ .clusterId }}"
                  hyperfleet.io/resource-type: "namespace"
            - apiVersion: v1
              kind: ConfigMap
              metadata:
                name: "{{ .clusterId }}-config"
                namespace: "{{ .clusterId }}"
              data:
                cluster_id: "{{ .clusterId }}"
        manifestConfigs:
          - resourceIdentifier:
              group: ""
              resource: "namespaces"
              name: "{{ .clusterId }}"
            updateStrategy:
              type: "ServerSideApply"
            feedbackRules:
              - type: "JSONPaths"
                jsonPaths:
                  - name: "phase"
                    path: ".status.phase"
    discovery:
      by_selectors:
        label_selector:
          hyperfleet.io/cluster-id: "{{ .clusterId }}"
```

</details>

#### Nested discovery (Maestro)

A ManifestWork bundles multiple sub-resources. To inspect those sub-resources individually in your post-action CEL expressions without traversing the whole resources tree, you can use `nested_discoveries`:

```yaml
    nested_discoveries:
      - name: "namespace0"
        discovery:
          by_selectors:
            label_selector:
              hyperfleet.io/resource-type: "namespace"
      - name: "configmap0"
        discovery:
          by_name: "{{ .clusterId }}-config"
```

Nested discoveries are **promoted to top-level keys** in the `resources` map. Access them as `resources.namespace0`, not `resources.clusterSetup.namespace0`. This keeps CEL expressions clean.

Beside this shortcut, the nested Discovery also allows accessing status data from the resource such as statusFeedback and conditions.

**statusFeedback** — Maestro populates `statusFeedback.values` when `feedbackRules` are configured on the ManifestWork. Use it to read individual field values from the sub-resource without traversing the full ManifestWork tree:

```yaml
# Available when the namespace phase (reported via feedbackRules) is Active
status:
  expression: |
    resources.?namespace0.?statusFeedback.?values.orValue([])
      .filter(v, v.name == "phase").size() > 0
    ? resources.namespace0.statusFeedback.values
        .filter(v, v.name == "phase")[0].fieldValue.string == "Active"
      ? "True" : "False"
    : "Unknown"
```

**conditions** — If the sub-resource reports a standard Kubernetes `conditions` array (e.g., a CRD managed by an operator on the spoke cluster), access it the same way you would on a directly discovered resource:

```yaml
# Available when the nested resource reports Ready=True
status:
  expression: |
    resources.?namespace0.?status.?conditions.orValue([])
      .exists(c, c.type == "Ready" && c.status == "True")
    ? "True" : "False"
```

### Conditional creation (lifecycle.create)

Resources can gate their **initial creation** on a CEL expression using the `lifecycle.create` block. This lets you apply a resource only once some runtime condition holds (a feature flag param, a sibling resource's discovered state, an event payload field) without blocking the rest of the resources phase — unlike preconditions, which are all-or-nothing for the entire phase.

#### Configuring lifecycle.create

```yaml
resources:
  - name: "optionalFeatureConfig"
    transport:
      client: "kubernetes"
    manifest:
      # ...
    discovery:                              # required: needed to check whether the resource already exists
      by_name: "{{ .clusterId }}-feature"
    lifecycle:
      create: # optional: Choose if you want to control creation with CEL
        when:
          expression: "params.?enableOptionalFeature.orValue(false)"  
```

**Requirements:**

- `discovery` must be configured on the same resource — without it the executor cannot tell whether the resource already exists, and validation rejects the config.
- `when.expression` is required when `when` is configured, and must be a valid CEL expression — config validation rejects anything else.

#### Behavior

- **Resource doesn't exist yet**: the `when` expression is evaluated. `false` skips creation (operation `skip`, `adapter.resourcesSkipped` set to `true`); `true` or absent `lifecycle.create` proceeds with the normal create flow.
- **Resource already exists**: the `when` expression is **ignored** — the resource is applied normally (update flow). This makes `lifecycle.create.when` a one-time gate on initial creation, not a recurring condition; once created, the resource is reconciled like any other on subsequent events.
- **CEL evaluation error**: the resource execution fails with a descriptive error (referencing the failing expression) and the resource is neither applied nor silently skipped — the same fail-closed behavior as `lifecycle.delete.when`.

#### Skipping without blocking siblings

Because the skip is scoped to a single resource, other resources in the list still execute. This lets you write dependency conditions the same way as with deletion:

```yaml
resources:
  - name: "resourceA"
    # ... no lifecycle.create — always applied

  - name: "resourceB"
    # ...
    discovery:
      by_name: "{{ .clusterId }}-b"
    lifecycle:
      create:
        when:
          # Only create resourceB once resourceA has been discovered
          expression: "resources.?resourceA.hasValue()"
```

Resources are pre-discovered before any `lifecycle.create.when` or `lifecycle.delete.when` expression is evaluated, so `resources.?resourceA.hasValue()` reflects state from prior reconciliations regardless of list order.

#### Reporting skipped resources in post-actions

Because `adapter.resourcesSkipped` is shared with the precondition-level skip flag, a post-action `when` gate can react the same way regardless of which phase produced the skip:

```cel
adapter.?resourcesSkipped.orValue(false)
  ? "Resources skipped: " + adapter.?skipReason.orValue("unknown")
  : "All resources processed successfully"
```

### Conditional deletion (lifecycle.delete)

Resources can be conditionally deleted using the `lifecycle.delete` block. This enables the adapter to clean up managed resources when a deletion event occurs, with CEL expressions controlling deletion order between dependent resources.

#### Configuration

```yaml
resources:
  - name: "clusterNamespace"
    transport:
      client: "kubernetes"
    manifest:
      # ...
    discovery:                    # required: needed to locate the resource for deletion
      by_name: "{{ .clusterId }}"
    lifecycle:
      delete:
        propagationPolicy: Background   # optional: Background (default), Foreground, Orphan
        when:
          expression: "is_deleting"     # required: CEL expression evaluated each reconciliation
```

**Requirements:**

- `discovery` must be configured on the same resource — without it the executor cannot locate the resource to delete.
- `when.expression` is required — the resource is deleted only when the expression evaluates to `true`.

#### The is_deleting pattern

The standard way to detect pending deletion is to derive a boolean from the precondition response using the named-map-variable approach. The precondition name (e.g. `getCluster`) is automatically injected as a map variable in the capture CEL context, enabling safe optional-field access:

```yaml
preconditions:
  - name: "getCluster"
    api_call:
      url: "/api/hyperfleet/v1/clusters/{{ .clusterId }}"
    capture:
      - name: "is_deleting"
        expression: "has(getCluster.deleted_time)"
```

Then reference `is_deleting` in `lifecycle.delete.when.expression`.

> **Why not `field: deleted_time`?** Capturing `deleted_time` as a `field:` capture without a default logs a `WARN` on every reconciliation where the field is absent — which is ~99% of the time when the cluster is not being deleted. The `has()` expression approach reads the field directly from the named response map and returns `false` cleanly when absent, with no log noise.

#### Dependency ordering

When multiple resources must be deleted in a specific order, use `resources.?X.hasValue()` to check whether a dependency has been confirmed gone:

```yaml
resources:
  - name: "configMapResource"
    # ...
    lifecycle:
      delete:
        when:
          expression: "is_deleting"

  - name: "namespaceResource"
    # ...
    lifecycle:
      delete:
        when:
          # Delete namespace only once configMapResource is confirmed gone
          expression: "is_deleting && !resources.?configMapResource.hasValue()"
```

How this works across reconciliation cycles:

```text
Reconciliation 1 (is_deleting=true):
  → Delete configMapResource                  ✓ (condition met)
  → Skip namespaceResource deletion           ✗ (configMapResource still present)

Reconciliation 2 (configMapResource gone):
  → configMapResource absent in context       -
  → Delete namespaceResource                  ✓ (condition met)
```

> **Note**: Use `!resources.?X.hasValue()` to check resource absence. Do not use `has()` (returns `true` even for nil-valued keys) or `== null` (fails if the key was never added due to a mid-loop executor failure).

#### Post-delete context

After deleting a resource, the executor rediscovers it to determine its actual state and updates `resources.X` accordingly:

| Post-delete state | `resources.X` | Effect on dependents |
|---|---|---|
| Resource confirmed gone (NotFound) | absent from context | Dependents can cascade in the same reconciliation |
| Resource still present (finalizers or Maestro async) | existing object | Dependents wait for the next reconciliation |

This means same-reconciliation cascading works for Kubernetes resources without finalizers. Resources with finalizers or Maestro ManifestWorks defer to the next reconciliation.

#### propagationPolicy

Controls how Kubernetes removes dependent objects. Ignored for Maestro transport.

| Value | Behavior |
|---|---|
| `Background` (default) | Kubernetes GC runs asynchronously after the resource is deleted |
| `Foreground` | API call blocks until all dependents are gone before removing the owner |
| `Orphan` | Owner is deleted immediately; dependents are left behind (no GC) |

---

## 7. Error Handling

### Apply vs delete failures

The resource executor treats apply and delete operations differently when they fail:

| Operation | Failure behavior |
|---|---|
| **Apply** (create/update) | Fail fast — stop processing remaining resources, set `adapter.executionError` |
| **Delete** | Continue — all delete operations are attempted even if one fails; all errors are reported |

This means a list containing both apply and delete operations behaves predictably: a delete failure does not prevent the next resource from being deleted, but an apply failure stops further processing.

### Resource not found (404 handling)

When a precondition API call returns `404 Not Found`, it can mean the resource no longer exists (e.g., deleted externally, incorrect ID in the event, direct DB removal) or that the precondition URL itself is misconfigured. The adapter distinguishes between two types of 404:

- **Resource not found** (default): any 404 is treated as a legitimate "resource does not exist" unless proven otherwise. This includes responses with specific error codes (`HYPERFLEET-NTF-001`, `HYPERFLEET-NTF-002`, `HYPERFLEET-NTF-003`), as well as 404s where the response body was stripped by a proxy or gateway. The adapter handles this gracefully — resources are skipped and post-actions still execute.
- **Broken endpoint** (error code `HYPERFLEET-NTF-000`): the catch-all 404 handler confirms no route matched the URL. The adapter treats this as a configuration error and reports failure status.

When the adapter detects a resource-not-found 404:

- `adapter.resourcesSkipped` is set to `true`
- `adapter.skipReason` is set to `"ResourceNotFound"`
- The resources phase is skipped entirely
- Post-actions still execute, so the adapter can report the skip back to the API

This means your post-action CEL expressions can detect the missing resource and report an appropriate status:

```cel
adapter.?skipReason.orValue("") == "ResourceNotFound"
  ? "Resource does not exist"
  : adapter.?skipReason.orValue("unknown reason")
```

The same 404 handling applies during post-action execution: a post-action 404 is treated as resource-not-found and remaining post-actions are skipped gracefully, unless the response contains error code `HYPERFLEET-NTF-000` indicating a misconfigured URL.

### Partial delete failures

When one or more delete operations fail:

- The executor continues and attempts all remaining resources in the list
- All delete errors are collected and joined
- `adapter.executionError` is set to the **first** failure encountered (centralized signal)
- `adapter.resourceErrors` is populated with **one entry per failed resource** (granular detail)
- `adapter.executionStatus` is set to `"failed"`

If an apply failure occurs after some delete failures, all errors (delete + apply) are joined and surfaced together.

Use `adapter.executionError` in Health/Finalized conditions to detect any failure. Use `adapter.?resourceErrors.?<name>` when you need to know which specific resource failed — for example, to include the failing resource name in a status message.

### The Finalized condition

Adapters that handle deletion must report a `Finalized` condition that signals to the HyperFleet API when cleanup is complete. The condition must guard against three failure modes:

1. **Not yet deleting** — `is_deleting` prevents reporting `Finalized=True` before deletion is requested
2. **Executor failed mid-loop** — `adapter.executionStatus == "success"` prevents `Finalized=True` when some resources were never processed (their context keys are absent, making `hasValue()` return `false` incorrectly)
3. **Resources still present** — `!resources.?X.hasValue()` confirms the resource is actually gone

```yaml
- type: "Finalized"
  status:
    expression: |
      is_deleting
        && adapter.?executionStatus.orValue("") == "success"
        && !resources.?clusterNamespace.hasValue()
      ? "True" : "False"
  reason:
    expression: |
      !is_deleting
      ? "NotDeleting"
      : adapter.?executionStatus.orValue("") != "success"
        ? "AdapterUnhealthy"
        : !resources.?clusterNamespace.hasValue()
          ? "CleanupConfirmed"
          : "CleanupInProgress"
  message:
    expression: |
      !is_deleting
      ? "No pending deletion for this adapter instance"
      : adapter.?executionStatus.orValue("") != "success"
        ? "Cannot confirm cleanup while adapter is unhealthy"
        : !resources.?clusterNamespace.hasValue()
          ? "All managed resources deleted and verified"
          : "Resource cleanup in progress"
```

When an adapter does not handle deletion, use a static `Finalized=False`:

```yaml
- type: "Finalized"
  status: "False"
  reason: "NotDeleting"
  message: "No pending deletion for this adapter instance"
```

### Accessing error details in post-actions

Two complementary error signals are available in post-action CEL expressions:

**`adapter.executionError`** — centralized signal for the first failure across all phases. Use this in Health/Finalized conditions as a general "did something fail?" check:

| Variable | Description |
|---|---|
| `adapter.executionError.phase` | Phase where the first error occurred (`resources`, `preconditions`, etc.) |
| `adapter.executionError.step` | Resource or step name that first failed |
| `adapter.executionError.message` | Human-readable description of the first error |

```cel
adapter.?executionError.?phase.orValue("unknown")
adapter.?executionError.?step.orValue("unknown")
adapter.?executionError.?message.orValue("no details")
```

**`adapter.resourceErrors`** — per-resource error details from the resources phase. Only populated when a resource-phase operation fails. Use this when you need to surface which specific resource failed or include granular details in a status message:

```cel
# Check if a specific resource failed
adapter.?resourceErrors.?clusterNamespace.hasValue()

# Include the failing resource's error in a status message
adapter.?resourceErrors.?clusterNamespace.?message.orValue("")
```

The standard Health condition (Section 9 boilerplate) already incorporates these fields.

---

## 8. The Status Contract: Kubernetes Objects and the Adapter

The adapter creates Kubernetes objects that do the real work — Jobs that run validation scripts, Deployments that provision infrastructure, ConfigMaps that hold configuration. The adapter then reads the **status** of these objects and translates it into a report for the HyperFleet API.

This means there is a contract between your Kubernetes objects and your adapter configuration: the adapter needs to know where to look in the object's status to determine whether the work succeeded.

### How the feedback loop works

```mermaid
sequenceDiagram
    participant Adapter
    participant K8s as Kubernetes API
    participant Workload as K8s Object (Job, Deployment, etc.)
    participant API as HyperFleet API

    Adapter->>K8s: Apply manifest (Job, Namespace, etc.)
    K8s->>Workload: Schedule and run
    Workload->>K8s: Update status (conditions, phase)
    Adapter->>K8s: Discover resource (read status back)
    Note over Adapter: Evaluate CEL expressions against<br/>discovered resource status
    Adapter->>API: PUT /statuses {Applied, Available, Health}
```

The adapter does **not** wait for the workload to complete. It reads whatever status is available at discovery time and reports it. If the object is still pending, the adapter reports `Available=False`. The Sentinel will trigger another reconciliation cycle later, and the adapter will read the updated status then.

More information about the adapter contract can be found in [Architecture repository - HyperFleet Adapter Status Contract](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/components/adapter/framework/adapter-status-contract.md)

### What your Kubernetes objects must expose

The adapter reads status from the standard Kubernetes status subresource. The three conditions it reports to the HyperFleet API map to questions about your workload:

| Adapter Condition | What it maps to on the K8s object | Example |
|-------------------|-----------------------------------|---------|
| **Applied** | Does the resource exist? Was it accepted by the API server? | `has(resources.myJob)` — the manifest was applied successfully |
| **Available** | Is the workload operational? Has it completed or reached a ready state? | Job: `status.conditions` contains `type=Complete, status=True`. Namespace: `status.phase == "Active"`. Deployment: `status.availableReplicas > 0` |
| **Health** | Did the adapter framework itself execute without errors? | This comes from `adapter.*` metadata, not from the K8s object |

The possible values for these conditions statuses are: `True`, `False` and `Unknown`.

The `Unknown` value is used when there the condition value is still pending and there is no valid answer yet. Since adapters report always to the API, the status payload should account for this case:

- If there are errors applying the resources
- If conditions from resources are not conclusive

When the HyperFleet API receives an status update with any of the mandatory condition's status to `Unknown` value, the API will not update the internal state. Therefore, Sentinel will keep emitting reconciliation events for status updates.

**Applied** and **Available** are derived from your K8s object's status. **Health** reflects the adapter framework's own execution and uses the standard boilerplate (see section 8).

### Status patterns by resource type

Different Kubernetes resource types expose status differently. Your CEL expressions in the post-action payload need to match the status shape of the objects you create.

#### Namespace

Namespaces have a simple `status.phase` field:

```yaml
# Available when namespace is Active
status:
  expression: |
    resources.?clusterNamespace.?status.?phase.orValue("") == "Active"
      ? "True" : "False"
```

#### Job

Jobs use a `conditions` array. A completed Job has a condition with `type=Complete`:

```yaml
# Available when Job has completed successfully
status:
  expression: |
    resources.?validationJob.?status.?conditions.orValue([])
      .exists(c, c.type == "Complete" && c.status == "True")
    ? "True"
    : resources.?validationJob.?status.?conditions.orValue([])
        .exists(c, c.type == "Failed" && c.status == "True")
      ? "False"
      : "Unknown"
```

A Job that is still running will have no `Complete` or `Failed` condition — the adapter reports `Unknown`, and the next Sentinel cycle will re-evaluate.

#### Custom workloads with conditions

If your workload is a CRD or operator-managed resource that sets its own conditions, read them the same way:

```yaml
# Available when custom resource reports Ready=True
status:
  expression: |
    resources.?myResource.?status.?conditions.orValue([])
      .exists(c, c.type == "Ready" && c.status == "True")
    ? "True" : "False"
```

#### ManifestWork (Maestro)

ManifestWork status is richer — it includes both top-level conditions and per-manifest `statusFeedback`. Use nested discoveries to access individual sub-resource status:

```yaml
# Available from the ManifestWork's own conditions
status:
  expression: |
    resources.?clusterSetup.?status.?conditions.orValue([])
      .filter(c, c.type == "Available").size() > 0
    ? resources.clusterSetup.status.conditions
        .filter(c, c.type == "Available")[0].status
    : "False"

# Or from a nested discovery's statusFeedback
status:
  expression: |
    has(resources.namespace0)
      && has(resources.namespace0.statusFeedback)
      && has(resources.namespace0.statusFeedback.values)
      && resources.namespace0.statusFeedback.values
          .filter(v, v.name == "phase").size() > 0
      && resources.namespace0.statusFeedback.values
          .filter(v, v.name == "phase")[0].fieldValue.string == "Active"
    ? "True" : "False"
```

### Designing your workload for observability

When building the Kubernetes objects that your adapter manages, keep these guidelines in mind:

- **Use standard Kubernetes condition conventions** (`type`, `status`, `reason`, `message`). The adapter's CEL expressions are designed to work with this pattern.
- **Set conditions on your CRDs.** If you control the workload (e.g., a custom operator), have it report `Available`, `Ready`, or `Complete` conditions so the adapter can read them directly.
- **For Jobs, use success/failure exit codes.** Kubernetes automatically sets `Complete` or `Failed` conditions based on container exit codes. The adapter reads these without extra work.
- **For Maestro, configure `feedbackRules`.** Without them, the ManifestWork status won't include sub-resource state, and your nested discoveries will have no data to report on.

### The reconciliation loop

Because the adapter reads status at a point in time, the overall flow is a **convergence loop**:

1. First cycle: adapter creates resources, discovers them immediately — status may be `Pending` or `Unknown`
2. Adapter reports `Applied=True, Available=Unknown` to the API
3. Sentinel detects the cluster is not yet Reconciled (generation mismatch or max-age exceeded)
4. Next cycle: adapter discovers the same resources — status has progressed to `Active` or `Complete`
5. Adapter reports `Applied=True, Available=True`
6. API aggregates: all adapters at current generation with `Available=True` → cluster is `Reconciled`

This means your adapter does not need to poll or wait. The framework and Sentinel handle retry timing. Your job is to write CEL expressions that correctly read the current state, whatever it may be.

---

## 9. Post-Actions (Status Reporting)

> The post-action payload is where you wire the status patterns from Section 8 into the actual report sent to the API.

Post-actions build a status payload and send it to the HyperFleet API. This is how the system knows your adapter's work is done (or failed, or in progress).

The process has two steps: **build payloads**, then **execute post actions**.

### Conditional post-actions (`when`)

Post-actions can be gated with a CEL expression. When the expression evaluates to `false`, the action is **skipped** (not failed) — useful for sending different status reports depending on execution outcome.

```yaml
post_actions:
  - name: "reportSuccess"
    when:
      expression: "adapter.?executionStatus.orValue('') == 'success'"
    api_call:
      method: "PUT"
      url: "/api/hyperfleet/v1/clusters/{{ .clusterId }}/statuses"
      body: "{{ .successPayload }}"

  - name: "reportFailure"
    when:
      expression: "adapter.?executionStatus.orValue('') != 'success'"
    api_call:
      method: "PUT"
      url: "/api/hyperfleet/v1/clusters/{{ .clusterId }}/statuses"
      body: "{{ .failurePayload }}"
```

The `when` expression has access to the full execution context: all `adapter.*` metadata, extracted params, and `resources.*`. If `when` is omitted, the action always executes (existing behavior). If the expression fails to parse or evaluate, the action is marked as **failed**.

### Conditional payloads (`when`)

Individual payloads can also be gated with a CEL expression. When the expression evaluates to `false`, the payload is **not built** and its name is absent from the template context — useful for skipping CEL evaluation of `resources.*` values that don't exist when preconditions are not met, or for building entirely different payloads for creation vs. deletion paths without deeply nested ternaries. A post-action that references a skipped payload is **silently skipped** (not failed).

```yaml
post:
  payloads:
    - name: "statusPayload"
      when:
        expression: "!adapter.resourcesSkipped"
      build:
        namespace:
          expression: 'resources.?clusterNamespace.?status.?phase.orValue("Pending")'

    - name: "skippedStatusPayload"
      when:
        expression: "adapter.resourcesSkipped"
      build:
        reason:
          expression: 'adapter.skipReason'

  post_actions:
    - name: "reportStatus"
      api_call:
        method: "PUT"
        url: "/api/hyperfleet/v1/clusters/{{ .clusterId }}/statuses"
        body: "{{ .statusPayload }}"

    - name: "reportSkipped"
      api_call:
        method: "PUT"
        url: "/api/hyperfleet/v1/clusters/{{ .clusterId }}/statuses"
        body: "{{ .skippedStatusPayload }}"
```

The `when` expression has access to the full execution context: all `adapter.*` metadata, extracted params, and `resources.*`. If `when` is omitted, the payload is always built (existing behavior). If the expression fails to parse or evaluate, the payload build is marked as **failed**. Evaluation order: payload `when` → build → post-action `when` → execute. Both gates are independent.

### Building payloads

A payload is a JSON structure built from CEL expressions and Go Templates. Each field can be specified in three ways:

| Form | Example | Use when |
|------|---------|----------|
| Direct string | `adapter: "my-adapter"` | Static values |
| CEL expression | `status: { expression: "..." }` | Computed values, conditionals |
| Field extraction | `status: { field: "path", default: "..." }` | Simple field reads |

### Condition types

Every adapter status reports three condition types:

| Type | Question it answers |
|------|---------------------|
| **Applied** | Were the Kubernetes resources created/configured? |
| **Available** | Are the resources operational and serving? |
| **Health** | Did the adapter execution itself succeed? |

### Minimal payload example

<details><summary>Minimal payload example</summary>

```yaml
post:
  payloads:
    - name: "statusPayload"
      build:
        adapter: "{{ .adapter.name }}"
        conditions:
          - type: "Applied"
            status:
              expression: |
                has(resources.clusterNamespace) ? "True" : "False"
            reason:
              expression: |
                has(resources.clusterNamespace) ? "Applied" : "Pending"
            message:
              expression: |
                has(resources.clusterNamespace)
                  ? "Resources applied successfully"
                  : "Resources pending"

          - type: "Available"
            status:
              expression: |
                resources.?clusterNamespace.?status.?phase.orValue("") == "Active"
                  ? "True" : "False"
            reason:
              expression: |
                resources.?clusterNamespace.?status.?phase.orValue("") == "Active"
                  ? "NamespaceReady" : "NamespaceNotReady"
            message:
              expression: |
                resources.?clusterNamespace.?status.?phase.orValue("") == "Active"
                  ? "Namespace is active" : "Namespace not yet active"

          - type: "Health"
            # ... (see standard boilerplate below)

        observed_generation:
          expression: "generation"
        observed_time: "{{ now | date \"2006-01-02T15:04:05Z07:00\" }}"

  post_actions:
    - name: "reportStatus"
      api_call:
        method: "PUT"
        url: "/api/hyperfleet/v1/clusters/{{ .clusterId }}/statuses"
        body: "{{ .statusPayload }}"
```

</details>

### The `observed_generation` gotcha

Always use a **CEL expression** for `observed_generation`, not a Go Template. Go Templates output strings, but the API expects an integer. CEL preserves the numeric type:

```yaml
# Correct — preserves integer type
observed_generation:
  expression: "generation"

# Wrong — sends a string "5" instead of integer 5
observed_generation: "{{ .generation }}"
```

### The Health condition boilerplate

The Health condition follows a standard pattern that surfaces execution errors and skip reasons. Copy this into your adapter and leave it as-is:

<details>
<summary>Standard Health condition (click to expand)</summary>

```yaml
- type: "Health"
  status:
    expression: |
      adapter.?executionStatus.orValue("") == "success"
        && !adapter.?resourcesSkipped.orValue(false)
      ? "True"
      : "False"
  reason:
    expression: |
      adapter.?executionStatus.orValue("") != "success"
      ? "ExecutionFailed:" + adapter.?executionError.?phase.orValue("unknown")
      : adapter.?resourcesSkipped.orValue(false)
        ? "ResourcesSkipped"
        : "Healthy"
  message:
    expression: |
      adapter.?executionStatus.orValue("") != "success"
      ? "Adapter failed at phase ["
          + adapter.?executionError.?phase.orValue("unknown")
          + "] step ["
          + adapter.?executionError.?step.orValue("unknown")
          + "]: "
          + adapter.?executionError.?message.orValue(
              adapter.?errorMessage.orValue("no details"))
      : adapter.?resourcesSkipped.orValue(false)
        ? "Resources skipped: " + adapter.?skipReason.orValue("unknown reason")
        : "Adapter execution completed successfully"
```

</details>

### The `data` field

Optionally attach adapter-specific metrics extracted from your resources:

```yaml
        data:
          namespace:
            name:
              expression: |
                resources.?clusterNamespace.?metadata.?name.orValue("")
            phase:
              expression: |
                resources.?clusterNamespace.?status.?phase.orValue("")
```

### How status aggregation works

When your adapter reports status, the API aggregates across **all registered adapters**:

- **Available** = all adapters report `Available=True` at *any* generation (last known good)
- **Reconciled** = all adapters report `Available=True` at the *current* generation (fully reconciled)

Your adapter name must be registered in the `HYPERFLEET_CLUSTER_ADAPTERS` environment variable on the API for it to participate in aggregation.

---

## 10. Dry-Run Mode

Dry-run mode simulates the full execution pipeline locally. No Kubernetes cluster, no message broker, no API server needed. This is useful to test the creation of adapter-task-config files, as expressions can get complex and going through the real cycle of deploying the adapter is slow.

### Running a dry-run

```bash
hyperfleet-adapter serve \
  --config ./adapter-config.yaml \
  --task-config ./adapter-task-config.yaml \
  --dry-run-event ./event.json \
  --dry-run-api-responses ./api-responses.json \
  --dry-run-discovery ./discovery-overrides.json \
  --dry-run-verbose \
  --dry-run-output text    # or "json"
```

### Mock input files

You need three files to simulate the environment.

#### 1. Event file (`event.json`)

A standard CloudEvent with the data your adapter expects:
<details><summary>event file example</summary>

```json
{
  "specversion": "1.0",
  "id": "abc123",
  "type": "io.hyperfleet.cluster.updated",
  "source": "/api/hyperfleet/v1/clusters/abc123",
  "data": {
    "id": "abc123",
    "kind": "Cluster",
    "href": "/api/hyperfleet/v1/clusters/abc123",
    "generation": 5
  }
}
```

</details>

#### 2. API responses (`api-responses.json`)

Mock responses matched by HTTP method and URL regex. Supports sequential responses for endpoints called multiple times:

<details><summary>HyperFleet API response for statuses update</summary>

```json
{
  "responses": [
    {
      "match": {
        "method": "GET",
        "urlPattern": "/api/hyperfleet/v1/clusters/.*"
      },
      "responses": [
        {
          "statusCode": 200,
          "body": {
            "id": "abc123",
            "name": "my-cluster",
            "generation": 5,
            "status": {
              "conditions": [
                { "type": "Reconciled", "status": "False" }
              ]
            }
          }
        }
      ]
    },
    {
      "match": {
        "method": "PUT",
        "urlPattern": "/api/hyperfleet/v1/clusters/.*/statuses"
      },
      "responses": [
        { "statusCode": 200, "body": {} }
      ]
    }
  ]
}
```

</details>

#### 3. Discovery overrides (`discovery-overrides.json`)

Simulates the server-populated fields (uid, resourceVersion, status) that Kubernetes would add after creating resources. Keys are the **rendered resource names**:

```json
{
  "abc123": {
    "apiVersion": "v1",
    "kind": "Namespace",
    "metadata": {
      "name": "abc123",
      "uid": "a1b2c3d4-e5f6-7890-abcd-ef1234567890",
      "resourceVersion": "100"
    },
    "status": {
      "phase": "Active"
    }
  }
}
```

### Reading the trace output

The trace walks through each phase showing what happened:

<details><summary>Example of a Dry-run execution</summary>

```
Dry-Run Execution Trace
========================
Event: id=abc123 type=io.hyperfleet.cluster.updated

Phase 1: Parameter Extraction .............. SUCCESS
  clusterId        = "abc123"
  generation       = 5
  region           = "us-east-1"

Phase 2: Preconditions ..................... SUCCESS (MET)
  [1/1] fetch-cluster                      PASS
    API Call: GET /api/hyperfleet/v1/clusters/abc123 -> 200
    Captured: clusterName = "my-cluster"
    Captured: reconciledStatus = "False"

Phase 3: Resources ........................ SUCCESS
  [1/2] namespace0                         CREATE
    Kind: Namespace    Namespace:            Name: abc123
  [2/2] configmap0                         CREATE
    Kind: ConfigMap    Namespace: abc123     Name: abc123-config

Phase 3.5: Discovery Results ................. (available as resources.* in payload)
  namespace0:
    {"apiVersion":"v1","kind":"Namespace","metadata":{"name":"abc123",...},"status":{"phase":"Active"}}

Phase 4: Post Actions ..................... SUCCESS
  [1/1] update-status                      EXECUTED
    API Call: PUT /api/hyperfleet/v1/clusters/abc123/statuses -> 200

Result: SUCCESS
```

</details>

Use `--dry-run-verbose` to see rendered manifests and full API request/response bodies. Use `--dry-run-output json` for machine-readable output you can pipe into `jq`.

### Development loop

1. Write your `adapter-task-config.yaml`
2. Create mock files for a representative cluster
3. Run dry-run, inspect the trace
4. Fix config issues, re-run
5. Test edge cases: change mock API responses to simulate different cluster states (Reconciled=True, missing fields, error responses)
6. Deploy when the trace shows the expected behavior

---

## 11. NodePool Adapters

NodePool adapters follow the same pattern as cluster adapters with a few differences.

### Event structure

NodePool events include an `owner_references` pointing to the parent cluster:

```yaml
params:
  - name: "clusterId"
    source: "event.owner_references.id"    # Parent cluster
    type: "string"
    required: true

  - name: "nodepoolId"
    source: "event.id"                    # The NodePool itself
    type: "string"
    required: true
```

### Checking parent cluster readiness

NodePool adapters typically wait for the parent cluster to be fully set up. Query the parent cluster's adapter statuses as a precondition:

<details><summary>NodePool precondition example</summary>

```yaml
preconditions:
  - name: "nodepoolStatus"
    api_call:
      url: "/api/hyperfleet/v1/clusters/{{ .clusterId }}/nodepools/{{ .nodepoolId }}"
    capture:
      - name: "generation"
        field: "generation"
      - name: "reconciledStatus"
        expression: |
          status.conditions.filter(c, c.type == "Reconciled").size() > 0
            ? status.conditions.filter(c, c.type == "Reconciled")[0].status
            : "False"
    conditions:
      - field: "reconciledStatus"
        operator: "equals"
        value: "False"

  - name: "clusterAdapterStatus"
    api_call:
      url: "/api/hyperfleet/v1/clusters/{{ .clusterId }}/statuses"
    capture:
      - name: "clusterNamespaceStatus"
        field: "{.items[?(@.adapter=='landing-zone')].data.namespace.status}"
    conditions:
      - field: "clusterNamespaceStatus"
        operator: "equals"
        value: "Active"
```

</details>

### Reporting NodePool status

Post-actions target the NodePool status endpoint instead of the cluster one:

```yaml
post_actions:
  - name: "reportNodepoolStatus"
    when:
      expression: "!adapter.resourcesSkipped"
    api_call:
      method: "PUT"
      url: "/api/hyperfleet/v1/clusters/{{ .clusterId }}/nodepools/{{ .nodepoolId }}/statuses"
      body: "{{ .nodepoolStatusPayload }}"
```

Register NodePool adapters in `HYPERFLEET_NODEPOOL_ADAPTERS` (not `HYPERFLEET_CLUSTER_ADAPTERS`).

---

## 12. Testing and Validation

### Configuration validation

The framework validates your config at load time in two passes:

**Structural validation** — checked always:

- Required fields present (`name`, `source`, `method`, etc.)
- Valid operator values
- Mutual exclusivity (`field` vs `expression`, `build` vs `build_ref`)
- Valid Kubernetes resource names

**Semantic validation** — checked by default (can be skipped):

- CEL expressions parse without errors
- Go template variables reference defined params or captures
- `in`/`notIn` operators have array values

> **Note:** K8s structural validation (required fields like `apiVersion`, `kind`, `metadata.name`) is deferred to execution time since all manifests are rendered as Go templates. Invalid manifests will be caught when the adapter applies them.

### No-op adapter pattern

To test preconditions and post-actions without creating any resources, leave the resources section empty:

```yaml
resources: []
```

The adapter will run preconditions, skip straight to post-actions, and report status. Useful for validation adapters or framework testing.

### Common validation errors

| Error | Cause | Fix |
|-------|-------|-----|
| `required field missing` | Param without `name` or `source` | Add the required field |
| `mutually exclusive` | Both `field` and `expression` on a capture | Use only one |
| `CEL parse error` | Invalid CEL syntax | Check parentheses, string escaping |
| `template variable not found` | `{{ .foo }}` where `foo` is not a param or capture | Define it in params or captures |
| `invalid operator` | Typo in operator name | Use one from the supported list |

---

## 13. Deployment Checklist

1. **Register your adapter name** in the HyperFleet API's `HYPERFLEET_CLUSTER_ADAPTERS` (or `HYPERFLEET_NODEPOOL_ADAPTERS`) environment variable. Without this, the API won't include your adapter in status aggregation.

- The API will compute the `Reconciled` condition of the managed object as when all registered adapters have reported `True` as their `Available` condition status.

1. **Create the AdapterConfig** with your environment's API endpoint, broker subscription, and client settings:

<details><summary>Example minimal adapter-config</summary>

```yaml
adapter:
  name: my-adapter
  version: "0.1.0"
clients:
  hyperfleet_api:
    base_url: "http://hyperfleet-api:8000"
    timeout: 10s
    retry_attempts: 3
    retry_backoff: exponential
  broker:
    subscription_id: "my-adapter"   # must be unique per adapter — shared IDs cause competing consumers, not fan-out
    topic: "hyperfleet-clusters"    # for RabbitMQ: queue name prefix only (not a routing key)
  kubernetes:
    api_version: "v1"
```

</details>

1. **Configure the broker connection** — the Helm chart creates a `broker.yaml` ConfigMap from the `broker.*` Helm values. For RabbitMQ, set `broker.rabbitmq.exchange` to the value of the sentinel's `clients.broker.topic` — this is the exchange the sentinel publishes to and the only coupling point between them.

   ```yaml
   # Helm values
   broker:
     type: rabbitmq
     rabbitmq:
       url: "amqp://user:pass@rabbitmq:5672/"
       exchange: "hyperfleet-clusters"   # must match sentinel's clients.broker.topic
       exchange_type: "topic"
   ```

1. **Deploy using the Helm chart** — the generic `adapter/` chart mounts your task config as a ConfigMap and sets the environment variables.

2. **Set up broker subscription** — for Google Pub/Sub, ensure your adapter has a dedicated subscription on the cluster events topic so it receives events independently of other adapters (fan-out pattern). For RabbitMQ, fan-out is achieved automatically by giving each adapter a unique `subscription_id` — the broker library creates a separate queue per adapter.

3. Set permissions for the adapter to read from the broker subscription. This is cloud provider specific.

- E.g. In GCP you can use Workload Identity Federation to assign `role/pubsub.subscriber` directly to the k8s service account for the adapters.

More information about deployment can be found in [Architecture repository - HyperFleet Adapter Framework - Deployment Guide](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/components/adapter/framework/adapter-deployment.md)

1. **Verify broker metrics** — the adapter automatically exposes broker metrics on the `/metrics` endpoint (port 9090). No additional configuration is needed. See [Metrics](metrics.md) for the full list of available metrics.

---

## Appendix A: CEL Quick Reference

> For the full CEL reference including implementation details, see [CEL Conventions](conventions/cel.md).

```cel
# Optional chaining — safe access to fields that may not exist
resources.?clusterNamespace.?status.?phase.orValue("")

# Existence check
has(resources.clusterNamespace)

# Array filtering — find a condition by type
status.conditions.filter(c, c.type == "Reconciled")

# Array existence check
status.conditions.exists(c, c.type == "Reconciled" && c.status == "True")

# Get first matching element with fallback
status.conditions.filter(c, c.type == "Reconciled").size() > 0
  ? status.conditions.filter(c, c.type == "Reconciled")[0].status
  : "False"

# Ternary
condition ? "yes" : "no"

# String concatenation
"prefix-" + clusterId + "-suffix"

# Numeric comparison (use expression for observed_generation)
generation

# JSON serialization (debugging)
toJson(resources.resource0)
```

### String extension functions (`ext.Strings()`)

The CEL environment registers `ext.Strings()`, making the following methods available on string values:

`charAt`, `indexOf`, `lastIndexOf`, `lowerAscii`, `upperAscii`, `replace`, `split`, `substring`, `trim`, `join`

```cel
# Lowercase a cluster name
clusterName.lowerAscii()

# Split a comma-separated list and check membership
"us-east-1,us-west-2".split(",").exists(r, r == region)

# Trim whitespace from a captured value
resources.?myResource.?metadata.?name.orValue("").trim()
```

### Common patterns

<details>
<summary>Extract a condition status from a Kubernetes-style conditions array</summary>

```cel
resources.?myResource.?status.?conditions.orValue([])
  .exists(c, c.type == "Available")
? resources.myResource.status.conditions
    .filter(c, c.type == "Available")[0].status
: "Unknown"
```

</details>

<details>
<summary>Check ManifestWork statusFeedback for a namespace phase</summary>

```cel
has(resources.namespace0)
  && has(resources.namespace0.statusFeedback)
  && has(resources.namespace0.statusFeedback.values)
  && resources.namespace0.statusFeedback.values
      .filter(v, v.name == "phase").size() > 0
? resources.namespace0.statusFeedback.values
    .filter(v, v.name == "phase")[0].fieldValue.string
: ""
```

</details>

<details>
<summary>Build a composite status from multiple resources</summary>

```cel
has(resources.namespace0) && has(resources.configmap0)
  ? "True"
  : "False"
```

</details>

---

## Appendix B: Go Template Quick Reference

### Variable interpolation

```
{{ .variableName }}                              Variable interpolation
{{ .clusterId | lower }}                         Lowercase filter
{{ now | date "2006-01-02T15:04:05Z07:00" }}     Current timestamp (RFC 3339)
{{ .adapter.name }}                              Adapter name from config
```

### Structural syntax

Go templates support conditional logic and iteration for producing dynamic YAML based on captured values. Structural directives work in:

- **External manifest files** (`manifest.ref`) — always treated as raw Go templates
- **Inline block scalars** (`manifest: |`) — the `|` preserves raw text for template rendering

Structural directives do **not** work in plain inline manifests (without `|`) because YAML parsing runs before template rendering.

**Conditionals (`if` / `else`)**

```yaml
{{ if .platformType }}
    hyperfleet.io/platform-type: "{{ .platformType }}"
{{ end }}

{{ if eq .environment "production" }}
    tier: "critical"
{{ else }}
    tier: "standard"
{{ end }}
```

**Iteration (`range`)**

Use `range` to iterate over list-type values captured via CEL expressions:

```yaml
{{ range $i, $subnet := .subnets }}
    subnet_{{ $subnet.id }}_name: "{{ $subnet.name }}"
    subnet_{{ $subnet.id }}_cidr: "{{ $subnet.cidr }}"
{{ end }}
```

> **Note:** To iterate over a list, the corresponding precondition capture must use a CEL expression that returns the list directly (not a string). For example:
>
> ```yaml
> captures:
>   - name: "subnets"
>     expression: |
>       has(spec.platform) && has(spec.platform.gcp) && has(spec.platform.gcp.subnets)
>         ? spec.platform.gcp.subnets
>         : []
> ```

Go Templates are used in: URLs, manifest field values, direct string values in payloads, external template files (`manifest.ref`), and inline block scalars (`manifest: |`).

> **Tip:** Go date format uses the reference time `Mon Jan 2 15:04:05 MST 2006` as the layout. The digits are not arbitrary — `2006` is the year, `01` is the month, etc.

---

## Appendix C: Condition Operators Reference

See also [Preconditions — Supported operators](#supported-operators).

| Operator | Value type | Example |
|----------|-----------|---------|
| `equals` | any | `value: "True"` |
| `notEquals` | any | `value: "Terminating"` |
| `in` | array | `value: ["us-east-1", "us-west-2"]` |
| `notIn` | array | `value: ["deprecated-region"]` |
| `contains` | string | `value: "prod"` |
| `greaterThan` | numeric | `value: 0` |
| `greaterThanOrEqual` | numeric | `value: 0` |
| `lessThan` | numeric | `value: 100` |
| `lessThanOrEqual` | numeric | `value: 100` |
| `exists` | (none) | Field must exist, no value needed |
| `notExists` | (none) | Field must not exist, no value needed |

---

## Appendix D: Troubleshooting

| Symptom | Likely cause | Solution |
|---------|-------------|----------|
| Resources skipped, Health=False with "ResourcesSkipped" | Precondition not met | Check precondition conditions — the cluster may not be in the expected state yet. This is often normal; the Sentinel will retry. |
| Status update rejected by API | Stale `observed_generation` | Your adapter is reporting an older generation than what's already stored. Ensure `observed_generation` uses the generation from the API response, not the event. |
| `template variable not found` | Variable referenced in `{{ .foo }}` but never defined | Add `foo` to params or captures. Check spelling. |
| `CEL expression parse error` | Invalid CEL syntax | Verify parentheses, string quoting, and optional chaining syntax (`?.` for safe field access). |
| Discovery returns empty | Labels don't match or wrong namespace | Verify `discovery.namespace` is correct. Use `by_name` for a simpler lookup. Check resource labels match the selector exactly. |
| `observed_generation` is a string | Using Go Template instead of CEL expression | Use `expression: "generation"` instead of `"{{ .generation }}"`. |
| Post-action API call returns 404 with error status | Wrong status endpoint path (error code `HYPERFLEET-NTF-000`) | Cluster statuses: `/clusters/{id}/statuses`. NodePool statuses: `/clusters/{id}/nodepools/{id}/statuses`. |
