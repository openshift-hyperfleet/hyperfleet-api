# HyperFleet API Logging

This document describes the logging system used in hyperfleet-api.

## Overview

HyperFleet API uses Go's standard library `log/slog` for structured logging with the following features:

- **Structured logging**: All logs use key-value pairs for better queryability
- **Context-aware logging**: Automatic operation_id, trace_id, and span_id propagation
- **Data masking**: Sensitive data redaction in headers and JSON payloads
- **OpenTelemetry integration**: Distributed tracing with configurable sampling
- **JSON and text output**: Machine-parseable JSON or human-readable text format
- **Custom handler**: Automatic component, version, and hostname fields

## Architecture

### Components

1. **pkg/logger/logger.go**: Core logger with HyperFleetHandler (custom slog.Handler)
2. **pkg/logger/context.go**: Context key definitions for trace_id, span_id, cluster_id, etc.
3. **pkg/logger/operationid_middleware.go**: Operation ID generation and middleware
4. **pkg/logger/ocm_bridge.go**: OCMLogger interface for backward compatibility
5. **pkg/middleware/otel.go**: OpenTelemetry trace context extraction
6. **pkg/telemetry/otel.go**: OpenTelemetry trace provider initialization

### Middleware Chain

```text
HTTP Request
  ↓
OperationIDMiddleware (adds operation_id to context)
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
export LOGGING_LEVEL=info

# JSON output (true/false)
export LOGGING_JSON=true

# Enable OpenTelemetry (true/false)
export LOGGING_OTEL_ENABLED=true

# OpenTelemetry sampling rate (0.0 to 1.0)
# 0.0 = no traces, 1.0 = all traces
export LOGGING_OTEL_SAMPLING_RATE=0.1

# Data masking (true/false)
export LOGGING_MASKING_ENABLED=true

# Headers to mask (comma-separated)
export LOGGING_MASKING_HEADERS="Authorization,Cookie,X-API-Key"

# JSON body fields to mask (comma-separated)
export LOGGING_MASKING_BODY_FIELDS="password,token,secret,api_key"
```

### Configuration Struct

```go
type LoggingConfig struct {
    Level   string         `env:"LOGGING_LEVEL" envDefault:"info"`
    JSON    bool           `env:"LOGGING_JSON" envDefault:"true"`
    OTel    OTelConfig     `env:",prefix=LOGGING_OTEL_"`
    Masking MaskingConfig  `env:",prefix=LOGGING_MASKING_"`
}

type OTelConfig struct {
    Enabled      bool    `env:"ENABLED" envDefault:"true"`
    SamplingRate float64 `env:"SAMPLING_RATE" envDefault:"0.1"`
}

type MaskingConfig struct {
    Enabled    bool     `env:"ENABLED" envDefault:"true"`
    Headers    []string `env:"HEADERS" envSeparator:","`
    BodyFields []string `env:"BODY_FIELDS" envSeparator:","`
}
```

## Usage

### Basic Logging

Always use context-aware logging to include automatic fields (operation_id, trace_id, span_id):

```go
import (
    "context"
    "github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

func MyHandler(w http.ResponseWriter, r *http.Request) {
    ctx := r.Context()

    // Info level
    logger.Info(ctx, "Processing cluster creation",
        "cluster_id", clusterID,
        "region", region)

    // Error level (automatically includes stack trace)
    logger.Error(ctx, "Database connection failed",
        "host", "localhost",
        "error", err)

    // Debug level
    logger.Debug(ctx, "Cache hit", "key", "cluster:123")

    // Warning level
    logger.Warn(ctx, "High memory usage",
        "used_mb", 1024,
        "threshold_mb", 800)
}
```

Logs automatically include:
- `component`: "hyperfleet-api"
- `version`: Application version
- `hostname`: Pod/host name
- `operation_id`: Unique request identifier
- `trace_id`: W3C trace ID (when OTel enabled)
- `span_id`: Current span ID (when OTel enabled)

### Available Functions

```go
// Standard logging functions
logger.Info(ctx, msg, "key", "value")
logger.Warn(ctx, msg, "key", "value")
logger.Error(ctx, msg, "key", "value")
logger.Debug(ctx, msg, "key", "value")

// Formatted logging functions (prefer structured logging above)
logger.Infof(ctx, "Processing %d items", count)
logger.Warnf(ctx, "Retry %d/%d", attempt, maxRetries)
logger.Errorf(ctx, "Failed: %v", err)

// Add context fields
ctx = logger.WithClusterID(ctx, "cluster-123")
ctx = logger.WithResourceType(ctx, "managed-cluster")
ctx = logger.WithResourceID(ctx, "resource-456")
```

## Log Output Examples

This section shows actual log output from HyperFleet API in key scenarios.

### Example 1: Basic Structured Logging

**Code**:
```go
logger.Info(ctx, "Server started", "port", 8080, "environment", "production")
```

