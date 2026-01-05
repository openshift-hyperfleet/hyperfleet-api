# HyperFleet API Configuration Documentation

This document describes all configuration options available for the HyperFleet API service.

## Configuration Sources

The HyperFleet API follows the [HyperFleet Configuration Standard](../configuration-standard.md) and loads configuration from multiple sources with the following precedence (highest to lowest):

1. **Command-line flags** (highest priority)
2. **Environment variables** (prefixed with `HYPERFLEET_`)
3. **Configuration files** (YAML format)
4. **Default values** (lowest priority)

## Configuration File Location

The configuration file path is determined by:

1. Path specified via `--config` flag (if provided)
2. Path specified via `HYPERFLEET_CONFIG` environment variable
3. Default paths (first found is used):
   - Development: `./configs/config.yaml`
   - Production: `/etc/hyperfleet/config.yaml`

If no configuration file is found, the application continues with flags, environment variables, and defaults only.

## Global Configuration Options

### Application Settings

| Field | Flag | Environment Variable | Type | Default | Description | Required |
|-------|------|---------------------|------|---------|-------------|----------|
| `app.name` | `--name`, `-n` | `HYPERFLEET_APP_NAME` | string | `""` | Component name | Yes |
| `app.version` | `--version`, `-v` | `HYPERFLEET_APP_VERSION` | string | `"1.0.0"` | Component version | No |

**Example:**
```yaml
app:
  name: hyperfleet-api
  version: 1.0.0
```

```bash
# Via flags
./hyperfleet-api serve --name hyperfleet-api --version 1.0.0

# Via environment variables
export HYPERFLEET_APP_NAME=hyperfleet-api
export HYPERFLEET_APP_VERSION=1.0.0
```

## Server Configuration

### Server Settings

| Field | Flag | Environment Variable | Type | Default | Description | Validation |
|-------|------|---------------------|------|---------|-------------|------------|
| `server.hostname` | `--server-hostname` | `HYPERFLEET_SERVER_HOSTNAME` | string | `""` | Server's public hostname | |
| `server.host` | `--server-host` | `HYPERFLEET_SERVER_HOST` | string | `"localhost"` | Server bind host | Required |
| `server.port` | `--server-port`, `-p` | `HYPERFLEET_SERVER_PORT` | int | `8000` | Server bind port | Required, 1-65535 |
| `server.timeout.read` | `--server-timeout-read` | `HYPERFLEET_SERVER_TIMEOUT_READ` | duration | `5s` | HTTP server read timeout | |
| `server.timeout.write` | `--server-timeout-write` | `HYPERFLEET_SERVER_TIMEOUT_WRITE` | duration | `30s` | HTTP server write timeout | |

**Example:**
```yaml
server:
  hostname: ""
  host: localhost
  port: 8000
  timeout:
    read: 5s
    write: 30s
```

```bash
# Via flags
./hyperfleet-api serve --server-host 0.0.0.0 --server-port 8000

# Via environment variables
export HYPERFLEET_SERVER_HOST=0.0.0.0
export HYPERFLEET_SERVER_PORT=8000
```

### HTTPS Configuration

| Field | Flag | Environment Variable | Type | Default | Description |
|-------|------|---------------------|------|---------|-------------|
| `server.https.enabled` | `--server-https-enabled` | `HYPERFLEET_SERVER_HTTPS_ENABLED` | bool | `false` | Enable HTTPS |
| `server.https.cert_file` | `--server-https-cert-file` | `HYPERFLEET_SERVER_HTTPS_CERT_FILE` | string | `""` | Path to TLS certificate file |
| `server.https.key_file` | `--server-https-key-file` | `HYPERFLEET_SERVER_HTTPS_KEY_FILE` | string | `""` | Path to TLS key file |

**Example:**
```yaml
server:
  https:
    enabled: true
    cert_file: /etc/certs/tls.crt
    key_file: /etc/certs/tls.key
```

```bash
# Via flags
./hyperfleet-api serve --server-https-enabled --server-https-cert-file /etc/certs/tls.crt --server-https-key-file /etc/certs/tls.key

# Via environment variables
export HYPERFLEET_SERVER_HTTPS_ENABLED=true
export HYPERFLEET_SERVER_HTTPS_CERT_FILE=/etc/certs/tls.crt
export HYPERFLEET_SERVER_HTTPS_KEY_FILE=/etc/certs/tls.key
```

### JWT Authentication Configuration

| Field | Flag | Environment Variable | Type | Default | Description |
|-------|------|---------------------|------|---------|-------------|
| `server.auth.jwt.enabled` | `--auth-jwt-enabled` | `HYPERFLEET_SERVER_AUTH_JWT_ENABLED` | bool | `true` | Enable JWT authentication |
| `server.auth.jwt.cert_file` | `--auth-jwt-cert-file` | `HYPERFLEET_SERVER_AUTH_JWT_CERT_FILE` | string | `""` | JWK certificate file path |
| `server.auth.jwt.cert_url` | `--auth-jwt-cert-url` | `HYPERFLEET_SERVER_AUTH_JWT_CERT_URL` | string | `https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/certs` | JWK certificate URL |

