# Adapter Configuration Reference

> **Audience:** Operators deploying and configuring the hyperfleet-adapter service.

This document describes the deployment-level `AdapterConfig` options and how to set them
in three formats: YAML, command-line flags, and environment variables.

Overrides are applied in this order: CLI flags > environment variables > YAML file > defaults.

## Config file location

You can point the adapter at a deployment config file with either:

- CLI: `--config` (or `-c`)
- Env: `HYPERFLEET_ADAPTER_CONFIG`

Task config is separate (`--task-config` / `HYPERFLEET_TASK_CONFIG`) and not covered here.

## YAML options (AdapterConfig)

All fields use **snake_case** naming.

```yaml
adapter:
  name: example-adapter
  version: "0.1.0"

debug_config: false

log:
  level: "info"
  format: "json"
  output: "stdout"

clients:
  maestro:
    grpc_server_address: "maestro-grpc.maestro.svc.cluster.local:8090"
    http_server_address: "https://maestro-api.maestro.svc.cluster.local"
    source_id: "hyperfleet-adapter"
    client_id: "hyperfleet-adapter-client"
    auth:
      type: "tls"
      tls_config:
        ca_file: "/etc/maestro/certs/grpc/ca.crt"
        cert_file: "/etc/maestro/certs/grpc/client.crt"
        key_file: "/etc/maestro/certs/grpc/client.key"
        http_ca_file: "/etc/maestro/certs/https/ca.crt"
    timeout: "30s"
    server_healthiness_timeout: "20s"
    retry_attempts: 3
    keepalive:
      time: "30s"
      timeout: "10s"
    insecure: false
  hyperfleet_api:
    base_url: "http://hyperfleet-api:8000"
    version: "v1"
    timeout: "10s"
    retry_attempts: 3
    retry_backoff: "exponential"
    base_delay: "1s"
    max_delay: "30s"
    default_headers:
      X-Example: "value"
    auth:
      token_path: "/var/run/secrets/hyperfleet/token"
      token_cache_ttl: "30s"
  broker:
    subscription_id: "example-subscription"
    topic: "example-topic"
  kubernetes:
    api_version: "v1"
    kube_config_path: "/path/to/kubeconfig"
    qps: 100
    burst: 200
```

### Top-level fields

- `adapter.name` (string, required): Adapter name.
- `adapter.version` (string, optional): when set, the binary validates it matches the running version. Only major and minor versions are compared — patch differences are allowed (e.g., config `1.2.0` with binary `1.2.3` is valid). Non-semver versions (e.g., `dev`, `latest`, custom tags) skip validation gracefully.
- `debug_config` (bool, optional): Log the merged config after load. Default: `false`.

### Logging (`log`)

- `log.level` (string, optional): Log level (`debug`, `info`, `warn`, `error`). Default: `info`.
- `log.format` (string, optional): Log format (`text`, `json`). Default: `json`.
- `log.output` (string, optional): Log output destination (`stdout`, `stderr`). Default: `stdout`.

### Maestro client (`clients.maestro`)

- `grpc_server_address` (string): Maestro gRPC endpoint.
- `http_server_address` (string): Maestro HTTP API endpoint.
- `source_id` (string): CloudEvents source identifier.
- `client_id` (string): Maestro client identifier.
- `auth.type` (string): Authentication type (`tls` or `none`).
- `auth.tls_config.ca_file` (string): gRPC CA certificate path.
- `auth.tls_config.cert_file` (string): gRPC client certificate path.
- `auth.tls_config.key_file` (string): gRPC client key path.
- `auth.tls_config.http_ca_file` (string, optional): CA certificate for the HTTP API. Falls back to `ca_file` if unset.
- `timeout` (duration string): Request timeout (e.g. `30s`).
- `server_healthiness_timeout` (duration string, optional): Timeout for the server healthiness check (e.g. `20s`).
- `retry_attempts` (int): Number of retry attempts.
- `keepalive.time` (duration string): gRPC keepalive ping interval.
- `keepalive.timeout` (duration string): gRPC keepalive ping timeout.
- `insecure` (bool): Allow insecure connection.

### HyperFleet API client (`clients.hyperfleet_api`)

