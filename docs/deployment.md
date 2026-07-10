# Deployment Guide

> **Audience:** Operators deploying the adapter to Kubernetes via Helm.

This guide explains how to configure and deploy an adapter instance using the Helm chart. For the exhaustive list of every Helm value with types and defaults, see the [Helm Values Reference](../charts/README.md).

## Configuration Overview

The HyperFleet Adapter Helm chart is released as an OCI artifact at `oci://quay.io/redhat-services-prod/hyperfleet-tenant/hyperfleet/hyperfleet-adapter-chart`.

An adapter deployment requires three pieces of configuration, all settable through Helm values:

| Config | Helm value | Purpose |
|--------|-----------|---------|
| **Adapter config** | `adapterConfig.*` | Infrastructure: adapter identity, client connections, timeouts |
| **Task config** | `adapterTaskConfig.*` | Business logic: params, preconditions, resources, status reporting |
| **Broker config** | `broker.*` | Message broker connection: Pub/Sub or RabbitMQ settings |

The adapter binary loads two config files at startup (adapter config + task config). The Helm chart creates ConfigMaps for both and mounts them into the pod. Some adapter config fields also have environment variable overrides — the chart auto-sets several of these from dedicated Helm values, and you can set additional ones via the `env` list.

### How adapter config fields map to Helm values

There are three ways to set deployment config fields, in decreasing order of convenience:

| Mechanism | When to use | Example |
|-----------|------------|---------|
| **`adapterConfig.yaml` blob** | Fields with no env var support | Provide the full adapter config as inline YAML |
| **Dedicated Helm value** | Fields the chart exposes directly | `adapterConfig.log.level: debug` |
| **`env` list** | Fields with env var support but no dedicated Helm value | `env: [{name: HYPERFLEET_MAESTRO_GRPC_SERVER_ADDRESS, value: "..."}]` |

The [Configuration Reference](configuration.md) lists every field with its env var and CLI flag equivalents.

---

## Adapter Config (yaml blob)

The adapter config defines the adapter identity, logging, and client connections (Maestro, HyperFleet API, Kubernetes). It can be sourced in three ways:

| Option | Helm value | Description |
|--------|-----------|-------------|
| Inline YAML | `adapterConfig.yaml` | Full adapter config as YAML (structured object or string) |
| Chart-packaged files | `adapterConfig.files` | Reference files bundled with the chart via `$.Files.Get` |
| Existing ConfigMap | `adapterConfig.configMapName` | Pre-existing ConfigMap (set `adapterConfig.create: false`) |

### Fields requiring `adapterConfig.yaml`

These fields have no environment variable override and can only be set by providing the full adapter config as inline YAML via `adapterConfig.yaml`:

- `adapter.name` — adapter identity name
- `adapter.version` — version string for binary validation
- `clients.hyperfleet_api.default_headers` — custom headers for API requests

When you use `adapterConfig.yaml`, the Helm values `adapterConfig.log.level`, `adapterConfig.hyperfleetApi.baseUrl`, and `adapterConfig.hyperfleetApi.version` still work — they are injected as env vars which take priority over the YAML file.

Example with inline YAML:

```yaml
adapterConfig:
  create: true
  yaml:
    adapter:
      name: my-adapter
      version: "0.1.0"
    clients:
      hyperfleet_api:
        base_url: http://hyperfleet-api:8000
        version: v1
        timeout: 10s
        retry_attempts: 3
        retry_backoff: exponential
      broker:
        subscription_id: my-adapter-sub
        topic: cluster-events
      kubernetes:
        api_version: "v1"
```

### Dedicated Helm values

These fields have first-class Helm values that the chart injects as environment variables:

| Parameter | Description | Env var set by chart | Default |
|-----------|-------------|---------------------|---------|
| `adapterConfig.log.level` | Log level (`debug`, `info`, `warn`, `error`) | `LOG_LEVEL` | `info` |
| `adapterConfig.hyperfleetApi.baseUrl` | HyperFleet API base URL | `HYPERFLEET_API_BASE_URL` | `http://hyperfleet-api:8000` |
| `adapterConfig.hyperfleetApi.version` | API version | `HYPERFLEET_API_VERSION` | `v1` |
| `adapterConfig.hyperfleetApi.auth.enabled` | Enable JWT bearer token auth | — (controls volume + env vars) | `false` |
| `adapterConfig.hyperfleetApi.auth.tokenPath` | Absolute path to the token file | `HYPERFLEET_API_AUTH_TOKEN_PATH` | `/var/run/secrets/hyperfleet/token` |
| `adapterConfig.hyperfleetApi.auth.tokenCacheTtl` | In-memory token cache TTL | `HYPERFLEET_API_AUTH_TOKEN_CACHE_TTL` | `30s` |
| `adapterConfig.hyperfleetApi.auth.audience` | ServiceAccount token audience | — (used in projected volume) | `hyperfleet-api` |
| `adapterConfig.hyperfleetApi.auth.expirationSeconds` | ServiceAccount token lifetime (seconds) | — (used in projected volume) | `3600` |

### Fields settable via the `env` list

