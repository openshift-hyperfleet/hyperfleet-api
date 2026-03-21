# HyperFleet API Configuration Guide

Complete reference for configuring HyperFleet API following the [HyperFleet Configuration Standard](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/standards/configuration.md).

---

## Quick Start

**Development:**
```bash
# Create config.yaml with database settings, then:
hyperfleet-api serve --config=config.yaml
```

**Production (Kubernetes):**
```bash
helm install hyperfleet-api ./charts/ \
  --set 'config.adapters.required.cluster={validation,dns}' \
  --set 'config.adapters.required.nodepool={validation}'
```

See [Configuration Examples](#configuration-examples) for complete setup.

---

## Configuration Methods

HyperFleet API supports multiple configuration sources:

| Method | Use Case | Example |
|--------|----------|---------|
| **Configuration File** | Local development, complex configs | `config.yaml` with all settings |
| **Environment Variables** | Kubernetes (secretKeyRef), CI/CD | `HYPERFLEET_DATABASE_HOST=localhost` |
| **CLI Flags** | Quick overrides, testing | `--server-port=9000` |

All configuration follows these conventions:
- **Environment variables**: `HYPERFLEET_*` prefix, uppercase, underscores (e.g., `HYPERFLEET_SERVER_PORT`)
- **CLI flags**: `--kebab-case`, lowercase, hyphens (e.g., `--server-port`)
- **YAML properties**: `snake_case`, lowercase, underscores (e.g., `server.port`)

---

## Configuration Priority

Configuration sources are applied in the following order (highest to lowest priority):

```text
1. Command-line flags (highest)
   ↓
2. Environment variables (e.g., HYPERFLEET_DATABASE_PASSWORD)
   ↓
3. Configuration file (config.yaml or ConfigMap)
   ↓
4. Default values (lowest)
```

**Examples**:

*Flag overrides environment variable:*
```bash
export HYPERFLEET_SERVER_PORT=8000
hyperfleet-api serve --server-port=9000
# Result: Uses 9000 (flag wins)
```

*Environment variable overrides config file:*
```bash
# config.yaml has: database.password: "config-password"
export HYPERFLEET_DATABASE_PASSWORD=secret-password
# Result: Uses "secret-password" (env var wins)
```

**Special Case - OpenTelemetry Tracing:**

`TRACING_ENABLED` (Tracing standard) has special precedence for cross-component consistency:

```text
TRACING_ENABLED > config (env/flags) > default
```

See [OpenTelemetry Configuration](#opentelemetry-configuration) for details.

---

## Configuration File Locations

The configuration file is resolved in the following order:

1. **`--config` flag** - Explicit path provided via CLI
   ```bash
   hyperfleet-api serve --config=/path/to/config.yaml
   ```

2. **`HYPERFLEET_CONFIG` environment variable** - Path in environment
   ```bash
   export HYPERFLEET_CONFIG=/path/to/config.yaml
   ```

3. **Default paths** - Automatic discovery
   - Production: `/etc/hyperfleet/config.yaml`
   - Development: `./configs/config.yaml`

If no configuration file is found, the application continues using environment variables, CLI flags, and defaults.

---

## Core Configuration

These settings are required or commonly used by most deployments.

### Database Configuration

PostgreSQL database connection settings.

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `database.dialect` | string | `postgres` | Database dialect |
| `database.host` | string | `localhost` | Database server hostname |
| `database.port` | int | `5432` | Database server port |
| `database.name` | string | `hyperfleet` | Database name |
| `database.username` | string | `hyperfleet` | Database username |
| `database.password` | string | `""` | Database password (**use env var with secretKeyRef for Kubernetes**) |
| `database.ssl.mode` | string | `disable` | SSL mode: `disable`, `require`, `verify-ca`, `verify-full` |
| `database.ssl.root_cert_file` | string | `""` | Root CA certificate for SSL verification |
| `database.pool.max_connections` | int | `50` | Maximum open database connections |
| `database.pool.max_idle_connections` | int | `10` | Maximum idle database connections |
| `database.pool.conn_max_lifetime` | duration | `5m` | Maximum connection lifetime |
| `database.pool.conn_max_idle_time` | duration | `1m` | Maximum connection idle time |
| `database.pool.request_timeout` | duration | `30s` | Database request timeout |
| `database.pool.conn_retry_attempts` | int | `10` | Connection retry attempts on startup (for pgbouncer/sidecar startup races) |
| `database.pool.conn_retry_interval` | duration | `3s` | Interval between connection retry attempts |
| `database.debug` | bool | `false` | Enable SQL query logging |

**Example:**
```yaml
database:
  host: postgres.example.com
  port: 5432
  name: hyperfleet
  username: hyperfleet
  # Password via environment variable (recommended for Kubernetes: secretKeyRef)
  ssl:
    mode: verify-full
    root_cert_file: /etc/certs/ca.crt
  pool:
    max_connections: 100
    max_idle_connections: 20
    conn_max_lifetime: 10m
    conn_max_idle_time: 2m
    request_timeout: 60s
    conn_retry_attempts: 15
    conn_retry_interval: 5s
```

### Adapters Configuration

Specifies which adapters must be ready for resources to be marked as "Ready". Should be configured for production deployments.

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `adapters.required.cluster` | []string | `[]` | Cluster adapters required for Ready state (e.g., `["validation","dns"]`) |
| `adapters.required.nodepool` | []string | `[]` | Nodepool adapters required for Ready state (e.g., `["validation"]`) |

**Example:**
```yaml
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
```

**Environment variable (JSON array format):**
```bash
export HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER='["validation","dns","pullsecret","hypershift"]'
export HYPERFLEET_ADAPTERS_REQUIRED_NODEPOOL='["validation","hypershift"]'
```

### Logging Configuration

Logging behavior and output settings.

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `logging.level` | string | `info` | Log level: `debug`, `info`, `warn`, `error` |
| `logging.format` | string | `json` | Log format: `json`, `text` |
| `logging.output` | string | `stdout` | Log output: `stdout`, `stderr` |
| `logging.otel.enabled` | bool | `false` | Enable OpenTelemetry tracing (see [OpenTelemetry Configuration](#opentelemetry-configuration)) |
| `logging.masking.enabled` | bool | `true` | Enable sensitive data masking in logs |

**Example:**
```yaml
logging:
  level: info
  format: json
  output: stdout
  masking:
    enabled: true
    headers:
      - Authorization
      - Cookie
    fields:
      - password
      - token
```

### OpenTelemetry Configuration

OpenTelemetry tracing is configured using standard OpenTelemetry environment variables.

**Enabling Tracing:**

HyperFleet supports multiple ways to enable tracing (in order of precedence):

1. **`TRACING_ENABLED`** (Tracing standard - recommended for production)
   ```bash
   export TRACING_ENABLED=true
   ```

2. **`HYPERFLEET_LOGGING_OTEL_ENABLED`** (Config via environment variable)
   ```bash
   export HYPERFLEET_LOGGING_OTEL_ENABLED=true
   ```

3. **Config file** (`config.yaml`)
   ```yaml
   logging:
     otel:
       enabled: true
   ```

**OpenTelemetry Standard Environment Variables:**

Once tracing is enabled, configure OpenTelemetry behavior using standard variables:

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `OTEL_SERVICE_NAME` | Service name in traces | `hyperfleet-api` | `hyperfleet-api-prod` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector endpoint | stdout | `http://otel-collector:4317` |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | Export protocol | `grpc` | `grpc`, `http/protobuf` |
| `OTEL_TRACES_SAMPLER` | Sampler type | `parentbased_traceidratio` | `always_on`, `traceidratio` |
| `OTEL_TRACES_SAMPLER_ARG` | Sampling rate (0.0-1.0) | `1.0` | `0.1` (10% sampling) |
| `OTEL_RESOURCE_ATTRIBUTES` | Additional attributes | - | `env=prod,region=us-east` |

**Example - Production with OTLP Collector:**
```bash
# Enable tracing (Tracing standard)
export TRACING_ENABLED=true

# Configure OpenTelemetry
export OTEL_SERVICE_NAME=hyperfleet-api-prod
export OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc
export OTEL_TRACES_SAMPLER=parentbased_traceidratio
export OTEL_TRACES_SAMPLER_ARG=0.1  # Sample 10% of requests
export OTEL_RESOURCE_ATTRIBUTES=environment=production,region=us-east-1
```
**Note**: Sample 10% of requests (OTEL_TRACES_SAMPLER_ARG=0.1) provides sufficient observability while minimizing performance impact for high-traffic systems

**Example - Development with stdout:**
```bash
# Enable tracing
export TRACING_ENABLED=true

# Sample all requests (no OTLP endpoint = stdout export)
export OTEL_TRACES_SAMPLER_ARG=1.0
```

**Sampler Types:**
- `always_on` - Sample all requests
- `always_off` - Sample no requests
- `traceidratio` - Sample based on trace ID ratio (use with `OTEL_TRACES_SAMPLER_ARG`)
- `parentbased_traceidratio` - Respect parent decision, otherwise use trace ID ratio (default)
- `parentbased_always_on` - Respect parent decision, otherwise sample all
- `parentbased_always_off` - Respect parent decision, otherwise sample none

**See also:** [Logging Documentation](logging.md#opentelemetry-integration) for detailed tracing setup.

---

## Advanced Configuration

<details>
<summary><b>Server Configuration</b> (click to expand)</summary>

HTTP server settings for the API endpoint.

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `server.hostname` | string | `""` | Public hostname for logging (optional) |
| `server.host` | string | `localhost` | Server bind host (`0.0.0.0` for Kubernetes) |
| `server.port` | int | `8000` | Server bind port |
| `server.timeouts.read` | duration | `5s` | HTTP read timeout |
| `server.timeouts.write` | duration | `30s` | HTTP write timeout |
| `server.tls.enabled` | bool | `false` | Enable HTTPS/TLS |
| `server.tls.cert_file` | string | `""` | Path to TLS certificate file |
| `server.tls.key_file` | string | `""` | Path to TLS key file |
| `server.jwt.enabled` | bool | `true` | Enable JWT authentication |
| `server.authz.enabled` | bool | `false` | Enable authorization checks |
| `server.jwk.cert_file` | string | `""` | JWK certificate file path (optional) |
| `server.jwk.cert_url` | string | `https://sso.redhat.com/...` | JWK certificate URL |
| `server.acl.file` | string | `""` | Access control list (ACL) file path |

**Example:**
```yaml
server:
  hostname: api.example.com
  host: 0.0.0.0
  port: 8000
  tls:
    enabled: true
    cert_file: /etc/certs/tls.crt
    key_file: /etc/certs/tls.key
  jwt:
    enabled: true
  jwk:
    cert_url: https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/certs
```

</details>

<details>
<summary><b>Metrics Configuration</b> (click to expand)</summary>

Prometheus metrics endpoint settings.

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `metrics.host` | string | `localhost` | Metrics bind host (`0.0.0.0` for Kubernetes) |
| `metrics.port` | int | `9090` | Metrics port |
| `metrics.tls.enabled` | bool | `false` | Enable TLS for metrics endpoint |
| `metrics.label_metrics_inclusion_duration` | duration | `168h` | Duration to include label metrics (7 days) |

**Example:**
```yaml
metrics:
  host: 0.0.0.0  # Required for Kubernetes Service access
  port: 9090
  tls:
    enabled: false
```

</details>

<details>
<summary><b>Health Configuration</b> (click to expand)</summary>

Health check endpoint settings.

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `health.host` | string | `localhost` | Health bind host (`0.0.0.0` for Kubernetes) |
| `health.port` | int | `8080` | Health port |
| `health.tls.enabled` | bool | `false` | Enable TLS for health endpoint |
| `health.shutdown_timeout` | duration | `20s` | Graceful shutdown timeout |
| `health.db_ping_timeout` | duration | `2s` | Database ping timeout for readiness check |

**Example:**
```yaml
health:
  host: 0.0.0.0  # Required for Kubernetes probes
  port: 8080
  shutdown_timeout: 30s
  db_ping_timeout: 2s
```

</details>

<details>
<summary><b>OCM Configuration</b> (click to expand)</summary>

OpenShift Cluster Manager integration settings.

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `ocm.base_url` | string | `https://api.integration.openshift.com` | OCM API base URL |
| `ocm.client_id` | string | `""` | OCM client ID (use env var with secretKeyRef for Kubernetes) |
| `ocm.client_secret` | string | `""` | OCM client secret (use env var with secretKeyRef for Kubernetes) |
| `ocm.self_token` | string | `""` | OCM self token (use env var with secretKeyRef for Kubernetes) |
| `ocm.mock.enabled` | bool | `false` | Enable mock OCM client for testing |
| `ocm.insecure` | bool | `false` | Skip TLS verification (development only) |

**Example:**
```yaml
ocm:
  base_url: https://api.openshift.com
  # Credentials via environment variables (recommended for Kubernetes: secretKeyRef)
  mock:
    enabled: false
```

</details>

---

## Complete Reference

### All Configuration Properties

Complete table of all configuration properties, their environment variables, and types.

| Config Path | Environment Variable | Type | Default |
|-------------|---------------------|------|---------|
| **Server** | | | |
| `server.hostname` | `HYPERFLEET_SERVER_HOSTNAME` | string | `""` |
| `server.host` | `HYPERFLEET_SERVER_HOST` | string | `localhost` |
| `server.port` | `HYPERFLEET_SERVER_PORT` | int | `8000` |
| `server.timeouts.read` | `HYPERFLEET_SERVER_TIMEOUTS_READ` | duration | `5s` |
| `server.timeouts.write` | `HYPERFLEET_SERVER_TIMEOUTS_WRITE` | duration | `30s` |
| `server.tls.enabled` | `HYPERFLEET_SERVER_TLS_ENABLED` | bool | `false` |
| `server.tls.cert_file` | `HYPERFLEET_SERVER_TLS_CERT_FILE` | string | `""` |
| `server.tls.key_file` | `HYPERFLEET_SERVER_TLS_KEY_FILE` | string | `""` |
| `server.jwt.enabled` | `HYPERFLEET_SERVER_JWT_ENABLED` | bool | `true` |
| `server.authz.enabled` | `HYPERFLEET_SERVER_AUTHZ_ENABLED` | bool | `false` |
| `server.jwk.cert_file` | `HYPERFLEET_SERVER_JWK_CERT_FILE` | string | `""` |
| `server.jwk.cert_url` | `HYPERFLEET_SERVER_JWK_CERT_URL` | string | `https://sso.redhat.com/...` |
| `server.acl.file` | `HYPERFLEET_SERVER_ACL_FILE` | string | `""` |
| **Database** | | | |
| `database.dialect` | `HYPERFLEET_DATABASE_DIALECT` | string | `postgres` |
| `database.host` | `HYPERFLEET_DATABASE_HOST` | string | `localhost` |
| `database.port` | `HYPERFLEET_DATABASE_PORT` | int | `5432` |
| `database.name` | `HYPERFLEET_DATABASE_NAME` | string | `hyperfleet` |
| `database.username` | `HYPERFLEET_DATABASE_USERNAME` | string | `hyperfleet` |
| `database.password` | `HYPERFLEET_DATABASE_PASSWORD` | string | `""` |
| `database.debug` | `HYPERFLEET_DATABASE_DEBUG` | bool | `false` |
| `database.ssl.mode` | `HYPERFLEET_DATABASE_SSL_MODE` | string | `disable` |
| `database.ssl.root_cert_file` | `HYPERFLEET_DATABASE_SSL_ROOT_CERT_FILE` | string | `""` |
| `database.pool.max_connections` | `HYPERFLEET_DATABASE_POOL_MAX_CONNECTIONS` | int | `50` |
| `database.pool.max_idle_connections` | `HYPERFLEET_DATABASE_POOL_MAX_IDLE_CONNECTIONS` | int | `10` |
| `database.pool.conn_max_lifetime` | `HYPERFLEET_DATABASE_POOL_CONN_MAX_LIFETIME` | duration | `5m` |
| `database.pool.conn_max_idle_time` | `HYPERFLEET_DATABASE_POOL_CONN_MAX_IDLE_TIME` | duration | `1m` |
| `database.pool.request_timeout` | `HYPERFLEET_DATABASE_POOL_REQUEST_TIMEOUT` | duration | `30s` |
| `database.pool.conn_retry_attempts` | `HYPERFLEET_DATABASE_POOL_CONN_RETRY_ATTEMPTS` | int | `10` |
| `database.pool.conn_retry_interval` | `HYPERFLEET_DATABASE_POOL_CONN_RETRY_INTERVAL` | duration | `3s` |
| **Logging** | | | |
| `logging.level` | `HYPERFLEET_LOGGING_LEVEL` | string | `info` |
| `logging.format` | `HYPERFLEET_LOGGING_FORMAT` | string | `json` |
| `logging.output` | `HYPERFLEET_LOGGING_OUTPUT` | string | `stdout` |
| `logging.otel.enabled` | `HYPERFLEET_LOGGING_OTEL_ENABLED` | bool | `false` |
| `logging.masking.enabled` | `HYPERFLEET_LOGGING_MASKING_ENABLED` | bool | `true` |
| `logging.masking.headers` | `HYPERFLEET_LOGGING_MASKING_HEADERS` | csv | `Authorization,Cookie` |
| `logging.masking.fields` | `HYPERFLEET_LOGGING_MASKING_FIELDS` | csv | `password,token` |
| **OCM** | | | |
| `ocm.base_url` | `HYPERFLEET_OCM_BASE_URL` | string | `https://api.integration.openshift.com` |
| `ocm.client_id` | `HYPERFLEET_OCM_CLIENT_ID` | string | `""` |
| `ocm.client_secret` | `HYPERFLEET_OCM_CLIENT_SECRET` | string | `""` |
| `ocm.self_token` | `HYPERFLEET_OCM_SELF_TOKEN` | string | `""` |
| `ocm.mock.enabled` | `HYPERFLEET_OCM_MOCK_ENABLED` | bool | `false` |
| `ocm.insecure` | `HYPERFLEET_OCM_INSECURE` | bool | `false` |
| **Metrics** | | | |
| `metrics.host` | `HYPERFLEET_METRICS_HOST` | string | `localhost` |
| `metrics.port` | `HYPERFLEET_METRICS_PORT` | int | `9090` |
| `metrics.tls.enabled` | `HYPERFLEET_METRICS_TLS_ENABLED` | bool | `false` |
| `metrics.label_metrics_inclusion_duration` | `HYPERFLEET_METRICS_LABEL_METRICS_INCLUSION_DURATION` | duration | `168h` |
| **Health** | | | |
| `health.host` | `HYPERFLEET_HEALTH_HOST` | string | `localhost` |
| `health.port` | `HYPERFLEET_HEALTH_PORT` | int | `8080` |
| `health.tls.enabled` | `HYPERFLEET_HEALTH_TLS_ENABLED` | bool | `false` |
| `health.shutdown_timeout` | `HYPERFLEET_HEALTH_SHUTDOWN_TIMEOUT` | duration | `20s` |
| `health.db_ping_timeout` | `HYPERFLEET_HEALTH_DB_PING_TIMEOUT` | duration | `2s` |
| **Adapters** | | | |
| `adapters.required.cluster` | `HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER` | json | `[]` |
| `adapters.required.nodepool` | `HYPERFLEET_ADAPTERS_REQUIRED_NODEPOOL` | json | `[]` |

### CLI Flags Reference

All CLI flags and their corresponding configuration paths.

| CLI Flag | Config Path | Type |
|----------|-------------|------|
| `--config` | N/A (config file path) | string |
| **Server** | | |
| `--server-hostname` | `server.hostname` | string |
| `--server-host` | `server.host` | string |
| `--server-port` | `server.port` | int |
| `--server-read-timeout` | `server.timeouts.read` | duration |
| `--server-write-timeout` | `server.timeouts.write` | duration |
| `--server-https-enabled` | `server.tls.enabled` | bool |
| `--server-https-cert-file` | `server.tls.cert_file` | string |
| `--server-https-key-file` | `server.tls.key_file` | string |
| `--server-jwt-enabled` | `server.jwt.enabled` | bool |
| `--server-authz-enabled` | `server.authz.enabled` | bool |
| `--server-jwk-cert-file` | `server.jwk.cert_file` | string |
| `--server-jwk-cert-url` | `server.jwk.cert_url` | string |
| `--server-acl-file` | `server.acl.file` | string |
| **Database** | | |
| `--db-dialect` | `database.dialect` | string |
| `--db-host` | `database.host` | string |
| `--db-port` | `database.port` | int |
| `--db-name` | `database.name` | string |
| `--db-username` | `database.username` | string |
| `--db-password` | `database.password` | string |
| `--db-debug` | `database.debug` | bool |
| `--db-max-open-connections` | `database.pool.max_connections` | int |
| `--db-root-cert-file` | `database.ssl.root_cert_file` | string |
| **Logging** | | |
| `--log-level`, `-l` | `logging.level` | string |
| `--log-format` | `logging.format` | string |
| `--log-output` | `logging.output` | string |
| `--log-otel-enabled` | `logging.otel.enabled` | bool |
| **OCM** | | |
| `--ocm-base-url` | `ocm.base_url` | string |
| `--ocm-client-id` | `ocm.client_id` | string |
| `--ocm-client-secret` | `ocm.client_secret` | string |
| `--ocm-self-token` | `ocm.self_token` | string |
| `--ocm-mock` | `ocm.mock.enabled` | bool |
| `--ocm-insecure` | `ocm.insecure` | bool |
| **Metrics** | | |
| `--metrics-host` | `metrics.host` | string |
| `--metrics-port` | `metrics.port` | int |
| **Health** | | |
| `--health-host` | `health.host` | string |
| `--health-port` | `health.port` | int |

---

## Configuration Examples

**Complete configuration file**: See [configs/config.yaml.example](../configs/config.yaml.example) for all options with inline comments.

**Deployment**: See [Deployment Guide](deployment.md) for Kubernetes/Helm setup.

---

### Development

Minimal config for local development:
```yaml
database:
  password: devpassword

adapters:
  required:
    cluster: []
    nodepool: []
```

### Enable TLS

```yaml
server:
  tls:
    enabled: true
    cert_file: /etc/certs/tls.crt
    key_file: /etc/certs/tls.key
```

### Testing with Mock OCM

```yaml
server:
  jwt:
    enabled: false

ocm:
  mock:
    enabled: true
```

---

## Configuration Validation

The application performs comprehensive validation at startup.

### Validation Rules

**Server**:
- `server.port`: 1-65535
- `server.timeouts.read`: ≥ 1s
- `server.timeouts.write`: ≥ 1s

**Database**:
- `database.host`: required
- `database.port`: 1-65535
- `database.name`: required
- `database.username`: required
- `database.password`: required
- `database.ssl.mode`: must be `disable`, `require`, `verify-ca`, or `verify-full`

**Logging**:
- `logging.level`: must be `debug`, `info`, `warn`, or `error`
- `logging.format`: must be `json` or `text`

**Adapters**:
- `adapters.required.cluster`: must be array of strings
- `adapters.required.nodepool`: must be array of strings

### Validation Errors

If validation fails, the application will exit with a detailed error message:

```text
Error: Configuration validation failed:
- Server.Port must be between 1 and 65535 (got: 0)
- Database.Host is required
- Logging.Level must be one of: debug, info, warn, error (got: invalid)
```

---

## Troubleshooting

### Configuration not loading

**Check configuration file path:**
```bash
# Verify file exists
ls -l /etc/hyperfleet/config.yaml

# Check environment variable
echo $HYPERFLEET_CONFIG

# Use explicit path
hyperfleet-api serve --config=/path/to/config.yaml
```

### Environment variables not working

**Verify variable names:**
```bash
# Check all HYPERFLEET_* variables
env | grep HYPERFLEET_

# Correct format
export HYPERFLEET_SERVER_PORT=8000  # ✅

# Wrong format
export SERVER_PORT=8000  # ❌ Missing HYPERFLEET_ prefix
```

### Validation errors

**Common issues:**

1. **Invalid log level:**
   ```text
   Error: Logging.Level must be one of: debug, info, warn, error
   ```
   Solution: Use lowercase: `info`, not `INFO`

2. **Invalid port:**
   ```text
   Error: Server.Port must be between 1 and 65535
   ```
   Solution: Check port value in config file or environment variable

3. **Missing required field:**
   ```text
   Error: Database.Host is required
   ```
   Solution: Set via config file, environment variable, or CLI flag

### Debugging configuration

**Enable debug logging to see configuration loading:**
```bash
export HYPERFLEET_LOGGING_LEVEL=debug
hyperfleet-api serve
```

**Check effective configuration:**
```bash
# The application logs loaded configuration at startup (with secrets masked)
# Look for log messages like:
# {"level":"info","msg":"Configuration loaded successfully"}
```

---

## Additional Resources

- [HyperFleet Configuration Standard](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/standards/configuration.md)
- [Deployment Guide](deployment.md) - Kubernetes deployment with Helm
- [Development Guide](development.md) - Local development setup

---

## Configuration Checklist

Before deploying to production, verify:

**Core Configuration:**
- ✅ Database credentials set via `HYPERFLEET_DATABASE_*` environment variables
- ✅ Database SSL/TLS configured (`HYPERFLEET_DATABASE_SSL_MODE=require`)
- ✅ Server authentication enabled (`HYPERFLEET_SERVER_JWT_ENABLED=true`)
- ✅ Required adapters configured (`HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER`, `HYPERFLEET_ADAPTERS_REQUIRED_NODEPOOL`)

**OpenTelemetry Tracing (if enabled):**
- ✅ Tracing enabled via `TRACING_ENABLED=true` or `HYPERFLEET_LOGGING_OTEL_ENABLED=true`
- ✅ Service name set: `OTEL_SERVICE_NAME=hyperfleet-api-prod` (or environment-specific)
- ✅ OTLP endpoint configured: `OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317`
- ✅ Sampling rate appropriate for environment:
  - Production: `OTEL_TRACES_SAMPLER_ARG=0.1` (10% sampling recommended)
  - Development: `OTEL_TRACES_SAMPLER_ARG=1.0` (100% sampling)
- ✅ Resource attributes set: `OTEL_RESOURCE_ATTRIBUTES=environment=prod,region=us-east-1`
- ✅ Protocol matches collector: `OTEL_EXPORTER_OTLP_PROTOCOL=grpc` (or `http/protobuf`)

**Security:**
- ✅ Secrets stored in Kubernetes Secrets (not ConfigMaps)
- ✅ `secretKeyRef` used for sensitive values in Deployment
- ✅ Log masking enabled: `HYPERFLEET_LOGGING_MASKING_ENABLED=true`

**Logging:**
- ✅ Log level appropriate: `HYPERFLEET_LOGGING_LEVEL=info` (production)
- ✅ Log format: `HYPERFLEET_LOGGING_FORMAT=json` (for log aggregation)
- ✅ Database debug disabled in production: `HYPERFLEET_DATABASE_DEBUG=false`
