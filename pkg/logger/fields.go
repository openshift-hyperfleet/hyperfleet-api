package logger

// Temporary field name constants for structured logging
// These constants provide type safety and prevent typos when adding temporary fields to logs.
//
// Usage:
//   logger.With(ctx, logger.FieldBindAddress, addr).Info("Server starting")
//
// For high-frequency fields (>10 occurrences), use helper functions instead (e.g., WithError).

// Server/Config related fields
const (
	FieldBindAddress = "bind_address"
	FieldEnvironment = "environment"
	FieldLogLevel    = "level"
	FieldLogFormat   = "format"
	FieldLogOutput   = "output"
)

// Resource related fields
const (
	FieldNodePoolID = "nodepool_id"
	// Note: cluster_id, resource_type, resource_id are context fields (see context.go)
)

// Database related fields
const (
	FieldMigrationID = "migration_id"
	// FieldConnectionString - WARNING: Always sanitize connection strings before logging
	// to prevent exposing passwords. Never log raw connection strings.
	FieldConnectionString = "connection_string"
	FieldTable            = "table"
	FieldChannel          = "channel"
	// Note: transaction_id is a context field (see context.go)
)

// OpenTelemetry related fields
const (
	FieldOTelEnabled      = "otel_enabled"
	FieldSamplingRate     = "sampling_rate"
	FieldExporterEndpoint = "exporter_endpoint"
)

// Schema related fields
const (
	FieldSchemaPath = "schema_path"
)

// Generic fields
const (
	FieldAdapter   = "adapter"
	FieldErrorCode = "error_code"
	FieldFlag      = "flag"
	FieldData      = "data"
)

// Endpoint related fields (used in handlers)
const (
	FieldEndpoint = "endpoint"
)

// Note: HTTP-related field constants are defined in http.go
// Note: For error field, use WithError(ctx, err) helper function instead of FieldError constant
