# HyperFleet API Logging

HyperFleet API uses Go's `log/slog` for structured, context-aware logging. The
shared [`hyperfleet-logger`](https://github.com/openshift-hyperfleet/hyperfleet-logger)
module owns JSON and text formatting, standard fields, context extraction,
control-character sanitization, and stack capture. This repository owns the API
integration around that handler: request IDs, HTTP attributes, request-header
masking, the GORM adapter, and the API's stack-trace policy.

## Features

- Structured JSON output for log aggregation and human-readable text output for
  local development
- A consistent `component=api`, application version, and OS/pod hostname on
  every record
- Request, trace, span, and resource correlation through `context.Context`
- UUIDv7 request IDs returned in the `X-Request-ID` response header
- Sensitive request-header redaction with a configurable header list
- OpenTelemetry trace propagation and configurable export and sampling
- GORM query, slow-query, and error records using the same logger
- Automatic stack traces on JSON error records, matching the previous JSON logger

## Architecture

The relevant components are:

| Component | Responsibility |
| --- | --- |
| `pkg/logger/handler.go` | Builds the API logger around `hyperfleet-logger`; defines component identity, sanitization, and stack policy |
| `pkg/logger/context.go` | Defines the API-owned `request_id` context field |
| `pkg/logger/requestid_middleware.go` | Generates request IDs and sets `X-Request-ID` |
| `pkg/logger/http.go` | Provides typed `slog.Attr` helpers for HTTP fields |
| `pkg/logger/gorm_logger.go` | Adapts GORM logging to `slog` |
| `pkg/middleware/otel.go` | Creates/continues HTTP spans and adds trace/span IDs to log context |
| `pkg/middleware/masking.go` | Redacts configured request headers |
| `cmd/hyperfleet-api/server/logging/request_logging_middleware.go` | Emits request-start and request-completion records |
| `pkg/telemetry/otel.go` | Configures the OpenTelemetry provider, exporter, sampler, and propagators |

The main API middleware is applied in this order:

```text
HTTP request
  -> RequestIDMiddleware
  -> OTelMiddleware (when tracing is enabled)
  -> RequestLoggingMiddleware
  -> API/auth/validation/transaction middleware
  -> Handler -> Service -> DAO
```

This order makes the request and trace identifiers available to request logs and
all downstream `slog.*Context` calls.

## Configuration

Application configuration uses this precedence, from highest to lowest:

1. CLI flag
2. Environment variable
3. Configuration file
4. Default

### Logging settings

| Setting | CLI flag | Environment variable | Default |
| --- | --- | --- | --- |
| Level | `--log-level`, `-l` | `HYPERFLEET_LOGGING_LEVEL` | `info` |
| Format | `--log-format`, `-f` | `HYPERFLEET_LOGGING_FORMAT` | `json` |
| Output | `--log-output` | `HYPERFLEET_LOGGING_OUTPUT` | `stdout` |
| Masking enabled | `--log-masking-enabled` | `HYPERFLEET_LOGGING_MASKING_ENABLED` | `true` |
| Sensitive headers | `--log-masking-sensitive-headers` | `HYPERFLEET_LOGGING_MASKING_HEADERS` | See [Data masking](#data-masking) |

Valid levels are `debug`, `info`, `warn`, and `error`. Valid formats are `json`
and `text`; valid outputs are `stdout` and `stderr`.

For local development:

```bash
HYPERFLEET_LOGGING_LEVEL=debug \
HYPERFLEET_LOGGING_FORMAT=text \
./bin/hyperfleet-api serve
```

Equivalent YAML configuration is:

```yaml
logging:
  level: info
  format: json
  output: stdout
  masking:
    enabled: true
    headers:
      - Authorization
      - X-API-Key
      - Cookie
```

Both `serve` and `migrate` install the loaded logging level, format, and output.
Before the configuration file is loaded, bootstrap records use the three
`HYPERFLEET_LOGGING_{LEVEL,FORMAT,OUTPUT}` variables, falling back to the normal
defaults. See the [configuration guide](config.md) for the complete application
configuration reference.

## Writing application logs

Use the standard `slog` context methods so correlation fields flow into every
record:

```go
import (
	"context"
	"log/slog"

	hfl "github.com/openshift-hyperfleet/hyperfleet-logger"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

func updateResource(ctx context.Context, kind, id, adapter string) {
	ctx = hfl.WithResourceType(ctx, kind)
	ctx = hfl.WithResourceID(ctx, id)
	ctx = logger.WithAdapter(ctx, adapter)

	slog.DebugContext(ctx, "Preparing resource update")
	slog.InfoContext(ctx, "Resource updated")
}
```

Derive a child context from the one received by the handler or service. Do not
replace it with `context.Background()`, because that discards request and trace
correlation. For recursive or batched work, derive a separate resource context
for each item so identifiers do not leak into sibling operations.

Use levels consistently:

- `DEBUG`: diagnostic detail normally disabled in production
- `INFO`: normal lifecycle events and handled client responses
- `WARN`: degraded behavior, retryable problems, slow queries, and fallbacks
- `ERROR`: server-side failures requiring attention

### Automatic fields

| Field | Source | When present |
| --- | --- | --- |
| `timestamp` | Shared handler | Every record |
| `level` | `slog.Record` | Every record |
| `message` | `slog.Record` | Every record |
| `component` | API handler factory | Every record; always `api` |
| `version` | Build metadata | Every record |
| `hostname` | `os.Hostname()` through the shared handler | Every record; `unknown` if discovery fails |
| `request_id` | API request-ID middleware | HTTP request and downstream records |
| `trace_id`, `span_id` | OpenTelemetry middleware | Traced HTTP request and downstream records |
| `resource_type`, `resource_id` | Shared context helpers | Operations whose service context has been enriched with a known resource |
| `adapter` | `logger.WithAdapter` | Adapter report processing, including validation, persistence, and aggregation |

`server.hostname` is the public server name and does not override the logging
hostname.

The HTTP status handlers derive a child context with the incoming adapter name
after decoding and validating the report, and pass it through conversion,
service calls, presentation, and error handling.
Concurrent reports retain their own `adapter` and `request_id` fields without
changing the caller's context. When aggregation inspects another adapter's
report, its diagnostic log derives a context with that adapter's name.

`RequestIDMiddleware` generates a UUIDv7 when its incoming context does not
already contain the API request-ID key. It preserves a request ID already placed
in that context and returns the selected value in `X-Request-ID`. It does not
trust an arbitrary inbound `X-Request-ID` header as the request ID.

### Temporary fields and helpers

Use structured attributes for values scoped to one record. Prefer constants
from `pkg/logger/fields.go` and HTTP helpers from `pkg/logger/http.go` when a
field is already defined:

```go
slog.InfoContext(ctx,
	"HTTP operation completed",
	logger.HTTPMethod(r.Method),
	logger.HTTPPath(r.URL.Path),
	logger.HTTPStatusCode(http.StatusOK),
	logger.HTTPDuration(elapsed),
)

slog.WarnContext(ctx,
	"Adapter report was ignored",
	logger.FieldAdapter, adapter,
	logger.FieldErrorCode, code,
)
```

Do not log passwords, credentials, bearer tokens, raw connection strings, or
complete objects that may contain secrets. Masking the HTTP request log is not a
substitute for selecting safe fields at application call sites.

## Output formats

### JSON

JSON is the default and is intended for log aggregation. The handler uses
lowercase level names and does not add a `source` field.

For this call:

```go
slog.InfoContext(ctx, "Resource updated", logger.FieldAdapter, "example")
```

a representative record is:

```json
{"timestamp":"2026-09-14T18:00:00Z","level":"info","message":"Resource updated","component":"api","version":"v1.2.3","hostname":"pod-abc","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7","resource_type":"Cluster","resource_id":"cluster-123","request_id":"0199458e-c331-7a22-8443-e25a91e95d81","adapter":"example"}
```

Only context fields actually present on the supplied context are emitted.

### Text

Text output has this shape:

```text
{timestamp} {LEVEL} [{component}] [{version}] [{hostname}] {message} {key=value}...
```

For example:

```text
2026-09-14T18:00:00Z INFO [api] [v1.2.3] [pod-abc] Resource updated trace_id=4bf92f3577b34da6a3ce929d0e0e4736 span_id=00f067aa0ba902b7 resource_type=Cluster resource_id=cluster-123 request_id=0199458e-c331-7a22-8443-e25a91e95d81 adapter=example
```

The API enables the shared handler's text sanitization. Newlines and other
control characters in messages and attribute values are escaped so
client-controlled values cannot forge additional log lines.

## Errors and stack traces

Stack capture preserves the behavior of the previous API handlers:

- In JSON format, every emitted record at `ERROR` or above includes `stack_trace`,
  regardless of the configured logging level.
- In text format, stack traces are not captured automatically.
- Records below `ERROR` never receive a captured stack trace. Handled HTTP client
  failures use `INFO` or `WARN`.

Log failures with their error value; no additional classification attribute is
required:

```go
slog.ErrorContext(ctx,
	"Database operation failed",
	"error", err,
)
```

Recovered startup panics separately include the recovered `panic_stack` captured
by `runtime/debug`, in either format.

## HTTP request logging

The request logger emits two `INFO` records for each API request except
`/healthcheck` (with or without a trailing slash):

- `HTTP request received` with `method`, `path`, `remote_addr`, `user_agent`,
  and masked request `headers`
- `HTTP request completed` with `method`, `path`, `status_code`, `duration_ms`,
  `remote_addr`, and `user_agent`

Because request ID and OpenTelemetry middleware run first, these records also
carry `request_id` and, when tracing is enabled, `trace_id` and `span_id`.

The middleware deliberately does not log request or response bodies. If code
has a justified need to log a body, it must call `MaskingMiddleware.MaskBody`
before logging it and should still prefer a small allowlist of safe fields.

## Data masking

Masking is enabled by default. The default sensitive headers are:

- `Authorization`
- `X-API-Key`
- `Cookie`
- `X-Auth-Token`
- `X-Forwarded-Authorization`

Header matching is case-insensitive. Sensitive header values are replaced by
`***REDACTED***` in the request-start record.

Body-field matching is case-insensitive and recursively traverses JSON objects
and arrays. For invalid or oversized JSON, `MaskBody` applies best-effort text
fallback masking.

To replace the header list through an environment variable, use a
comma-separated value:

```bash
export HYPERFLEET_LOGGING_MASKING_HEADERS='Authorization,Cookie,X-Custom-Auth'
```

Disabling masking makes logged request headers visible. Do so only in a
controlled environment with non-sensitive traffic.

## Database logging

The GORM adapter emits structured records using the request context, so query
records can include request, trace, and resource correlation:

| Event | Level | Fields |
| --- | --- | --- |
| Normal query | `INFO` | `duration_ms`, `rows`, `sql` |
| Query slower than 200 ms | `WARN` | `duration_ms`, `threshold_ms`, `rows`, `sql` |
| Query failure other than record-not-found | `ERROR` | `error`, `duration_ms`, `rows`, `sql` |

For the serving API, GORM verbosity is selected as follows:

| Configuration | GORM mode | Records offered to `slog` |
| --- | --- | --- |
| `database.debug=true` | Info | All queries, slow queries, and errors |
| Otherwise, `logging.level=debug` | Info | All queries, slow queries, and errors |
| Otherwise, `logging.level=info` or `warn` | Warn | Slow queries and errors |
| Otherwise, `logging.level=error` | Silent | No GORM records |

The global `slog` level still filters GORM records. For example,
`database.debug=true` with `logging.level=warn` does not make normal
`INFO`-level query records visible. Use `logging.level=debug` or `info` while
temporarily enabling database debug if all SQL is required.

```bash
HYPERFLEET_LOGGING_LEVEL=info \
HYPERFLEET_DATABASE_DEBUG=true \
./bin/hyperfleet-api serve
```

SQL records may include query arguments. Enable full query logging only for
short-lived diagnosis and avoid storing production query logs where sensitive
data could be exposed.

## OpenTelemetry integration

Tracing has its own application configuration, separate from logging:

```yaml
tracing:
  enabled: true
  service_name: hyperfleet-api
```

The application default is enabled. The Helm chart may supply a different value
through `HYPERFLEET_TRACING_ENABLED`. The former
`logging.otel.enabled` configuration key is accepted for compatibility but is
deprecated; `tracing.enabled` wins when both are set.

### Environment variables

| Variable | Purpose | Application default |
| --- | --- | --- |
| `HYPERFLEET_TRACING_ENABLED` | Enables provider setup and HTTP tracing middleware | `true` |
| `OTEL_SERVICE_NAME` | Service name attached to spans | `hyperfleet-api` |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector endpoint; absence selects the stdout trace exporter | unset |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `grpc` or `http/protobuf` | `grpc` |
| `OTEL_TRACES_SAMPLER` | Sampling strategy | `parentbased_traceidratio` |
| `OTEL_TRACES_SAMPLER_ARG` | Ratio from `0.0` to `1.0` for ratio samplers | `1.0` |
| `OTEL_PROPAGATORS` | Comma-separated propagation formats | `tracecontext,baggage` |
| `OTEL_RESOURCE_ATTRIBUTES` | Extra resource attributes as `key=value` pairs | unset |

Supported sampler values are `always_on`, `always_off`, `traceidratio`,
`parentbased_always_on`, `parentbased_always_off`, and
`parentbased_traceidratio`. An unknown sampler or invalid ratio falls back to
the default sampler/rate and emits a warning.

For a production collector with 10% root-span sampling:

```bash
export HYPERFLEET_TRACING_ENABLED=true
# Use HTTPS for TLS in production; HTTP is suitable for local development only.
export OTEL_EXPORTER_OTLP_ENDPOINT=https://otel-collector.example.com:4317
export OTEL_EXPORTER_OTLP_PROTOCOL=grpc
export OTEL_TRACES_SAMPLER=parentbased_traceidratio
export OTEL_TRACES_SAMPLER_ARG=0.1
```

The HTTP middleware extracts standard W3C trace context, creates or continues a
span, and places its IDs in logging context. Route templates are used for span
names when available to avoid high-cardinality names. The trace provider is
flushed during graceful shutdown.

If no OTLP endpoint is configured while tracing is enabled, spans are written
by the stdout trace exporter. Set an endpoint or disable tracing when that
additional stdout output is not wanted.

## Testing logging behavior

Construct an isolated logger backed by a buffer and assert decoded fields. Do
not compare complete records containing timestamps or generated request IDs.

```go
func TestResourceLog(t *testing.T) {
	var output bytes.Buffer
	log := logger.NewLogger("test-version", logger.HandlerConfig{
		Level:    slog.LevelInfo,
		Format:   hfl.FormatJSON,
		Output:   &output,
		Hostname: "test-host",
	})

	ctx := hfl.WithResourceType(context.Background(), "Cluster")
	ctx = hfl.WithResourceID(ctx, "cluster-123")
	ctx, err := logger.WithRequestID(ctx)
	if err != nil {
		t.Fatal(err)
	}
	log.InfoContext(ctx, "Resource updated")

	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatal(err)
	}
	if got := record["component"]; got != "api" {
		t.Fatalf("component = %v, want api", got)
	}
	if got := record["resource_id"]; got != "cluster-123" {
		t.Fatalf("resource_id = %v, want cluster-123", got)
	}
	if _, ok := record["request_id"].(string); !ok {
		t.Fatalf("request_id = %T, want string", record["request_id"])
	}
}
```

Tests that must replace the process-wide logger should restore it:

```go
previous := slog.Default()
slog.SetDefault(testLogger)
t.Cleanup(func() { slog.SetDefault(previous) })
```

Useful commands are:

```bash
HYPERFLEET_LOGGING_LEVEL=debug make test
HYPERFLEET_TRACING_ENABLED=false make test-integration
```

## Troubleshooting

### Logs do not appear

- Confirm that the record level is at or above `HYPERFLEET_LOGGING_LEVEL`.
- Confirm whether `HYPERFLEET_LOGGING_OUTPUT` sends records to `stdout` or
  `stderr`.
- Use text/debug mode locally to make filtering easier.

### `request_id` is missing

- Use a `slog.*Context` method with the request-derived context.
- Confirm `RequestIDMiddleware` wraps the handler that emits the record.
- Do not replace the request context with `context.Background()` downstream.

### `trace_id` or `span_id` is missing

- Confirm `HYPERFLEET_TRACING_ENABLED=true`.
- Confirm the log uses the context passed through `OTelMiddleware`.
- For local diagnosis, use `OTEL_TRACES_SAMPLER_ARG=1.0`.

### A sensitive header is visible

- Confirm `HYPERFLEET_LOGGING_MASKING_ENABLED=true`.
- Confirm the header appears in `HYPERFLEET_LOGGING_MASKING_HEADERS`; replacing
  this variable replaces the configured list.
- Remember that application log call sites outside request logging must choose
  and sanitize their own attributes.

### SQL queries do not appear

- Set `HYPERFLEET_LOGGING_LEVEL=debug` to enable and display normal queries.
- Alternatively, set `HYPERFLEET_DATABASE_DEBUG=true` with an overall level of
  `debug` or `info`.
- At `info` or `warn` without database debug, only queries slower than 200 ms
  and query errors are offered by GORM.
- At `error` without database debug, GORM logging is silent.

### Too many SQL queries appear

- Disable `HYPERFLEET_DATABASE_DEBUG`.
- Use `HYPERFLEET_LOGGING_LEVEL=info` for slow queries and errors without normal
  query records.
- Use `warn` to suppress other informational application records as well.

## References

- [Application configuration](config.md)
- [Go `log/slog` package](https://pkg.go.dev/log/slog)
- [OpenTelemetry Go documentation](https://opentelemetry.io/docs/languages/go/)
- [W3C Trace Context](https://www.w3.org/TR/trace-context/)
- [HyperFleet logging specification](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/standards/logging-specification.md)
- [HyperFleet tracing standard](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/standards/tracing.md)
