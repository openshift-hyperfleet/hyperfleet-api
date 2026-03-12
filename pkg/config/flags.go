package config

import (
	"github.com/spf13/cobra"
)

// AddConfigFlags adds the --config flag to the command
// This flag has highest priority in config file resolution
func AddConfigFlag(cmd *cobra.Command) {
	cmd.PersistentFlags().String(
		"config", "",
		"Path to configuration file (YAML format)",
	)
}

// AddServerFlags adds server configuration flags following standard naming
// Format: --server-<field> maps to HYPERFLEET_SERVER_<FIELD> and server.<field>
func AddServerFlags(cmd *cobra.Command) {
	defaults := NewServerConfig()

	cmd.Flags().String("server-hostname", defaults.Hostname, "Server's public hostname")
	cmd.Flags().String("server-host", defaults.Host, "Server bind host")
	cmd.Flags().Int("server-port", defaults.Port, "Server bind port")
	cmd.Flags().String("server-openapi-schema-path", defaults.OpenAPISchemaPath,
		"Path to OpenAPI schema for spec validation")
	cmd.Flags().Duration("server-read-timeout", defaults.Timeouts.Read, "HTTP server read timeout")
	cmd.Flags().Duration("server-write-timeout", defaults.Timeouts.Write, "HTTP server write timeout")
	cmd.Flags().String("server-https-cert-file", defaults.TLS.CertFile, "Path to TLS certificate file")
	cmd.Flags().String("server-https-key-file", defaults.TLS.KeyFile, "Path to TLS key file")
	cmd.Flags().Bool("server-https-enabled", defaults.TLS.Enabled, "Enable HTTPS rather than HTTP")
	cmd.Flags().Bool("server-jwt-enabled", defaults.JWT.Enabled, "Enable JWT authentication")
	cmd.Flags().Bool("server-authz-enabled", defaults.Authz.Enabled, "Enable authorization on endpoints")
	cmd.Flags().String("server-jwk-cert-file", defaults.JWK.CertFile, "JWK certificate file path")
	cmd.Flags().String("server-jwk-cert-url", defaults.JWK.CertURL, "JWK certificate URL")
	cmd.Flags().String("server-acl-file", defaults.ACL.File, "Access control list file path")
}

// AddDatabaseFlags adds database configuration flags following standard naming
// Format: --db-<field> maps to HYPERFLEET_DATABASE_<FIELD> and database.<field>
func AddDatabaseFlags(cmd *cobra.Command) {
	defaults := NewDatabaseConfig()

	cmd.Flags().String("db-host", "", "Database host")
	cmd.Flags().Int("db-port", 0, "Database port")
	cmd.Flags().String("db-username", "", "Database username")
	cmd.Flags().String("db-password", "", "Database password (prefer env var for security)")
	cmd.Flags().String("db-name", "", "Database name")
	cmd.Flags().String("db-dialect", defaults.Dialect, "Database dialect (postgres, mysql)")
	cmd.Flags().String("db-ssl-mode", defaults.SSL.Mode, "SSL mode (disable, require, verify-ca, verify-full)")
	cmd.Flags().Bool("db-debug", defaults.Debug, "Enable database debug mode")
	cmd.Flags().Int("db-max-open-connections", defaults.Pool.MaxConnections, "Maximum open database connections")
	cmd.Flags().String("db-root-cert-file", defaults.SSL.RootCertFile, "Database root certificate file")
}

// AddLoggingFlags adds logging configuration flags following standard naming
// Format: --log-<field> maps to HYPERFLEET_LOGGING_<FIELD> and logging.<field>
func AddLoggingFlags(cmd *cobra.Command) {
	defaults := NewLoggingConfig()

	cmd.Flags().StringP("log-level", "l", defaults.Level, "Log level (debug, info, warn, error)")
	cmd.Flags().StringP("log-format", "f", defaults.Format, "Log format (json, text)")
	cmd.Flags().String("log-output", defaults.Output, "Log output (stdout, stderr)")
	cmd.Flags().Bool("log-otel-enabled", defaults.OTel.Enabled, "Enable OpenTelemetry tracing")
	cmd.Flags().Float64("log-otel-sampling-rate", defaults.OTel.SamplingRate, "OpenTelemetry sampling rate (0.0-1.0)")
	cmd.Flags().Bool("log-masking-enabled", defaults.Masking.Enabled, "Enable log masking for sensitive data")
	cmd.Flags().String("log-masking-sensitive-headers", defaults.GetSensitiveHeadersString(),
		"Comma-separated list of sensitive HTTP headers to mask")
	cmd.Flags().String("log-masking-sensitive-fields", defaults.GetSensitiveFieldsString(),
		"Comma-separated list of sensitive fields to mask")
}