**Example:**
```yaml
server:
  auth:
    jwt:
      enabled: true
      cert_file: ""
      cert_url: https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/certs
```

### Authorization Configuration

| Field | Flag | Environment Variable | Type | Default | Description |
|-------|------|---------------------|------|---------|-------------|
| `server.auth.authz.enabled` | `--auth-authz-enabled` | `HYPERFLEET_SERVER_AUTH_AUTHZ_ENABLED` | bool | `true` | Enable authorization |
| `server.auth.authz.acl_file` | `--auth-authz-acl-file` | `HYPERFLEET_SERVER_AUTH_AUTHZ_ACL_FILE` | string | `""` | Access control list file |

**Example:**
```yaml
server:
  auth:
    authz:
      enabled: true
      acl_file: /etc/config/acl.yaml
```

## Database Configuration

| Field | Flag | Environment Variable | Type | Default | Description | Validation |
|-------|------|---------------------|------|---------|-------------|------------|
| `database.dialect` | | `HYPERFLEET_DATABASE_DIALECT` | string | `"postgres"` | Database dialect | Required |
| `database.host` | `--db-host` | `HYPERFLEET_DATABASE_HOST` | string | `""` | Database host | |
| `database.port` | `--db-port` | `HYPERFLEET_DATABASE_PORT` | int | `0` | Database port | 0-65535 |
| `database.name` | `--db-name`, `-d` | `HYPERFLEET_DATABASE_NAME` | string | `""` | Database name | |
| `database.username` | `--db-username`, `-u` | `HYPERFLEET_DATABASE_USERNAME` | string | `""` | Database username | |
| `database.password` | `--db-password` | `HYPERFLEET_DATABASE_PASSWORD` | string | `""` | Database password (prefer env var) | |
| `database.sslmode` | `--db-sslmode` | `HYPERFLEET_DATABASE_SSLMODE` | string | `"disable"` | SSL mode | disable, require, verify-ca, verify-full |
| `database.rootcert_file` | `--db-rootcert` | `HYPERFLEET_DATABASE_ROOTCERT_FILE` | string | `"secrets/db.rootcert"` | Root certificate file | |
| `database.debug` | `--db-debug` | `HYPERFLEET_DATABASE_DEBUG` | bool | `false` | Enable database debug mode | |
| `database.max_open_connections` | `--db-max-open-connections` | `HYPERFLEET_DATABASE_MAX_OPEN_CONNECTIONS` | int | `50` | Maximum open connections | Min: 1 |

### File-based Secrets

For enhanced security, database credentials can be loaded from files:

| Field | Flag | Default |
|-------|------|---------|
| `database.host_file` | `--db-host-file` | `secrets/db.host` |
| `database.port_file` | `--db-port-file` | `secrets/db.port` |
| `database.name_file` | `--db-name-file` | `secrets/db.name` |
| `database.username_file` | `--db-username-file` | `secrets/db.user` |
| `database.password_file` | `--db-password-file` | `secrets/db.password` |

**Note:** File-based values are only used if the corresponding direct value is not set.

**Example:**
```yaml
database:
  dialect: postgres
  host: db.example.com
  port: 5432
  name: hyperfleet
  username: hyperfleet_user
  # Password should be set via environment variable or file
  sslmode: require
  max_open_connections: 50
```

```bash
# Via environment variables (recommended for credentials)
export HYPERFLEET_DATABASE_HOST=db.example.com
export HYPERFLEET_DATABASE_PORT=5432
export HYPERFLEET_DATABASE_NAME=hyperfleet
export HYPERFLEET_DATABASE_USERNAME=hyperfleet_user
export HYPERFLEET_DATABASE_PASSWORD=super_secret_password
```

## Metrics Configuration

| Field | Flag | Environment Variable | Type | Default | Description | Validation |
|-------|------|---------------------|------|---------|-------------|------------|
| `metrics.host` | `--metrics-host` | `HYPERFLEET_METRICS_HOST` | string | `"localhost"` | Metrics server bind host | |
| `metrics.port` | `--metrics-port` | `HYPERFLEET_METRICS_PORT` | int | `8080` | Metrics server bind port | 1-65535 |
| `metrics.enable_https` | `--metrics-https-enabled` | `HYPERFLEET_METRICS_ENABLE_HTTPS` | bool | `false` | Enable HTTPS for metrics | |
| `metrics.label_metrics_inclusion_duration` | `--metrics-label-inclusion-duration` | `HYPERFLEET_METRICS_LABEL_METRICS_INCLUSION_DURATION` | duration | `168h` | Label metrics inclusion duration | |

**Example:**
```yaml
metrics:
  host: localhost
  port: 8080
  enable_https: false
  label_metrics_inclusion_duration: 168h  # 7 days
```

## Health Check Configuration

| Field | Flag | Environment Variable | Type | Default | Description | Validation |
|-------|------|---------------------|------|---------|-------------|------------|
| `health_check.host` | `--health-check-host` | `HYPERFLEET_HEALTH_CHECK_HOST` | string | `"localhost"` | Health check server bind host | |
| `health_check.port` | `--health-check-port` | `HYPERFLEET_HEALTH_CHECK_PORT` | int | `8083` | Health check server bind port | 1-65535 |
| `health_check.enable_https` | `--health-check-https-enabled` | `HYPERFLEET_HEALTH_CHECK_ENABLE_HTTPS` | bool | `false` | Enable HTTPS for health check | |

