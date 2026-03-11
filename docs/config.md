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
  --set config.adapters.required.cluster={validation,dns} \
  --set config.adapters.required.nodepool={validation}
```

See [Configuration Examples](#configuration-examples) for complete setup.

---

## Configuration Methods

HyperFleet API supports multiple configuration sources:

| Method | Use Case | Example |
|--------|----------|---------|
| **Configuration File** | Local development, complex configs | `config.yaml` with all settings |
| **Environment Variables** | Kubernetes, CI/CD | `HYPERFLEET_DATABASE_HOST=localhost` |
| **File-based Secrets** | Kubernetes Secrets | `HYPERFLEET_DATABASE_PASSWORD_FILE=/secrets/db-password` |
| **CLI Flags** | Quick overrides, testing | `--server-port=9000` |

All configuration follows these conventions:
- **Environment variables**: `HYPERFLEET_*` prefix, uppercase, underscores (e.g., `HYPERFLEET_SERVER_PORT`)
- **CLI flags**: `--kebab-case`, lowercase, hyphens (e.g., `--server-port`)
- **YAML properties**: `snake_case`, lowercase, underscores (e.g., `server.port`)

---

## Configuration Priority

Configuration sources are applied in the following order (highest to lowest priority):

```
1. Command-line flags (highest)
   ↓
2. Environment variables
   ├─ Plain (e.g., HYPERFLEET_DATABASE_PASSWORD)
   └─ File-based (e.g., HYPERFLEET_DATABASE_PASSWORD_FILE)
      Note: Plain environment variables take precedence over file-based
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

*Plain environment variable overrides file-based:*
```bash
export HYPERFLEET_DATABASE_PASSWORD=direct-password
export HYPERFLEET_DATABASE_PASSWORD_FILE=/secrets/db-password
# Result: Uses "direct-password" (plain env wins)
# Use case: Quick override for testing without changing Secret
```

*File-based environment variable overrides config file:*
```bash
# config.yaml has: database.password: "config-password"
export HYPERFLEET_DATABASE_PASSWORD_FILE=/secrets/db-password
# Result: Uses content from /secrets/db-password (file env wins)
# Use case: Kubernetes Secret pattern
```

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
| `database.password` | string | `""` | Database password (**use `*_FILE` for production**) |
| `database.ssl.mode` | string | `disable` | SSL mode: `disable`, `require`, `verify-ca`, `verify-full` |
| `database.ssl.root_cert_file` | string | `""` | Root CA certificate for SSL verification |
| `database.pool.max_connections` | int | `50` | Maximum open database connections |
| `database.debug` | bool | `false` | Enable SQL query logging |

**Example:**
```yaml
database:
  host: postgres.example.com
  port: 5432
  name: hyperfleet
  username: hyperfleet
  # Password via environment variable or *_FILE (recommended)
  ssl:
    mode: verify-full
    root_cert_file: /etc/certs/ca.crt
  pool:
    max_connections: 100
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
| `logging.otel.enabled` | bool | `false` | Enable OpenTelemetry tracing |
| `logging.otel.sampling_rate` | float | `1.0` | OTEL sampling rate (0.0-1.0) |
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
| `server.timeouts.read` | duration | `15s` | HTTP read timeout |
| `server.timeouts.write` | duration | `15s` | HTTP write timeout |
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
| `metrics.label_metrics_inclusion_duration` | duration | `5m` | Duration to include label metrics |

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
| `health.shutdown_timeout` | duration | `30s` | Graceful shutdown timeout |

**Example:**
```yaml
health:
  host: 0.0.0.0  # Required for Kubernetes probes
  port: 8080
  shutdown_timeout: 30s
```

</details>

<details>
<summary><b>OCM Configuration</b> (click to expand)</summary>

OpenShift Cluster Manager integration settings.

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `ocm.base_url` | string | `https://api.integration.openshift.com` | OCM API base URL |
| `ocm.client_id` | string | `""` | OCM client ID (use `*_FILE` for production) |
| `ocm.client_secret` | string | `""` | OCM client secret (use `*_FILE` for production) |
| `ocm.self_token` | string | `""` | OCM self token (use `*_FILE` for production) |
| `ocm.mock.enabled` | bool | `false` | Enable mock OCM client for testing |
| `ocm.insecure` | bool | `false` | Skip TLS verification (development only) |