// AddMetricsFlags adds metrics configuration flags following standard naming
// Format: --metrics-<field> maps to HYPERFLEET_METRICS_<FIELD> and metrics.<field>
func AddMetricsFlags(cmd *cobra.Command) {
	defaults := NewMetricsConfig()

	cmd.Flags().String("metrics-host", defaults.Host, "Metrics server bind host")
	cmd.Flags().Int("metrics-port", defaults.Port, "Metrics server bind port")
	cmd.Flags().Bool("metrics-tls-enabled", defaults.TLS.Enabled, "Enable TLS for metrics server")
	cmd.Flags().String("metrics-tls-cert-file", defaults.TLS.CertFile, "Path to TLS certificate file for metrics")
	cmd.Flags().String("metrics-tls-key-file", defaults.TLS.KeyFile, "Path to TLS key file for metrics")
	cmd.Flags().Duration("metrics-label-metrics-inclusion-duration", defaults.LabelMetricsInclusionDuration,
		"Duration for cluster telemetry label inclusion")
}

// AddHealthFlags adds health check configuration flags following standard naming
// Format: --health-<field> maps to HYPERFLEET_HEALTH_<FIELD> and health.<field>
func AddHealthFlags(cmd *cobra.Command) {
	defaults := NewHealthConfig()

	cmd.Flags().String("health-host", defaults.Host, "Health check server bind host")
	cmd.Flags().Int("health-port", defaults.Port, "Health check server bind port")
	cmd.Flags().Bool("health-tls-enabled", defaults.TLS.Enabled, "Enable TLS for health server")
	cmd.Flags().String("health-tls-cert-file", defaults.TLS.CertFile, "Path to TLS certificate file for health")
	cmd.Flags().String("health-tls-key-file", defaults.TLS.KeyFile, "Path to TLS key file for health")
	cmd.Flags().Duration("health-shutdown-timeout", defaults.ShutdownTimeout, "Graceful shutdown timeout")
	cmd.Flags().Duration("health-db-ping-timeout", defaults.DBPingTimeout, "Database ping timeout")
}

// AddOCMFlags adds OCM configuration flags following standard naming
// Format: --ocm-<field> maps to HYPERFLEET_OCM_<FIELD> and ocm.<field>
func AddOCMFlags(cmd *cobra.Command) {
	defaults := NewOCMConfig()

	cmd.Flags().String("ocm-base-url", defaults.BaseURL, "OCM API base URL")
	cmd.Flags().String("ocm-token-url", defaults.TokenURL, "OCM token URL")
	cmd.Flags().String("ocm-client-id", "", "OCM client ID (prefer env var for security)")
	cmd.Flags().String("ocm-client-secret", "", "OCM client secret (prefer env var for security)")
	cmd.Flags().String("ocm-self-token", "", "OCM self token (prefer env var for security)")
	cmd.Flags().Bool("ocm-debug", defaults.Debug, "Enable OCM debug mode")
	cmd.Flags().Bool("ocm-mock-enabled", defaults.Mock.Enabled, "Enable mock OCM clients")
}

// AddAllConfigFlags adds all configuration flags to the command
// This is a convenience function that adds all flag groups
//
// Note: Adapter configuration is handled via environment variables (JSON arrays):
//   - HYPERFLEET_ADAPTERS_REQUIRED_CLUSTER: Required cluster adapters
//   - HYPERFLEET_ADAPTERS_REQUIRED_NODEPOOL: Required nodepool adapters
//
// No CLI flags are provided for adapters as they are complex types (arrays)
func AddAllConfigFlags(cmd *cobra.Command) {
	AddConfigFlag(cmd)
	AddServerFlags(cmd)
	AddDatabaseFlags(cmd)
	AddLoggingFlags(cmd)
	AddMetricsFlags(cmd)
	AddHealthFlags(cmd)
	AddOCMFlags(cmd)
}