**Example:**
```yaml
health_check:
  host: localhost
  port: 8083
  enable_https: false
```

## OCM (OpenShift Cluster Manager) Configuration

| Field | Flag | Environment Variable | Type | Default | Description |
|-------|------|---------------------|------|---------|-------------|
| `ocm.base_url` | `--ocm-base-url` | `HYPERFLEET_OCM_BASE_URL` | string | `https://api.integration.openshift.com` | OCM API base URL |
| `ocm.token_url` | `--ocm-token-url` | `HYPERFLEET_OCM_TOKEN_URL` | string | `https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token` | OCM token URL |
| `ocm.debug` | `--ocm-debug` | `HYPERFLEET_OCM_DEBUG` | bool | `false` | Enable OCM debug mode |
| `ocm.enable_mock` | `--ocm-mock` | `HYPERFLEET_OCM_ENABLE_MOCK` | bool | `true` | Enable mock OCM client |

### File-based Secrets

OCM credentials can be loaded from files:

| Field | Flag | Default |
|-------|------|---------|
| `ocm.client_id_file` | `--ocm-client-id-file` | `secrets/ocm-service.clientId` |
| `ocm.client_secret_file` | `--ocm-client-secret-file` | `secrets/ocm-service.clientSecret` |
| `ocm.self_token_file` | `--ocm-self-token-file` | `""` |

**Example:**
```yaml
ocm:
  base_url: https://api.integration.openshift.com
  token_url: https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token
  debug: false
  enable_mock: false
  client_id_file: secrets/ocm-service.clientId
  client_secret_file: secrets/ocm-service.clientSecret
```

## Configuration Validation

The service performs validation on startup and will fail to start if:

- Required fields are missing (e.g., `app.name`)
- Values are outside valid ranges (e.g., port numbers)
- Unknown fields are present in the configuration file
- Required files cannot be read

Validation errors include helpful hints showing how to provide the correct values via flags, environment variables, or configuration files.

**Example validation error:**
```
Configuration validation failed:
  - Field 'Config.App.Name' failed validation: required
    Value:
    Please provide via:
      • Flag: --name or -n
      • Environment variable: HYPERFLEET_APP_NAME
      • Config file: app.name
```

## Configuration Display

On startup, the merged configuration is displayed in the logs with sensitive values redacted. The following fields are automatically redacted:

- `database.password`
- `ocm.client_secret`
- `ocm.self_token`

Redacted values are shown as `***` to indicate they are set but not displayed.

## Complete Configuration Example

```yaml
# Application configuration
app:
  name: hyperfleet-api
  version: 1.0.0

# Server configuration
server:
  hostname: ""
  host: 0.0.0.0
  port: 8000
  timeout:
    read: 5s
    write: 30s
  https:
    enabled: false
    cert_file: ""
    key_file: ""
  auth:
    jwt:
      enabled: true
      cert_file: ""
      cert_url: https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/certs
    authz:
      enabled: true
      acl_file: ""

# Database configuration
database:
  dialect: postgres
  host: db.example.com
  port: 5432
  name: hyperfleet
  username: hyperfleet_user
  sslmode: require
  debug: false
  max_open_connections: 50
  # File-based secrets
  host_file: secrets/db.host
  port_file: secrets/db.port
  name_file: secrets/db.name
  username_file: secrets/db.user
  password_file: secrets/db.password
  rootcert_file: secrets/db.rootcert

# Metrics configuration
metrics:
  host: localhost
  port: 8080
  enable_https: false
  label_metrics_inclusion_duration: 168h

# Health check configuration
health_check:
  host: localhost
  port: 8083
  enable_https: false

# OCM configuration
ocm:
  base_url: https://api.integration.openshift.com
  token_url: https://sso.redhat.com/auth/realms/redhat-external/protocol/openid-connect/token
  debug: false
  enable_mock: false
  client_id_file: secrets/ocm-service.clientId
  client_secret_file: secrets/ocm-service.clientSecret
  self_token_file: ""
```

## Environment-Specific Configuration

The HyperFleet API supports multiple runtime environments controlled via the `OCM_ENV` environment variable:

- `development` (default)
- `integration-testing`
- `unit-testing`
- `production`

Each environment can have specific default configurations that override the base defaults.

## Best Practices

1. **Never commit secrets**: Use environment variables or file-based secrets for sensitive data
2. **Use configuration files for non-sensitive defaults**: Keep your deployment-specific settings in YAML files
3. **Override at runtime**: Use flags for quick testing and overrides
4. **Validate early**: The application will fail fast on invalid configuration
5. **Document exceptions**: If you add custom configuration options, document them in this file

## See Also

- [HyperFleet Configuration Standard](../configuration-standard.md)
- [Configuration Implementation Example](https://github.com/rh-amarin/viper-cobra-validation-poc)
