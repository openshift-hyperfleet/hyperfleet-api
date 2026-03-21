# HyperFleet API Logging

This document describes the logging system used in hyperfleet-api.

## Overview

HyperFleet API uses Go's standard library `log/slog` for structured logging with the following features:

- **Structured logging**: All logs use key-value pairs for better queryability
- **Context-aware logging**: Automatic request_id, trace_id, and span_id propagation
- **Data masking**: Sensitive data redaction in headers and JSON payloads
- **OpenTelemetry integration**: Distributed tracing with configurable sampling
- **JSON and text output**: Machine-parseable JSON or human-readable text format
- **Custom handler**: Automatic component, version, and hostname fields

## Architecture

### Components

1. **pkg/logger/logger.go**: Core logger with HyperFleetHandler (custom slog.Handler)
2. **pkg/logger/context.go**: Context key definitions for trace_id, span_id, cluster_id, etc.
3. **pkg/logger/requestid_middleware.go**: Request ID generation and middleware
4. **pkg/logger/ocm_bridge.go**: OCMLogger interface for backward compatibility
5. **pkg/middleware/otel.go**: OpenTelemetry trace context extraction
6. **pkg/telemetry/otel.go**: OpenTelemetry trace provider initialization

### Middleware Chain

```text
HTTP Request
  ↓
RequestIDMiddleware (adds request_id to context)
  ↓
OTelMiddleware (extracts trace_id and span_id, optional)
  ↓
RequestLoggingMiddleware (logs request/response with masking)
  ↓
Handler (business logic)
```

## Configuration

Logging is configured through environment variables or configuration files.

**Development:**
```bash
# Human-readable text format with debug level
export HYPERFLEET_LOGGING_FORMAT=text
export HYPERFLEET_LOGGING_LEVEL=debug
```

**Production:**
```bash
# Structured JSON format with info level
export HYPERFLEET_LOGGING_FORMAT=json
export HYPERFLEET_LOGGING_LEVEL=info

# OpenTelemetry tracing (Tracing standard)
export TRACING_ENABLED=true
export OTEL_TRACES_SAMPLER=parentbased_traceidratio
export OTEL_TRACES_SAMPLER_ARG=0.1
export OTEL_EXPORTER_OTLP_ENDPOINT=http://otel-collector:4317
```

**For complete configuration reference**, including all logging settings (levels, formats, OpenTelemetry, masking), see:
- **[Configuration Guide](config.md)** - All logging environment variables and defaults

### OpenTelemetry Environment Variables

HyperFleet uses standard OpenTelemetry environment variables for tracing configuration:

| Variable | Description | Default | Example |
|----------|-------------|---------|---------|
| `TRACING_ENABLED` | Enable/disable tracing (Tracing standard, overrides config) | - | `true`, `false` |
| `HYPERFLEET_LOGGING_OTEL_ENABLED` | Enable tracing via config (Viper) | `false` | `true`, `false` |
| `OTEL_SERVICE_NAME` | Service name in traces | `hyperfleet-api` | `hyperfleet-api-prod` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector endpoint (if not set, uses stdout) | - | `http://otel-collector:4317` |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | OTLP protocol | `grpc` | `grpc`, `http/protobuf` |
| `OTEL_TRACES_SAMPLER` | Sampler type | `parentbased_traceidratio` | `always_on`, `traceidratio` |
| `OTEL_TRACES_SAMPLER_ARG` | Sampling rate (0.0-1.0) | `1.0` | `0.1` (10%) |
| `OTEL_RESOURCE_ATTRIBUTES` | Additional resource attributes | - | `env=prod,region=us-east` |

**Variable Precedence (highest to lowest):**
1. `TRACING_ENABLED` - Tracing standard override
2. `HYPERFLEET_LOGGING_OTEL_ENABLED` - Config via Viper (env var)
3. `config.yaml: logging.otel.enabled` - Config file
4. Default (`false`)

## Usage

### Basic Logging

Always use context-aware logging to include automatic fields (request_id, trace_id, span_id):

```go
import (
    "context"
    "github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

func MyHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Simple log (context fields only)
    logger.Info(ctx, "Processing cluster creation")

    // Log with temporary fields using With()
    logger.With(ctx, "cluster_id", clusterID, "region", region).Info("Cluster created")

    // Error level (automatically includes stack trace)
    logger.With(ctx, "host", "localhost").WithError(err).Error("Database connection failed")

    // Debug level
    logger.With(ctx, "key", "cluster:123").Debug("Cache hit")

    // Warning level
    logger.With(ctx, "used_mb", 1024, "threshold_mb", 800).Warn("High memory usage")
}
```