- `base_url` (string): Base URL for HyperFleet API requests.
- `version` (string): API version. Default: `v1`.
- `timeout` (duration string): HTTP client timeout. Default: `10s`.
- `retry_attempts` (int): Retry attempts. Default: `3`.
- `retry_backoff` (string): Backoff strategy (`exponential`, `linear`, `constant`). Default: `exponential`.
- `base_delay` (duration string): Initial retry delay. Default: `1s`.
- `max_delay` (duration string): Maximum retry delay. Default: `30s`.
- `default_headers` (map[string]string): Headers added to all API requests.
- `auth.token_path` (string): Absolute path to a file containing a JWT bearer token. When set, the token is read from this file and attached as `Authorization: Bearer <token>` on every request. Typically a Kubernetes projected ServiceAccount token. Must be an absolute path.
- `auth.token_cache_ttl` (duration string): How long the token is cached in memory before re-reading the file. Zero (default) means re-read on every request.

### Broker (`clients.broker`)

These fields appear in the **adapter deployment config** and control which events the adapter consumes. The actual broker connection details (URL, credentials, exchange) live in a separate `broker.yaml` file managed by the Helm chart.

- `subscription_id` (string, required): A unique identifier for this adapter instance's subscription. **Must be unique across adapter instances** that should each receive all events independently (fan-out). Two adapters with the same `subscription_id` and same queue name will share a queue and compete for messages — each event goes to only one of them.
- `topic` (string, required): For RabbitMQ, this is the AMQP queue name prefix (not a routing key — see below). Set it to a meaningful value that identifies this adapter's event stream (e.g. `hyperfleet-clusters`). For Google Pub/Sub this is the Pub/Sub topic name.

Set these values directly in the adapter config YAML. The env var overrides (`HYPERFLEET_BROKER_SUBSCRIPTION_ID`, `HYPERFLEET_BROKER_TOPIC`) exist as an escape hatch but are not required — values in the YAML take effect without them.

### Broker connection config (`broker.yaml`)

The broker connection is configured separately, via a mounted `broker.yaml` (or the Helm `broker.*` values). This file is read by the hyperfleet-broker library directly and **does not support Viper/env var overrides** — it is pure YAML.

#### Google Pub/Sub

```yaml
broker:
  type: googlepubsub
  googlepubsub:
    project_id: "my-gcp-project"
    topic: "cluster-events"
    subscription_id: "my-adapter-sub"
    dead_letter_topic: ""            # optional
    create_topic_if_missing: false
    create_subscription_if_missing: false
```

#### RabbitMQ

```yaml
broker:
  type: rabbitmq
  rabbitmq:
    url: "amqp://user:pass@rabbitmq:5672/"   # required; amqp:// or amqps://
    exchange: "hyperfleet-clusters"           # required; must match sentinel's clients.broker.topic
    exchange_type: "topic"                    # optional; default: topic
    queue: "my-adapter"                       # optional; see queue naming below
    prefetch_count: 0                         # optional; 0 = broker default
    prefetch_size: 0                          # optional; 0 = no limit
    consumer_tag: ""                          # optional; auto-assigned if empty
    publisher_confirm: false                  # optional; enable publisher confirms
```

**Connecting to the sentinel.** The sentinel publishes events to a RabbitMQ exchange whose name equals the sentinel's `clients.broker.topic` (e.g. `hyperfleet-clusters`). The adapter's `broker.yaml` `exchange` field must match this value exactly — this is the only coupling point between sentinel and adapter.

**Queue naming in RabbitMQ.** The adapter binds a queue to the exchange and consumes from it. The queue name is derived as:

- If `queue` is set: `{queue}-{subscription_id}` (e.g. `my-adapter-adapter-1`)
- If `queue` is omitted: `{topic}-{subscription_id}` (e.g. `hyperfleet-clusters-adapter-1`)

The queue-to-exchange binding always uses an **empty routing key** — all queues bound to the same exchange receive all messages published to it.

**Fan-out vs competing consumers.** Because queue name is derived from both `topic` and `subscription_id`, two adapters sharing the same values for both fields will land on the same queue and compete (each event goes to one adapter). Give each adapter instance a unique `subscription_id` in its adapter config YAML to ensure each gets an independent copy of every event.

`url` and `exchange` are required. `queue` is optional.

### Kubernetes (`clients.kubernetes`)

- `api_version` (string): Kubernetes API version.
- `kube_config_path` (string): Path to kubeconfig (empty uses in-cluster auth).
- `qps` (float): Client-side QPS limit (0 uses defaults).
- `burst` (int): Client-side burst limit (0 uses defaults).