**JSON Output** (`LOGGING_JSON=true`):
```json
{
  "timestamp": "2026-01-07T18:30:45.123Z",
  "level": "info",
  "message": "Server started",
  "component": "hyperfleet-api",
  "version": "v1.2.3",
  "hostname": "hyperfleet-api-pod-7f9c8b",
  "operation_id": "op-2C9zKDz8xQMqF3yH",
  "port": 8080,
  "environment": "production"
}
```

**Text Output** (`LOGGING_JSON=false`):
```text
time=2026-01-07T18:30:45.123Z level=INFO msg="Server started" component=hyperfleet-api version=v1.2.3 hostname=hyperfleet-api-pod-7f9c8b operation_id=op-2C9zKDz8xQMqF3yH port=8080 environment=production
```

### Example 2: OpenTelemetry Integration

**Configuration**: `LOGGING_OTEL_ENABLED=true`

**Code**:
```go
ctx = logger.WithClusterID(ctx, "cluster-abc123")
logger.Info(ctx, "Processing cluster update", "status", "ready")
```

**Output** (showing only new fields):
```json
{
  "timestamp": "2026-01-07T18:30:45.456Z",
  "level": "info",
  "message": "Processing cluster update",
  "operation_id": "op-2C9zKDz8xQMqF3yH",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "span_id": "00f067aa0ba902b7",
  "cluster_id": "cluster-abc123",
  "status": "ready",
  "...": "standard fields omitted"
}
```

**Key additions**:
- `trace_id`: W3C trace context for distributed tracing
- `span_id`: Current operation span identifier

### Example 3: HTTP Request Correlation

**Output** (request and response share same IDs):
```json
// Request log
{
  "level": "info",
  "message": "HTTP request received",
  "operation_id": "op-3D8xLEy9yRNrG4zI",
  "trace_id": "5cg03g4688c45eb7b4df030e1f1f5847",
  "span_id": "11g178bb1cb013c8",
  "method": "POST",
  "path": "/api/hyperfleet/v1/clusters",
  "...": "standard fields omitted"
}

// Response log (same operation_id, trace_id, span_id)
{
  "level": "info",
  "message": "HTTP request completed",
  "operation_id": "op-3D8xLEy9yRNrG4zI",
  "trace_id": "5cg03g4688c45eb7b4df030e1f1f5847",
  "span_id": "11g178bb1cb013c8",
  "status_code": 201,
  "duration_ms": 445,
  "...": "standard fields omitted"
}
```

### Example 4: Error Logs with Stack Traces

**Code**:
```go
logger.Error(ctx, "Failed to connect to database", "host", "postgres.svc", "error", err)
```

**Output**:
```json
{
  "level": "error",
  "message": "Failed to connect to database",
  "host": "postgres.svc",
  "error": "dial tcp: lookup postgres.svc: no such host",
  "stack_trace": [
    "/workspace/pkg/db/connection.go:45 github.com/openshift-hyperfleet/hyperfleet-api/pkg/db.Connect",
    "/workspace/pkg/db/factory.go:78 github.com/openshift-hyperfleet/hyperfleet-api/pkg/db.NewSessionFactory",
    "/workspace/cmd/hyperfleet-api/servecmd/cmd.go:123 main.setupDatabase"
  ],
  "...": "standard fields omitted"
}
```

**Note**: Error-level logs automatically include stack traces (15 frames max, filtered for relevance).

### Example 5: Context Fields

**Code**:
```go
ctx = logger.WithClusterID(ctx, "cluster-prod-001")
ctx = logger.WithResourceType(ctx, "managed-cluster")
ctx = logger.WithResourceID(ctx, "mc-abc123")
logger.Info(ctx, "Resource provisioned", "duration_ms", 2500)
```

**Output**:
```json
{
  "level": "info",
  "message": "Resource provisioned",
  "cluster_id": "cluster-prod-001",
  "resource_type": "managed-cluster",
  "resource_id": "mc-abc123",
  "duration_ms": 2500,
  "...": "standard fields omitted"
}
```

**Available context fields**: `operation_id`, `trace_id`, `span_id`, `cluster_id`, `resource_type`, `resource_id`

### Example 6: Data Masking

**Configuration**: `LOGGING_MASKING_ENABLED=true`

**Request body**:
```json
{"username": "alice@example.com", "password": "SuperSecret123!"}
```

**Logged output**:
```json
{
  "level": "info",
  "message": "HTTP request received",
  "path": "/api/hyperfleet/v1/auth/login",
  "headers": {
    "Authorization": "***REDACTED***"
  },
  "body": {
    "username": "alice@example.com",
    "password": "***REDACTED***"
  },
  "...": "standard fields omitted"
}
```

**Security**: Always enable masking in production to prevent credential leakage

## OpenTelemetry Integration

### Initialization