Logs automatically include:
- `component`: "hyperfleet-api"
- `version`: Application version
- `hostname`: Pod/host name
- `request_id`: Unique request identifier
- `trace_id`: W3C trace ID (when OTel enabled)
- `span_id`: Current span ID (when OTel enabled)

### Available Functions

```go
// Simple logging (context fields only)
logger.Info(ctx, "message")
logger.Warn(ctx, "message")
logger.Error(ctx, "message")
logger.Debug(ctx, "message")

// Logging with temporary fields (use With())
logger.With(ctx, "key", "value").Info("message")
logger.With(ctx, "key1", value1, "key2", value2).Error("message")

// Logging with errors (use WithError())
logger.WithError(ctx, err).Error("Operation failed")
logger.With(ctx, "host", "localhost").WithError(err).Error("Connection failed")

// Chaining multiple With() calls
logger.With(ctx, "user_id", userID).
    With("action", "login").
    WithError(err).
    Error("Login failed")

// Add persistent context fields
ctx = logger.WithClusterID(ctx, "cluster-123")
ctx = logger.WithResourceType(ctx, "managed-cluster")
ctx = logger.WithResourceID(ctx, "resource-456")
```

### Field Constants

Use field constants from `pkg/logger/fields.go` to prevent typos and enable IDE autocomplete:

```go
logger.With(ctx, logger.FieldEnvironment, "production").Info("Environment loaded")
logger.With(ctx, logger.FieldBindAddress, ":8080").Info("Server starting")
logger.With(ctx, logger.FieldConnectionString, sanitized).Info("Database connected")
```

See `pkg/logger/fields.go` for all available constants (Server/Config, Database, OpenTelemetry, etc.).

### HTTP Helper Functions

Use HTTP helpers from `pkg/logger/http.go` for consistent HTTP field logging:

```go
logger.With(r.Context(),
    logger.HTTPMethod(r.Method),
    logger.HTTPPath(r.URL.Path),
    logger.HTTPStatusCode(200),
).Info("Request processed")
```

See `pkg/logger/http.go` for all available helpers (HTTPMethod, HTTPPath, HTTPStatusCode, etc.).

## Format Comparison

HyperFleet API supports two log output formats: **JSON** (for production/log aggregation) and **Text** (for development/debugging).

### JSON Format (`LOG_FORMAT=json`)

**Characteristics:**
- Machine-parseable structured format
- Ideal for log aggregation systems (Elasticsearch, Splunk, etc.)
- All fields as JSON key-value pairs
- Default format for production deployments

**Example:**
```json
{"timestamp":"2026-01-09T12:30:45Z","level":"info","message":"Server started","component":"hyperfleet-api","version":"v1.2.3","hostname":"pod-abc","request_id":"2C9zKDz8xQMqF3yH","port":8000}
```

### Text Format (`LOG_FORMAT=text`)

**Characteristics:**
- Human-readable format following HyperFleet Logging Specification
- Format: `{timestamp} {LEVEL} [{component}] [{version}] [{hostname}] {message} {key=value}...`
- System fields (`component`, `version`, `hostname`) in brackets for clarity
- Level in uppercase for quick visual scanning
- Ideal for local development and real-time log monitoring

**Example:**
```text
2026-01-09T12:30:45Z INFO [hyperfleet-api] [v1.2.3] [pod-abc] Server started request_id=2C9zKDz8xQMqF3yH port=8000
```

### Side-by-Side Comparison

**Same log statement:**
```go
logger.With(ctx, "cluster_id", "cluster-abc123", "region", "us-east-1").Info("Processing cluster creation")
```

**JSON output:**
```json
{"timestamp":"2026-01-09T12:30:45Z","level":"info","source":{"function":"main.handler","file":"/app/main.go","line":45},"message":"Processing cluster creation","component":"hyperfleet-api","version":"v1.2.3","hostname":"pod-abc","request_id":"2C9zKDz8xQMqF3yH","cluster_id":"cluster-abc123","region":"us-east-1"}
```

**Text output:**
```text
2026-01-09T12:30:45Z INFO [hyperfleet-api] [v1.2.3] [pod-abc] Processing cluster creation request_id=2C9zKDz8xQMqF3yH cluster_id=cluster-abc123 region=us-east-1
```