**Example:**
```yaml
ocm:
  base_url: https://api.openshift.com
  # Credentials via *_FILE environment variables
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
| `server.timeouts.read` | `HYPERFLEET_SERVER_TIMEOUTS_READ` | duration | `15s` |
| `server.timeouts.write` | `HYPERFLEET_SERVER_TIMEOUTS_WRITE` | duration | `15s` |
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
| **Logging** | | | |
| `logging.level` | `HYPERFLEET_LOGGING_LEVEL` | string | `info` |
| `logging.format` | `HYPERFLEET_LOGGING_FORMAT` | string | `json` |
| `logging.output` | `HYPERFLEET_LOGGING_OUTPUT` | string | `stdout` |
| `logging.otel.enabled` | `HYPERFLEET_LOGGING_OTEL_ENABLED` | bool | `false` |
| `logging.otel.sampling_rate` | `HYPERFLEET_LOGGING_OTEL_SAMPLING_RATE` | float | `1.0` |
| `logging.masking.enabled` | `HYPERFLEET_LOGGING_MASKING_ENABLED` | bool | `true` |
| `logging.masking.headers` | `HYPERFLEET_LOGGING_MASKING_SENSITIVE_HEADERS` | csv | `Authorization,Cookie` |
| `logging.masking.fields` | `HYPERFLEET_LOGGING_MASKING_SENSITIVE_FIELDS` | csv | `password,token` |
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
| `metrics.label_metrics_inclusion_duration` | `HYPERFLEET_METRICS_LABEL_METRICS_INCLUSION_DURATION` | duration | `5m` |
| **Health** | | | |
| `health.host` | `HYPERFLEET_HEALTH_HOST` | string | `localhost` |
| `health.port` | `HYPERFLEET_HEALTH_PORT` | int | `8080` |
| `health.tls.enabled` | `HYPERFLEET_HEALTH_TLS_ENABLED` | bool | `false` |
| `health.shutdown_timeout` | `HYPERFLEET_HEALTH_SHUTDOWN_TIMEOUT` | duration | `30s` |
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
| `--db-user` | `database.username` | string |
| `--db-password` | `database.password` | string |
| `--db-debug` | `database.debug` | bool |
| `--db-max-open-connections` | `database.pool.max_connections` | int |
| `--db-root-cert-file` | `database.ssl.root_cert_file` | string |
| **Logging** | | |
| `--log-level`, `-l` | `logging.level` | string |
| `--log-format` | `logging.format` | string |
| `--log-output` | `logging.output` | string |
| `--log-otel-enabled` | `logging.otel.enabled` | bool |
| `--log-otel-sampling-rate` | `logging.otel.sampling_rate` | float |
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

## File-based Secrets

For enhanced security in Kubernetes environments, sensitive values can be loaded from files using `*_FILE` environment variables.

### How it works

Set a `*_FILE` environment variable pointing to a file path:
```bash
export HYPERFLEET_DATABASE_PASSWORD_FILE=/secrets/db-password
```

The application reads the file content and uses it as the configuration value.

### Supported Variables

| Configuration | File Environment Variable |
|---------------|---------------------------|
| `database.host` | `HYPERFLEET_DATABASE_HOST_FILE` |
| `database.port` | `HYPERFLEET_DATABASE_PORT_FILE` |
| `database.name` | `HYPERFLEET_DATABASE_NAME_FILE` |
| `database.username` | `HYPERFLEET_DATABASE_USERNAME_FILE` |
| `database.password` | `HYPERFLEET_DATABASE_PASSWORD_FILE` |
| `ocm.client_id` | `HYPERFLEET_OCM_CLIENT_ID_FILE` |
| `ocm.client_secret` | `HYPERFLEET_OCM_CLIENT_SECRET_FILE` |
| `ocm.self_token` | `HYPERFLEET_OCM_SELF_TOKEN_FILE` |

### Priority

Flags > Plain env vars > `*_FILE` env vars > ConfigMap > Defaults

**Emergency override example:**
```bash
# Override Secret file temporarily
kubectl set env deployment/hyperfleet-api HYPERFLEET_DATABASE_PASSWORD=temp-password

# Revert by removing the override
kubectl set env deployment/hyperfleet-api HYPERFLEET_DATABASE_PASSWORD-
```

See [Configuration Examples](#configuration-examples) for complete Kubernetes Secret setup.

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
- `logging.otel.sampling_rate`: 0.0-1.0

**Adapters**:
- `adapters.required.cluster`: must be array of strings
- `adapters.required.nodepool`: must be array of strings

### Validation Errors

If validation fails, the application will exit with a detailed error message:

```
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
   ```
   Error: Logging.Level must be one of: debug, info, warn, error
   ```
   Solution: Use lowercase: `info`, not `INFO`

2. **Invalid port:**
   ```
   Error: Server.Port must be between 1 and 65535
   ```
   Solution: Check port value in config file or environment variable

3. **Missing required field:**
   ```
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

- ✅ Environment variables (HYPERFLEET_*)
- ✅ CLI flags (--kebab-case)
- ✅ Configuration files (YAML snake_case)
- ✅ File-based secrets (*_FILE)
- ✅ Default values
