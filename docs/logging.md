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
    logger.With(ctx, "host", "localhost", "error", err).Error("Database connection failed")

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

// Add persistent context fields
ctx = logger.WithClusterID(ctx, "cluster-123")
ctx = logger.WithResourceType(ctx, "managed-cluster")
ctx = logger.WithResourceID(ctx, "resource-456")
```

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
```
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
```
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

This section shows actual log output from HyperFleet API in key scenarios.

### Example 1: Basic Structured Logging

**Code**:
```go
logger.With(ctx, "port", 8080, "environment", "production").Info("Server started")
```

**JSON Output** (`LOG_FORMAT=json`):
```json
{
  "timestamp": "2026-01-09T12:30:45Z",
  "level": "info",
  "source": {
    "function": "main.startServer",
    "file": "/app/server.go",
    "line": 123
  },
  "message": "Server started",
  "port": 8080,
  "environment": "production",
  "component": "hyperfleet-api",
  "version": "v1.2.3",
  "hostname": "hyperfleet-api-pod-7f9c8b",
  "request_id": "2C9zKDz8xQMqF3yH"
}
```

**Text Output** (`LOG_FORMAT=text`):
```
2026-01-09T12:30:45Z INFO [hyperfleet-api] [v1.2.3] [hyperfleet-api-pod-7f9c8b] Server started request_id=2C9zKDz8xQMqF3yH port=8080 environment=production
```

### Example 2: OpenTelemetry Integration

**Configuration**: `OTEL_ENABLED=true`

**Code**:
```go
ctx = logger.WithClusterID(ctx, "cluster-abc123")
logger.With(ctx, "status", "ready").Info("Processing cluster update")
```

**JSON Output** (showing key fields):
```json
{
  "timestamp": "2026-01-09T12:30:45Z",
  "level": "info",
  "source": {"function": "main.updateCluster", "file": "/app/cluster.go", "line": 67},
  "message": "Processing cluster update",
  "status": "ready",
  "component": "hyperfleet-api",
  "version": "v1.2.3",
  "hostname": "pod-abc",
  "request_id": "2C9zKDz8xQMqF3yH",
  "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
  "span_id": "00f067aa0ba902b7",
  "cluster_id": "cluster-abc123"
}
```

**Text Output**:
```
2026-01-09T12:30:45Z INFO [hyperfleet-api] [v1.2.3] [pod-abc] Processing cluster update request_id=2C9zKDz8xQMqF3yH trace_id=4bf92f3577b34da6a3ce929d0e0e4736 span_id=00f067aa0ba902b7 cluster_id=cluster-abc123 status=ready
```

**Key additions**:
- `trace_id`: W3C trace context for distributed tracing
- `span_id`: Current operation span identifier

### Example 3: HTTP Request Correlation

**JSON Output** (request and response share same IDs):
```json
// Request log
{
  "level": "info",
  "message": "HTTP request received",
  "request_id": "3D8xLEy9yRNrG4zI",
  "trace_id": "5cg03g4688c45eb7b4df030e1f1f5847",
  "span_id": "11g178bb1cb013c8",
  "method": "POST",
  "path": "/api/hyperfleet/v1/clusters",
  "...": "standard fields omitted"
}

// Response log (same request_id, trace_id, span_id)
{
  "level": "info",
  "message": "HTTP request completed",
  "request_id": "3D8xLEy9yRNrG4zI",
  "trace_id": "5cg03g4688c45eb7b4df030e1f1f5847",
  "span_id": "11g178bb1cb013c8",
  "status_code": 201,
  "duration_ms": 445,
  "...": "standard fields omitted"
}
```

**Text Output**:
```
// Request log
2026-01-09T12:30:45Z INFO [hyperfleet-api] [v1.2.3] [pod-abc] HTTP request received request_id=3D8xLEy9yRNrG4zI trace_id=5cg03g4688c45eb7b4df030e1f1f5847 span_id=11g178bb1cb013c8 method=POST path=/api/hyperfleet/v1/clusters

// Response log (same request_id, trace_id, span_id)
2026-01-09T12:30:46Z INFO [hyperfleet-api] [v1.2.3] [pod-abc] HTTP request completed request_id=3D8xLEy9yRNrG4zI trace_id=5cg03g4688c45eb7b4df030e1f1f5847 span_id=11g178bb1cb013c8 status_code=201 duration_ms=445
```

**Key benefit**: Same `request_id`, `trace_id`, and `span_id` allow easy correlation between request and response logs.

### Example 4: Error Logs with Stack Traces

**Code**:
```go
logger.With(ctx, "host", "postgres.svc", "error", err).Error("Failed to connect to database")
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
```
2026-01-09T12:30:45Z ERROR [hyperfleet-api] [v1.2.3] [pod-abc] Failed to connect to database request_id=2C9zKDz8xQMqF3yH host=postgres.svc error="dial tcp: lookup postgres.svc: no such host"
  stack_trace:
    /workspace/pkg/db/connection.go:45 github.com/openshift-hyperfleet/hyperfleet-api/pkg/db.Connect
    /workspace/pkg/db/factory.go:78 github.com/openshift-hyperfleet/hyperfleet-api/pkg/db.NewSessionFactory
    /workspace/cmd/hyperfleet-api/servecmd/cmd.go:123 main.setupDatabase
```

**Note**: Error-level logs automatically include stack traces. Text format displays them on separate lines for easy reading.