### Switching Between Formats

Toggle between formats using the `HYPERFLEET_LOGGING_FORMAT` environment variable (`json` or `text`). No code changes required - the logger automatically adapts output format based on configuration.

See the [Configuration](#configuration) section above for examples.

## Database Logging

HyperFleet API automatically integrates database (GORM) logging with the application's `LOG_LEVEL` configuration while providing a `DB_DEBUG` override for database-specific debugging.

### HYPERFLEET_LOGGING_LEVEL Integration

Database logs follow the application log level by default:

| HYPERFLEET_LOGGING_LEVEL | GORM Behavior | What Gets Logged |
|--------------------------|---------------|------------------|
| `debug` | Info level | All SQL queries with parameters, duration, and row counts |
| `info` | Warn level | Only slow queries (>200ms) and errors |
| `warn` | Warn level | Only slow queries (>200ms) and errors |
| `error` | Silent | Nothing (database logging disabled) |

### HYPERFLEET_DATABASE_DEBUG Override

The `HYPERFLEET_DATABASE_DEBUG` environment variable provides database-specific debugging without changing the global `HYPERFLEET_LOGGING_LEVEL`:

```bash
# Production environment with database debugging
export HYPERFLEET_LOGGING_LEVEL=info   # Application logs remain at INFO
export HYPERFLEET_LOGGING_FORMAT=json  # Production format
export HYPERFLEET_DATABASE_DEBUG=true  # Force all SQL queries to be logged
./bin/hyperfleet-api serve
```

**Priority:**
1. If `HYPERFLEET_DATABASE_DEBUG=true`, all SQL queries are logged (GORM Info level)
2. Otherwise, follow `HYPERFLEET_LOGGING_LEVEL` mapping (see table above)

### Database Log Examples

**Fast query (LOG_LEVEL=debug or DB_DEBUG=true):**

JSON format:
```json
{
  "timestamp": "2026-01-14T11:31:29.788683+08:00",
  "level": "info",
  "message": "GORM query",
  "duration_ms": 9.052167,
  "rows": 1,
  "sql": "INSERT INTO \"clusters\" (\"id\",\"created_time\",...) VALUES (...)",
  "component": "api",
  "version": "0120ac6-modified",
  "hostname": "yasun-mac",
  "request_id": "38EOuujxBDUduP0hYLxVGMm69Dq",
  "transaction_id": 1157
}
```

Text format:
```text
2026-01-14T11:34:23+08:00 INFO [api] [0120ac6-modified] [yasun-mac] GORM query request_id=38EPGnassU9SLNZ82XiXZLiWS4i duration_ms=10.135875 rows=1 sql="INSERT INTO \"clusters\" (\"id\",\"created_time\",...) VALUES (...)"
```

**Slow query (>200ms, visible at all log levels except error):**

```json
{
  "timestamp": "2026-01-14T12:00:00Z",
  "level": "warn",
  "message": "GORM query",
  "duration_ms": 250.5,
  "rows": 1000,
  "sql": "SELECT * FROM clusters WHERE ...",
  "request_id": "...",
  "transaction_id": 1234
}
```

**Database error (visible at all log levels):**

```json
{
  "timestamp": "2026-01-14T12:00:00Z",
  "level": "error",
  "message": "GORM query error",
  "error": "pq: duplicate key value violates unique constraint \"idx_clusters_name\"",
  "duration_ms": 10.5,
  "rows": 0,
  "sql": "INSERT INTO \"clusters\" ...",
  "request_id": "..."
}
```

### Configuration Priority

The `HYPERFLEET_DATABASE_DEBUG` environment variable takes precedence over the global logging level. When `HYPERFLEET_DATABASE_DEBUG` is not set, database logging automatically follows `HYPERFLEET_LOGGING_LEVEL`.

## Log Output Examples

### Error Logs with Stack Traces

**Code**:
```go
logger.With(ctx, "host", "postgres.svc").WithError(err).Error("Failed to connect to database")
```

**JSON Output**:
```json
{
  "timestamp": "2026-01-09T12:30:45Z",
  "level": "error",
  "source": {"function": "db.Connect", "file": "/app/db/connection.go", "line": 45},
  "message": "Failed to connect to database",
  "host": "postgres.svc",
  "error": "dial tcp: lookup postgres.svc: no such host",
  "component": "hyperfleet-api",
  "version": "v1.2.3",
  "hostname": "pod-abc",
  "request_id": "2C9zKDz8xQMqF3yH",
  "stack_trace": [
    "/workspace/pkg/db/connection.go:45 github.com/openshift-hyperfleet/hyperfleet-api/pkg/db.Connect",
    "/workspace/pkg/db/factory.go:78 github.com/openshift-hyperfleet/hyperfleet-api/pkg/db.NewSessionFactory",
    "/workspace/cmd/hyperfleet-api/servecmd/cmd.go:123 main.setupDatabase"
  ]
}
```

**Text Output** (multi-line stack trace for readability):
```text
2026-01-09T12:30:45Z ERROR [hyperfleet-api] [v1.2.3] [pod-abc] Failed to connect to database request_id=2C9zKDz8xQMqF3yH host=postgres.svc error="dial tcp: lookup postgres.svc: no such host"
  stack_trace:
    /workspace/pkg/db/connection.go:45 github.com/openshift-hyperfleet/hyperfleet-api/pkg/db.Connect
    /workspace/pkg/db/factory.go:78 github.com/openshift-hyperfleet/hyperfleet-api/pkg/db.NewSessionFactory
    /workspace/cmd/hyperfleet-api/servecmd/cmd.go:123 main.setupDatabase
```

**Note**: Error-level logs automatically include stack traces.

## OpenTelemetry Integration

### Initialization

OpenTelemetry is initialized in `cmd/hyperfleet-api/servecmd/cmd.go`:

```go
// Precedence: TRACING_ENABLED (tracing standard) > config (env/flags) > default
var tracingEnabled bool
if tracingEnv := os.Getenv("TRACING_ENABLED"); tracingEnv != "" {
    tracingEnabled, _ = strconv.ParseBool(tracingEnv)
} else {
    tracingEnabled = environments.Environment().Config.Logging.OTel.Enabled
}

if tracingEnabled {
    serviceName := "hyperfleet-api"
    if svcName := os.Getenv("OTEL_SERVICE_NAME"); svcName != "" {
        serviceName = svcName
    }

    tp, err := telemetry.InitTraceProvider(ctx, serviceName, api.Version)
    if err != nil {
        logger.WithError(ctx, err).Warn("Failed to initialize OpenTelemetry")
    } else {
        defer tp.Shutdown(context.Background())
        logger.With(ctx, logger.FieldServiceName, serviceName).Info("OpenTelemetry initialized")
    }
}
```

### Trace Propagation

The OTel middleware automatically:
1. Extracts W3C trace context from incoming HTTP headers
2. Creates or continues spans for each request
3. Injects trace_id and span_id into the logger context
4. Exports traces to stdout (can be configured for other exporters)

### Sampling

Configure sampling using standard OpenTelemetry environment variables:

```bash
# Sampler type (default: parentbased_traceidratio)
export OTEL_TRACES_SAMPLER=parentbased_traceidratio

# Sampling rate: 0.0-1.0 (default: 1.0)
export OTEL_TRACES_SAMPLER_ARG=0.1  # 10% of requests traced
```

**Sampling rate examples:**
- `0.0`: No traces (disabled)
- `0.1`: 10% of requests traced (recommended for production)
- `1.0`: All requests traced (development only)

**Sampler types:**
- `always_on`: Sample all requests
- `always_off`: Sample no requests
- `traceidratio`: Sample based on trace ID ratio (use with OTEL_TRACES_SAMPLER_ARG)
- `parentbased_traceidratio`: Respect parent decision, otherwise use trace ID ratio (default)

## Data Masking

Sensitive data is automatically masked when `MASKING_ENABLED=true`:

**Default masked headers**: `Authorization`, `Cookie`, `X-API-Key`, `X-Auth-Token`
**Default masked fields**: `password`, `token`, `secret`, `api_key`, `client_secret`

To add custom masking rules:

```go
env().Config.Logging.Masking.Headers = append(
    env().Config.Logging.Masking.Headers,
    "X-Custom-Auth-Header",
)

env().Config.Logging.Masking.Fields = append(
    env().Config.Logging.Masking.Fields,
    "credit_card",
    "ssn",
)
```

## Best Practices

### Application Logging

1. **Always use context**: `logger.Info(ctx, "msg")` not `slog.Info("msg")`
2. **Use WithError for errors**: `logger.WithError(ctx, err).Error(...)` not `"error", err`
3. **Use field constants**: `logger.FieldEnvironment` not `"environment"`
4. **Use HTTP helpers**: `logger.HTTPMethod(r.Method)` not `"method", r.Method`
5. **Chain for readability**: `logger.With(ctx, ...).WithError(err).Error(...)`
6. **Never log sensitive data**: Always sanitize passwords, tokens, connection strings
7. **Choose appropriate levels**: DEBUG (dev), INFO (normal), WARN (client error), ERROR (server error)

### Database Logging

1. **Use HYPERFLEET_LOGGING_LEVEL for database logs**: Don't set `HYPERFLEET_DATABASE_DEBUG` unless specifically debugging database issues
2. **Production default**: `HYPERFLEET_LOGGING_LEVEL=info` hides fast queries, shows slow queries (>200ms)
3. **Temporary debugging**: Use `HYPERFLEET_DATABASE_DEBUG=true` for production database troubleshooting, then disable it
4. **Development**: Use `HYPERFLEET_LOGGING_LEVEL=debug` to see all SQL queries during development
5. **High-traffic systems**: Consider `HYPERFLEET_LOGGING_LEVEL=warn` to minimize database log volume
6. **Monitor slow queries**: Review WARN-level GORM logs for queries exceeding 200ms threshold

## Troubleshooting

### Logs Not Appearing

1. Check log level: `export HYPERFLEET_LOGGING_LEVEL=debug`
2. Verify text mode: `export HYPERFLEET_LOGGING_FORMAT=text` (for human-readable output)
3. Check context propagation: Ensure middleware chain is correct

### Missing request_id

Verify `RequestIDMiddleware` is registered before `RequestLoggingMiddleware`:

```go
mainRouter.Use(logger.RequestIDMiddleware)
mainRouter.Use(middleware.OTelMiddleware)
mainRouter.Use(logging.RequestLoggingMiddleware)
```

### Missing trace_id/span_id

1. Check tracing is enabled: `export TRACING_ENABLED=true`
2. Verify middleware order: `OTelMiddleware` must be after `RequestIDMiddleware`
3. Check sampling rate: `export OTEL_TRACES_SAMPLER_ARG=1.0` (for testing - trace all requests)

### Data Not Masked

1. Check masking is enabled: `export HYPERFLEET_LOGGING_MASKING_ENABLED=true`
2. Verify field names match configuration (case-insensitive)
3. Check JSON structure: Masking only works on top-level fields

### SQL Queries Not Appearing

1. Check log level: `export HYPERFLEET_LOGGING_LEVEL=debug` (to see all SQL queries)
2. Check database debug: `export HYPERFLEET_DATABASE_DEBUG=true` (to force SQL logging at any log level)
3. Verify queries are executing: Check if API operations complete successfully
4. Check log format: Use `HYPERFLEET_LOGGING_FORMAT=text` for easier debugging

### Too Many SQL Queries in Logs

1. Production mode: `export HYPERFLEET_LOGGING_LEVEL=info` (hides fast queries < 200ms)
2. Disable database debug: `export HYPERFLEET_DATABASE_DEBUG=false` or unset it
3. Minimal mode: `export HYPERFLEET_LOGGING_LEVEL=warn` (only slow queries and errors)
4. Silent mode: `export HYPERFLEET_LOGGING_LEVEL=error` (no SQL queries logged)

### Only Want to See Slow Queries

Use production default configuration:
```bash
export HYPERFLEET_LOGGING_LEVEL=info
export HYPERFLEET_LOGGING_FORMAT=json
export HYPERFLEET_DATABASE_DEBUG=false  # or leave unset
```

This will only log SQL queries that take longer than 200ms.

## Testing

### Unit Tests

```go
func TestLogging(t *testing.T) {
    // Create context with request ID
    ctx := logger.WithRequestID(context.Background())

    // Log with context
    logger.With(ctx, "key", "value").Info("Test message")

    // Verify request_id is included
    // (Use a test handler to capture logs)
}
```

### Integration Tests

```bash
# Run tests with debug logging
HYPERFLEET_LOGGING_LEVEL=debug OCM_ENV=integration_testing go test ./test/integration/...

# Run tests without OTel
TRACING_ENABLED=false OCM_ENV=integration_testing go test ./...
```

## References

- [slog Documentation](https://pkg.go.dev/log/slog)
- [OpenTelemetry Go SDK](https://opentelemetry.io/docs/languages/go/)
- [W3C Trace Context](https://www.w3.org/TR/trace-context/)
- [HyperFleet Architecture](https://github.com/openshift-hyperfleet/architecture)