### Tracing (OpenTelemetry)

Tracing is configured entirely through environment variables — there is no YAML section.

- `HYPERFLEET_TRACING_ENABLED` (bool): Enable or disable tracing. Default: `true` in the binary, `false` in the Helm chart. Set to `false` to suppress OTLP export errors when no collector is available.

When tracing is enabled, the adapter uses standard [OpenTelemetry environment variables](https://opentelemetry.io/docs/specs/otel/configuration/sdk-environment-variables/):

| Variable | Description | Default |
|----------|-------------|---------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector endpoint (e.g., `http://otel-collector:4317`) | — (stdout exporter) |
| `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` | Signal-specific endpoint; overrides the above for traces only | — |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | Protocol: `grpc` or `http/protobuf` | `grpc` |
| `OTEL_EXPORTER_OTLP_TRACES_PROTOCOL` | Signal-specific protocol override | — |
| `OTEL_SERVICE_NAME` | Service name reported in spans | `adapter.name` from config |
| `OTEL_TRACES_SAMPLER` | Sampler type (`always_on`, `always_off`, `traceidratio`, `parentbased_*`) | `parentbased_traceidratio` |
| `OTEL_TRACES_SAMPLER_ARG` | Sampling ratio (0.0–1.0) | `1.0` |

When no `OTEL_EXPORTER_OTLP_ENDPOINT` is set, traces are written to stdout for local development.

The Helm chart exposes `tracing.enabled`, `tracing.otlpEndpoint`, `tracing.otlpProtocol`, `tracing.serviceName`, `tracing.sampler`, `tracing.samplerArg`, and `tracing.propagators` in `values.yaml` which map to these environment variables. For Helm deployment details, see the [Deployment Guide — Tracing](deployment.md#tracing).

## Command-line parameters

The following CLI flags override YAML values:

**General**

- `--debug-config` -> `debug_config`
- `--log-level` -> `log.level`
- `--log-format` -> `log.format`
- `--log-output` -> `log.output`

**Maestro**

- `--maestro-grpc-server-address` -> `clients.maestro.grpc_server_address`
- `--maestro-http-server-address` -> `clients.maestro.http_server_address`
- `--maestro-source-id` -> `clients.maestro.source_id`
- `--maestro-client-id` -> `clients.maestro.client_id`
- `--maestro-auth-type` -> `clients.maestro.auth.type`
- `--maestro-ca-file` -> `clients.maestro.auth.tls_config.ca_file`
- `--maestro-cert-file` -> `clients.maestro.auth.tls_config.cert_file`
- `--maestro-key-file` -> `clients.maestro.auth.tls_config.key_file`
- `--maestro-http-ca-file` -> `clients.maestro.auth.tls_config.http_ca_file`
- `--maestro-timeout` -> `clients.maestro.timeout`
- `--maestro-server-healthiness-timeout` -> `clients.maestro.server_healthiness_timeout`
- `--maestro-retry-attempts` -> `clients.maestro.retry_attempts`
- `--maestro-keepalive-time` -> `clients.maestro.keepalive.time`
- `--maestro-keepalive-timeout` -> `clients.maestro.keepalive.timeout`
- `--maestro-insecure` -> `clients.maestro.insecure`

**HyperFleet API**

- `--hyperfleet-api-base-url` -> `clients.hyperfleet_api.base_url`
- `--hyperfleet-api-version` -> `clients.hyperfleet_api.version`
- `--hyperfleet-api-timeout` -> `clients.hyperfleet_api.timeout`
- `--hyperfleet-api-retry` -> `clients.hyperfleet_api.retry_attempts`
- `--hyperfleet-api-retry-backoff` -> `clients.hyperfleet_api.retry_backoff`
- `--hyperfleet-api-base-delay` -> `clients.hyperfleet_api.base_delay`
- `--hyperfleet-api-max-delay` -> `clients.hyperfleet_api.max_delay`

**Broker**

- `--broker-subscription-id` -> `clients.broker.subscription_id`
- `--broker-topic` -> `clients.broker.topic`

**Kubernetes**

- `--kubernetes-api-version` -> `clients.kubernetes.api_version`
- `--kubernetes-kube-config-path` -> `clients.kubernetes.kube_config_path`
- `--kubernetes-qps` -> `clients.kubernetes.qps`
- `--kubernetes-burst` -> `clients.kubernetes.burst`

## Environment variables

All deployment overrides use the `HYPERFLEET_` prefix unless noted.

**General**

- `HYPERFLEET_DEBUG_CONFIG` -> `debug_config`
- `LOG_LEVEL` -> `log.level`
- `LOG_FORMAT` -> `log.format`
- `LOG_OUTPUT` -> `log.output`

**Maestro**

- `HYPERFLEET_MAESTRO_GRPC_SERVER_ADDRESS` -> `clients.maestro.grpc_server_address`
- `HYPERFLEET_MAESTRO_HTTP_SERVER_ADDRESS` -> `clients.maestro.http_server_address`
- `HYPERFLEET_MAESTRO_SOURCE_ID` -> `clients.maestro.source_id`
- `HYPERFLEET_MAESTRO_CLIENT_ID` -> `clients.maestro.client_id`
- `HYPERFLEET_MAESTRO_AUTH_TYPE` -> `clients.maestro.auth.type`
- `HYPERFLEET_MAESTRO_CA_FILE` -> `clients.maestro.auth.tls_config.ca_file`
- `HYPERFLEET_MAESTRO_CERT_FILE` -> `clients.maestro.auth.tls_config.cert_file`
- `HYPERFLEET_MAESTRO_KEY_FILE` -> `clients.maestro.auth.tls_config.key_file`
- `HYPERFLEET_MAESTRO_HTTP_CA_FILE` -> `clients.maestro.auth.tls_config.http_ca_file`
- `HYPERFLEET_MAESTRO_TIMEOUT` -> `clients.maestro.timeout`
- `HYPERFLEET_MAESTRO_SERVER_HEALTHINESS_TIMEOUT` -> `clients.maestro.server_healthiness_timeout`
- `HYPERFLEET_MAESTRO_RETRY_ATTEMPTS` -> `clients.maestro.retry_attempts`
- `HYPERFLEET_MAESTRO_KEEPALIVE_TIME` -> `clients.maestro.keepalive.time`
- `HYPERFLEET_MAESTRO_KEEPALIVE_TIMEOUT` -> `clients.maestro.keepalive.timeout`
- `HYPERFLEET_MAESTRO_INSECURE` -> `clients.maestro.insecure`

**HyperFleet API**

- `HYPERFLEET_API_BASE_URL` -> `clients.hyperfleet_api.base_url`
- `HYPERFLEET_API_VERSION` -> `clients.hyperfleet_api.version`
- `HYPERFLEET_API_TIMEOUT` -> `clients.hyperfleet_api.timeout`
- `HYPERFLEET_API_RETRY_ATTEMPTS` -> `clients.hyperfleet_api.retry_attempts`
- `HYPERFLEET_API_RETRY_BACKOFF` -> `clients.hyperfleet_api.retry_backoff`
- `HYPERFLEET_API_BASE_DELAY` -> `clients.hyperfleet_api.base_delay`
- `HYPERFLEET_API_MAX_DELAY` -> `clients.hyperfleet_api.max_delay`
- `HYPERFLEET_API_AUTH_TOKEN_PATH` -> `clients.hyperfleet_api.auth.token_path`
- `HYPERFLEET_API_AUTH_TOKEN_CACHE_TTL` -> `clients.hyperfleet_api.auth.token_cache_ttl`

**Broker**

- `HYPERFLEET_BROKER_SUBSCRIPTION_ID` -> `clients.broker.subscription_id`
- `HYPERFLEET_BROKER_TOPIC` -> `clients.broker.topic`

**Kubernetes**

- `HYPERFLEET_KUBERNETES_API_VERSION` -> `clients.kubernetes.api_version`
- `HYPERFLEET_KUBERNETES_KUBE_CONFIG_PATH` -> `clients.kubernetes.kube_config_path`
- `HYPERFLEET_KUBERNETES_QPS` -> `clients.kubernetes.qps`
- `HYPERFLEET_KUBERNETES_BURST` -> `clients.kubernetes.burst`

Legacy broker environment variables (used only if the prefixed version is unset):

- `BROKER_SUBSCRIPTION_ID` -> `clients.broker.subscription_id`
- `BROKER_TOPIC` -> `clients.broker.topic`