### Example 5: Context Fields

**Code**:
```go
ctx = logger.WithClusterID(ctx, "cluster-prod-001")
ctx = logger.WithResourceType(ctx, "managed-cluster")
ctx = logger.WithResourceID(ctx, "mc-abc123")
logger.With(ctx, "duration_ms", 2500).Info("Resource provisioned")
```

**Output**:
```json
{
  "timestamp": "2026-01-09T12:30:45Z",
  "level": "info",
  "source": {"function": "main.provision", "file": "/app/provision.go", "line": 89},
  "message": "Resource provisioned",
  "duration_ms": 2500,
  "component": "hyperfleet-api",
  "version": "v1.2.3",
  "hostname": "pod-abc",
  "request_id": "2C9zKDz8xQMqF3yH",
  "cluster_id": "cluster-prod-001",
  "resource_type": "managed-cluster",
  "resource_id": "mc-abc123"
}
```

**Available context fields**: `request_id`, `trace_id`, `span_id`, `cluster_id`, `resource_type`, `resource_id`, `transaction_id`

### Example 6: Data Masking

**Configuration**: `MASKING_ENABLED=true`

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

## Real-World Usage Scenarios

### Local Development

**Configuration:**
```bash
export LOG_FORMAT=text
export LOG_LEVEL=debug
./hyperfleet-api serve
```

**Console Output:**
```
2026-01-09T12:30:45Z INFO [hyperfleet-api] [dev] [localhost] Server started port=8000
2026-01-09T12:30:46Z DEBUG [hyperfleet-api] [dev] [localhost] HTTP request received request_id=test-123 method=GET path=/health
2026-01-09T12:30:46Z INFO [hyperfleet-api] [dev] [localhost] HTTP request completed request_id=test-123 status_code=200 duration_ms=2
2026-01-09T12:30:47Z ERROR [hyperfleet-api] [dev] [localhost] Database query failed request_id=test-456 error="connection refused"
  stack_trace:
    /workspace/pkg/db/query.go:123 github.com/openshift-hyperfleet/hyperfleet-api/pkg/db.(*Database).Query
    /workspace/pkg/dao/cluster.go:45 github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao.(*ClusterDAO).List
```

### Production Kubernetes

**Deployment Configuration:**
```yaml
env:
  - name: LOG_FORMAT
    value: "json"
  - name: LOG_LEVEL
    value: "info"
  - name: OTEL_ENABLED
    value: "true"
```

**Pod Logs (kubectl logs):**
```json
{"timestamp":"2026-01-09T12:30:45Z","level":"info","message":"Server started","component":"hyperfleet-api","version":"v1.2.3","hostname":"hyperfleet-api-pod-abc123","request_id":"2C9zKDz8xQMqF3yH","port":8000}
{"timestamp":"2026-01-09T12:30:46Z","level":"info","message":"HTTP request received","component":"hyperfleet-api","version":"v1.2.3","hostname":"hyperfleet-api-pod-abc123","request_id":"3D8xLEy9yRNrG4zI","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7","method":"POST","path":"/api/hyperfleet/v1/clusters"}
```

## OpenTelemetry Integration

### Initialization

OpenTelemetry is initialized in `cmd/hyperfleet-api/servecmd/cmd.go`:

```go
if environments.Environment().Config.Logging.OTel.Enabled {
    samplingRate := environments.Environment().Config.Logging.OTel.SamplingRate
    tp, err := telemetry.InitTraceProvider(ctx, "hyperfleet-api", api.Version, samplingRate)
    if err != nil {
        logger.With(ctx, "error", err).Warn("Failed to initialize OpenTelemetry")
    } else {
        defer func() {
            if err := tp.Shutdown(context.Background()); err != nil {
                logger.With(ctx, "error", err).Error("Error shutting down tracer provider")
            }
        }()
        logger.With(ctx, "sampling_rate", samplingRate).Info("OpenTelemetry initialized")
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

### 1. Use Context-Aware Logging

✅ **Good**:
```go
logger.With(ctx, "user_id", userID).Info("User logged in")
```

❌ **Bad**:
```go
slog.Info("User logged in", "user_id", userID) // Missing context
```

### 2. Use Structured Fields with With()

✅ **Good**:
```go
logger.With(ctx, "cluster_id", clusterID, "region", region, "node_count", nodeCount).Info("Cluster created")
```

❌ **Bad**:
```go
logger.Info(ctx, fmt.Sprintf("Cluster %s created in %s with %d nodes",
    clusterID, region, nodeCount)) // String formatting - loses structure
```

### 3. Log Errors with Context

✅ **Good**:
```go
logger.With(ctx, "query", "SELECT * FROM clusters", "error", err, "duration_ms", elapsed.Milliseconds()).Error("Database query failed")
```

❌ **Bad**:
```go
logger.With(ctx, "error", err).Error("Error") // Vague message, no context
```

### 4. Choose Appropriate Log Levels

- **DEBUG**: Detailed diagnostic information (disabled in production)
- **INFO**: General informational messages (default)
- **WARN**: Warning messages for unexpected but handled conditions
- **ERROR**: Error messages for failures

### 5. Avoid Logging Sensitive Data

✅ **Good**:
```go
logger.With(ctx, "user_id", userID).Info("User authenticated")
```

❌ **Bad**:
```go
logger.With(ctx, "user_id", userID, "password_hash", hash).Info("User authenticated") // Don't log even hashed passwords
```

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
