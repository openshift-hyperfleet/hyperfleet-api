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

### Environment Variables

```bash
# Logging level (debug, info, warn, error)
export LOG_LEVEL=info

# Logging format (json, text)
export LOG_FORMAT=json

# Log output destination (stdout, stderr)
export LOG_OUTPUT=stdout

# Enable OpenTelemetry (true/false)
export OTEL_ENABLED=true

# OpenTelemetry sampling rate (0.0 to 1.0)
# 0.0 = no traces, 1.0 = all traces
export OTEL_SAMPLING_RATE=0.1

# Data masking (true/false)
export MASKING_ENABLED=true

# Headers to mask (comma-separated)
export MASKING_HEADERS="Authorization,Cookie,X-API-Key"

# JSON body fields to mask (comma-separated)
export MASKING_FIELDS="password,token,secret,api_key"
```

### Configuration Struct

```go
type LoggingConfig struct {
    Level   string         // LOG_LEVEL
    Format  string         // LOG_FORMAT
    Output  string         // LOG_OUTPUT
    OTel    OTelConfig     // OTEL_*
    Masking MaskingConfig  // MASKING_*
}

type OTelConfig struct {
    Enabled      bool    // OTEL_ENABLED
    SamplingRate float64 // OTEL_SAMPLING_RATE
}

type MaskingConfig struct {
    Enabled bool     // MASKING_ENABLED
    Headers []string // MASKING_HEADERS (comma-separated)
    Fields  []string // MASKING_FIELDS (comma-separated)
}
```

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

Toggle between formats using the `LOG_FORMAT` environment variable:

```bash
# Development: human-readable text
export LOG_FORMAT=text
export LOG_LEVEL=debug

# Production: structured JSON
export LOG_FORMAT=json
export LOG_LEVEL=info
```

No code changes required - the logger automatically adapts output format based on configuration.

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
if environments.Environment().Config.Logging.OTel.Enabled {
    samplingRate := environments.Environment().Config.Logging.OTel.SamplingRate
    tp, err := telemetry.InitTraceProvider(ctx, "hyperfleet-api", api.Version, samplingRate)
    if err != nil {
        logger.WithError(ctx, err).Warn("Failed to initialize OpenTelemetry")
    } else {
        defer func() {
            if err := tp.Shutdown(context.Background()); err != nil {
                logger.WithError(ctx, err).Error("Error shutting down tracer provider")
            }
        }()
        logger.With(ctx, logger.FieldSamplingRate, samplingRate).Info("OpenTelemetry initialized")
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

Configure sampling rate to control trace volume:
- `0.0`: No traces (disabled)
- `0.1`: 10% of requests traced
- `1.0`: All requests traced (use in development only)

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

1. **Always use context**: `logger.Info(ctx, "msg")` not `slog.Info("msg")`
2. **Use WithError for errors**: `logger.WithError(ctx, err).Error(...)` not `"error", err`
3. **Use field constants**: `logger.FieldEnvironment` not `"environment"`
4. **Use HTTP helpers**: `logger.HTTPMethod(r.Method)` not `"method", r.Method`
5. **Chain for readability**: `logger.With(ctx, ...).WithError(err).Error(...)`
6. **Never log sensitive data**: Always sanitize passwords, tokens, connection strings
7. **Choose appropriate levels**: DEBUG (dev), INFO (normal), WARN (client error), ERROR (server error)

## Troubleshooting

### Logs Not Appearing

1. Check log level: `export LOG_LEVEL=debug`
2. Verify text mode: `export LOG_FORMAT=text` (for human-readable output)
3. Check context propagation: Ensure middleware chain is correct

### Missing request_id

Verify `RequestIDMiddleware` is registered before `RequestLoggingMiddleware`:

```go
mainRouter.Use(logger.RequestIDMiddleware)
mainRouter.Use(middleware.OTelMiddleware)
mainRouter.Use(logging.RequestLoggingMiddleware)
```

### Missing trace_id/span_id

1. Check OTel is enabled: `export OTEL_ENABLED=true`
2. Verify middleware order: `OTelMiddleware` must be after `RequestIDMiddleware`
3. Check sampling rate: `export OTEL_SAMPLING_RATE=1.0` (for testing)

### Data Not Masked

1. Check masking is enabled: `export MASKING_ENABLED=true`
2. Verify field names match configuration (case-insensitive)
3. Check JSON structure: Masking only works on top-level fields

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
LOG_LEVEL=debug OCM_ENV=integration_testing go test ./test/integration/...

# Run tests without OTel
OTEL_ENABLED=false OCM_ENV=integration_testing go test ./...
```

## References

- [slog Documentation](https://pkg.go.dev/log/slog)
- [OpenTelemetry Go SDK](https://opentelemetry.io/docs/languages/go/)
- [W3C Trace Context](https://www.w3.org/TR/trace-context/)
- [HyperFleet Architecture](https://github.com/openshift-hyperfleet/architecture)