Many adapter config fields have environment variable overrides but no dedicated Helm value. Set them using the `env` list. The naming convention is `HYPERFLEET_<SECTION>_<FIELD>` for most fields. For example:

```yaml
env:
  - name: HYPERFLEET_MAESTRO_GRPC_SERVER_ADDRESS
    value: "maestro-grpc.maestro.svc.cluster.local:8090"
  - name: HYPERFLEET_MAESTRO_TIMEOUT
    value: "30s"
  - name: LOG_FORMAT
    value: "text"
```

The [Configuration Reference](configuration.md#environment-variables) lists every adapter config field with its environment variable equivalent.

---

## Task Config

The task config defines the adapter's business logic (params, preconditions, resources, post-actions). See the [Adapter Authoring Guide](adapter-authoring-guide.md) for how to write one.

| Option | Helm value | Description |
|--------|-----------|-------------|
| Inline YAML | `adapterTaskConfig.yaml` | Full task config as YAML (mutually exclusive with files/external) |
| Chart-packaged files | `adapterTaskConfig.files` | Reference files bundled with the chart (can combine with external) |
| External content | `adapterTaskConfig.external` | Content passed via `--set-file` (can combine with files) |
| Existing ConfigMap | `adapterTaskConfig.configMapName` | Pre-existing ConfigMap (set `adapterTaskConfig.create: false`) |

Example with `--set-file`:

```bash
helm install my-adapter ./charts/ \
  --set-file adapterTaskConfig.external.task-config=/path/to/my-task-config.yaml \
  -f my-values.yaml
```

> **Note:** Files referenced via `manifest.ref` in the task config are not processed by Helm — they use Go templates resolved at adapter runtime.

---

## Broker Configuration

The broker config controls the message broker connection. It can be sourced in three ways:

| Option | Helm value | Description |
|--------|-----------|-------------|
| Individual properties | `broker.googlepubsub.*` or `broker.rabbitmq.*` | Chart builds the broker YAML |
| Inline YAML | `broker.yaml` | Full broker config as a YAML string |
| Existing ConfigMap | `broker.configMapName` | Pre-existing ConfigMap (set `broker.create: false`) |

When using individual properties, `broker.type` must be set to `googlepubsub` or `rabbitmq`. See the [Helm Values Reference](../charts/README.md) for the full list of `broker.googlepubsub.*` and `broker.rabbitmq.*` parameters with defaults. For broker config semantics (queue naming, fan-out behavior, exchange coupling), see the [Configuration Reference](configuration.md#broker).

---

## HyperFleet API Authentication

The adapter can authenticate to the HyperFleet API using a Kubernetes projected ServiceAccount token (JWT bearer token). Authentication is **disabled by default** — existing deployments are unaffected.

When enabled, the Helm chart:
1. Mounts a projected `serviceAccountToken` volume at the configured `tokenPath` directory.
2. Sets `HYPERFLEET_API_AUTH_TOKEN_PATH` and `HYPERFLEET_API_AUTH_TOKEN_CACHE_TTL` env vars.
3. The adapter reads the token file and attaches `Authorization: Bearer <token>` to every HyperFleet API request.

```yaml
adapterConfig:
  hyperfleetApi:
    auth:
      enabled: true
      audience: hyperfleet-api        # token audience claimed by the API server
      tokenPath: /var/run/secrets/hyperfleet/token
      expirationSeconds: 3600         # kubelet rotates the token before expiry
      tokenCacheTtl: 30s              # re-read file every 30s; 0 = re-read per request
```

`tokenPath` must be an absolute path (validated at `helm install`/`upgrade` time).

The token file is managed by the kubelet and rotated automatically before `expirationSeconds` elapses. Setting `tokenCacheTtl` shorter than the rotation interval ensures the adapter picks up a fresh token before the old one expires.

---

## Tracing

OpenTelemetry distributed tracing. The Helm chart defaults to tracing **disabled** (the binary defaults to enabled). When no endpoint is configured, traces are written to stdout.

See the [Helm Values Reference](../charts/README.md) for the full list of `tracing.*` parameters and defaults, and the [Configuration Reference](configuration.md#tracing-opentelemetry) for the underlying OTEL environment variables.

---

## Environment Variables

### Auto-set by the chart

The chart automatically sets these environment variables from Helm values:

| Variable | Source | Condition |
|----------|--------|-----------|
| `LOG_LEVEL` | `adapterConfig.log.level` | Always |
| `HYPERFLEET_API_BASE_URL` | `adapterConfig.hyperfleetApi.baseUrl` | Always |
| `HYPERFLEET_API_VERSION` | `adapterConfig.hyperfleetApi.version` | Always |
| `HYPERFLEET_API_AUTH_TOKEN_PATH` | `adapterConfig.hyperfleetApi.auth.tokenPath` | When `auth.enabled` is `true` |
| `HYPERFLEET_API_AUTH_TOKEN_CACHE_TTL` | `adapterConfig.hyperfleetApi.auth.tokenCacheTtl` | When `auth.enabled` is `true` |
| `BROKER_CONFIG_FILE` | Hardcoded `/etc/broker/broker.yaml` | Always |
| `HYPERFLEET_BROKER_SUBSCRIPTION_ID` | `broker.googlepubsub.subscriptionId` | When broker type is `googlepubsub` |
| `HYPERFLEET_BROKER_TOPIC` | `broker.googlepubsub.topic` | When broker type is `googlepubsub` |
| `BROKER_URL` | `broker.rabbitmq.url` | When broker type is `rabbitmq` |
| `BROKER_QUEUE` | `broker.rabbitmq.queue` | When broker type is `rabbitmq` |
| `HYPERFLEET_TRACING_ENABLED` | `tracing.enabled` | Always |
| `OTEL_SERVICE_NAME` | `tracing.serviceName` | When tracing is enabled |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `tracing.otlpEndpoint` | When tracing is enabled |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `tracing.otlpProtocol` | When tracing is enabled |
| `OTEL_TRACES_SAMPLER` | `tracing.sampler` | When tracing is enabled |
| `OTEL_TRACES_SAMPLER_ARG` | `tracing.samplerArg` | When tracing is enabled |
| `OTEL_PROPAGATORS` | `tracing.propagators` | When tracing is enabled |
| `K8S_NAMESPACE` | Pod field `metadata.namespace` | When tracing is enabled |
| `OTEL_RESOURCE_ATTRIBUTES` | Hardcoded `k8s.namespace.name=$(K8S_NAMESPACE)` | When tracing is enabled |

### Custom environment variables

Use the `env` list to inject arbitrary environment variables into the adapter container. This is the primary mechanism for setting adapter config fields that don't have dedicated Helm values (see [Fields settable via the `env` list](#fields-settable-via-the-env-list)).

```yaml
env:
  - name: MY_VAR
    value: "my-value"
  - name: MY_SECRET
    valueFrom:
      secretKeyRef:
        name: my-secret
        key: secret-key
```

---

## GCP Workload Identity

To use Google Pub/Sub with Workload Identity, bind a principal to the Kubernetes service account:

```bash
gcloud projects add-iam-policy-binding MY_PROJECT \
  --member="principal://iam.googleapis.com/projects/PROJECT_NUMBER/locations/global/workloadIdentityPools/POOL/subject/ns/NAMESPACE/sa/SERVICE_ACCOUNT" \
  --role="roles/pubsub.subscriber"
```

---

## Examples

### Minimal (RabbitMQ, chart-packaged files)

```yaml
image:
  registry: quay.io
  repository: redhat-services-prod/hyperfleet-tenant/hyperfleet/hyperfleet-adapter
  tag: <version>

adapterConfig:
  create: true
  files:
    adapter-config.yaml: examples/kubernetes/adapter-config.yaml

adapterTaskConfig:
  create: true
  files:
    task-config.yaml: examples/kubernetes/adapter-task-config.yaml

broker:
  create: true
  type: rabbitmq
  rabbitmq:
    url: amqp://user:password@rabbitmq.hyperfleet-system.svc.cluster.local:5672/
    queue: my-adapter
    exchange: hyperfleet
    exchangeType: topic
    routingKey: adapter.events # Optional
```

### Full (inline adapter config, external task config, Maestro, Pub/Sub, tracing)

```yaml
image:
  registry: quay.io
  repository: redhat-services-prod/hyperfleet-tenant/hyperfleet/hyperfleet-adapter
  tag: <version>

adapterConfig:
  create: true
  yaml:
    adapter:
      name: my-adapter
      version: "<version-no-v-prefix>"
    clients:
      hyperfleet_api:
        base_url: http://hyperfleet-api:8000
        version: v1
        timeout: 10s
        retry_attempts: 3
        retry_backoff: exponential
      maestro:
        grpc_server_address: "maestro-grpc.maestro.svc.cluster.local:8090"
        source_id: "my-adapter"
        auth:
          type: tls
          tls_config:
            ca_file: /etc/maestro/certs/grpc/ca.crt
            cert_file: /etc/maestro/certs/grpc/client.crt
            key_file: /etc/maestro/certs/grpc/client.key
        timeout: 30s
        keepalive:
          time: 30s
          timeout: 10s
      broker:
        subscription_id: my-adapter-sub
        topic: cluster-events
      kubernetes:
        api_version: "v1"
        qps: 100
        burst: 200

adapterTaskConfig:
  create: true
  external:
    task-config: |
      params:
        - name: "clusterId"
          source: "event.id"
          type: "string"
          required: true
      resources: []
      post:
        payloads: []
        post_actions: []

broker:
  create: true
  type: googlepubsub
  googlepubsub:
    projectId: my-gcp-project
    topic: cluster-events
    subscriptionId: my-adapter-sub

env:
  - name: NAMESPACE
    valueFrom:
      fieldRef:
        fieldPath: metadata.namespace

tracing:
  enabled: true
  otlpEndpoint: http://otel-collector:4317

rbac:
  resources:
    - namespaces
    - configmaps

extraVolumes:
  - name: maestro-certs
    secret:
      secretName: maestro-client-certs
extraVolumeMounts:
  - name: maestro-certs
    mountPath: /etc/maestro/certs/grpc
    readOnly: true
```

See `charts/examples/kubernetes/` for additional working examples.
