package logger

// Temporary field name constants for structured logging
// These constants provide type safety and prevent typos when adding temporary fields to logs.
//
// Usage:
//   slog.InfoContext(ctx, "Server starting", logger.FieldBindAddress, addr)
//
// For high-frequency fields (>10 occurrences), use the shared context helpers instead.

// Server/Config related fields
const (
	FieldBindAddress = "bind_address"
	FieldLogLevel    = "level"
	FieldLogFormat   = "format"
	FieldLogOutput   = "output"
)

// Database related fields
const (
	// FieldConnectionString - WARNING: Always sanitize connection strings before logging
	// to prevent exposing passwords. Never log raw connection strings.
	FieldConnectionString = "connection_string"
	FieldChannel          = "channel"
	FieldLockID           = "lock_id"
	FieldLockType         = "lock_type"
	FieldLockDurationMs   = "lock_duration_ms"
	// Note: transaction_id is a context field (see context.go)
)

// OpenTelemetry related fields
const (
	FieldOTelEnabled    = "otel_enabled"
	FieldSamplingRate   = "sampling_rate"
	FieldServiceName    = "service_name"
	FieldProtocol       = "protocol"
	FieldSampler        = "sampler"
	FieldServiceVersion = "service_version"
)

// Schema related fields
const (
	FieldSchemaPath = "schema_path"
)

// Generic fields
const (
	FieldRequestID = "request_id"
	FieldAdapter   = "adapter"
	FieldErrorCode = "error_code"
	FieldData      = "data"
)

// Note: HTTP-related field constants are defined in http.go