OpenTelemetry is initialized in `cmd/hyperfleet-api/servecmd/cmd.go`:

```go
if environments.Environment().Config.Logging.OTel.Enabled {
    samplingRate := environments.Environment().Config.Logging.OTel.SamplingRate
    tp, err := telemetry.InitTraceProvider("hyperfleet-api", api.Version, samplingRate)
    if err != nil {
        slog.Error("Failed to initialize OpenTelemetry", "error", err)
    } else {
        defer func() {
            if err := tp.Shutdown(context.Background()); err != nil {
                slog.Error("Error shutting down tracer provider", "error", err)
            }
        }()
        slog.Info("OpenTelemetry initialized", "sampling_rate", samplingRate)
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

Sensitive data is automatically masked when `LOGGING_MASKING_ENABLED=true`:

**Default masked headers**: `Authorization`, `Cookie`, `X-API-Key`, `X-Auth-Token`
**Default masked fields**: `password`, `token`, `secret`, `api_key`, `client_secret`

To add custom masking rules:

```go
env().Config.Logging.Masking.Headers = append(
    env().Config.Logging.Masking.Headers,
    "X-Custom-Auth-Header",
)

env().Config.Logging.Masking.BodyFields = append(
    env().Config.Logging.Masking.BodyFields,
    "credit_card",
    "ssn",
)
```

## Best Practices

### 1. Use Context-Aware Logging

✅ **Good**:
```go
logger.Info(ctx, "User logged in", "user_id", userID)
```

❌ **Bad**:
```go
slog.Info("User logged in", "user_id", userID) // Missing context
```

### 2. Use Key-Value Pairs

✅ **Good**:
```go
logger.Info(ctx, "Cluster created",
    "cluster_id", clusterID,
    "region", region,
    "node_count", nodeCount)
```

❌ **Bad**:
```go
logger.Info(ctx, fmt.Sprintf("Cluster %s created in %s with %d nodes",
    clusterID, region, nodeCount)) // String formatting
```

### 3. Log Errors with Context

✅ **Good**:
```go
logger.Error(ctx, "Database query failed",
    "query", "SELECT * FROM clusters",
    "error", err,
    "duration_ms", elapsed.Milliseconds())
```

❌ **Bad**:
```go
logger.Error(ctx, "Error", "error", err) // No context
```

### 4. Choose Appropriate Log Levels

- **DEBUG**: Detailed diagnostic information (disabled in production)
- **INFO**: General informational messages (default)
- **WARN**: Warning messages for unexpected but handled conditions
- **ERROR**: Error messages for failures

### 5. Avoid Logging Sensitive Data

✅ **Good**:
```go
logger.Info(ctx, "User authenticated", "user_id", userID)
```

❌ **Bad**:
```go
logger.Info(ctx, "User authenticated",
    "user_id", userID,
    "password_hash", hash) // Don't log even hashed passwords
```

## Troubleshooting

### Logs Not Appearing

1. Check log level: `export LOGGING_LEVEL=debug`
2. Verify JSON mode: `export LOGGING_JSON=false` (for human-readable output)
3. Check context propagation: Ensure middleware chain is correct

### Missing operation_id

Verify `OperationIDMiddleware` is registered before `RequestLoggingMiddleware`:

```go
mainRouter.Use(logger.OperationIDMiddleware)
mainRouter.Use(middleware.OTelMiddleware)
mainRouter.Use(logging.RequestLoggingMiddleware)
```

### Missing trace_id/span_id

1. Check OTel is enabled: `export LOGGING_OTEL_ENABLED=true`
2. Verify middleware order: `OTelMiddleware` must be after `OperationIDMiddleware`
3. Check sampling rate: `export LOGGING_OTEL_SAMPLING_RATE=1.0` (for testing)

### Data Not Masked

1. Check masking is enabled: `export LOGGING_MASKING_ENABLED=true`
2. Verify field names match configuration (case-insensitive)
3. Check JSON structure: Masking only works on top-level fields

## Testing

### Unit Tests

```go
func TestLogging(t *testing.T) {
    // Create context with operation ID
    ctx := logger.WithOperationID(context.Background(), "test-op-123")

    // Log with context
    logger.Info(ctx, "Test message", "key", "value")

    // Verify operation_id is included
    // (Use a test handler to capture logs)
}
```

### Integration Tests

```bash
# Run tests with debug logging
LOGGING_LEVEL=debug OCM_ENV=integration_testing go test ./test/integration/...

# Run tests without OTel
LOGGING_OTEL_ENABLED=false OCM_ENV=integration_testing go test ./...
```

## References

- [slog Documentation](https://pkg.go.dev/log/slog)
- [OpenTelemetry Go SDK](https://opentelemetry.io/docs/languages/go/)
- [W3C Trace Context](https://www.w3.org/TR/trace-context/)
- [HyperFleet Architecture](https://github.com/openshift-hyperfleet/architecture)
