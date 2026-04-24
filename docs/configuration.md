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
- `adapter.version` (string, optional): when set, the binary validates it matches the running version.
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

### Broker (`clients.broker`)

- `subscription_id` (string): Broker subscription ID (required at runtime).
- `topic` (string): Broker topic (required at runtime).

### Kubernetes (`clients.kubernetes`)

- `api_version` (string): Kubernetes API version.
- `kube_config_path` (string): Path to kubeconfig (empty uses in-cluster auth).
- `qps` (float): Client-side QPS limit (0 uses defaults).
- `burst` (int): Client-side burst limit (0 uses defaults).

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
