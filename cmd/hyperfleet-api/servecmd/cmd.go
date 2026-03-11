package servecmd

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"go.opentelemetry.io/otel/sdk/trace"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_session"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/health"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/telemetry"
)

func NewServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the hyperfleet",
		Long:  "Serve the hyperfleet.",
		Run:   runServe,
	}

	// Add configuration system flags
	config.AddAllConfigFlags(cmd)

	return cmd
}

func runServe(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	// ============================================================
	// CONFIGURATION LOADING
	// ============================================================
	// Load configuration using Viper-based system
	loader := config.NewConfigLoader()
	cfg, err := loader.Load(ctx, cmd)
	if err != nil {
		logger.WithError(ctx, err).Error("Failed to load configuration")
		os.Exit(1)
	}

	// IMPORTANT: Set config BEFORE calling Initialize()
	// Initialize() will apply environment-specific overrides (e.g., development disables JWT/TLS)
	// and ensure SessionFactory, clients, services, handlers all use the correct config
	environments.Environment().Config = cfg

	// Initialize environment (applies overrides, creates SessionFactory, loads clients, services, handlers)
	err = environments.Environment().Initialize()
	if err != nil {
		logger.WithError(ctx, err).Error("Unable to initialize environment")
		os.Exit(1)
	}

	// Initialize logger with configured settings
	initLogger()

	// Log effective configuration (with sensitive values redacted)
	// This happens AFTER initLogger() so it uses the configured logger settings
	logger.Info(ctx, "Starting HyperFleet API with configuration (sensitive values redacted):")
	logger.Info(ctx, config.DumpConfig(environments.Environment().Config))

	var tp *trace.TracerProvider
	if environments.Environment().Config.Logging.OTel.Enabled {
		samplingRate := environments.Environment().Config.Logging.OTel.SamplingRate
		traceProvider, err := telemetry.InitTraceProvider(ctx, "hyperfleet-api", api.Version, samplingRate)
		if err != nil {
			logger.WithError(ctx, err).Warn("Failed to initialize OpenTelemetry")
		} else {
			tp = traceProvider
			logger.With(ctx, logger.FieldSamplingRate, samplingRate).Info("OpenTelemetry initialized")
		}
	} else {
		logger.With(ctx, logger.FieldOTelEnabled, false).Info("OpenTelemetry disabled")
	}

	logger.With(ctx,
		"log_level", environments.Environment().Config.Logging.Level,
		"log_format", environments.Environment().Config.Logging.Format,
		"log_output", environments.Environment().Config.Logging.Output,
		"masking_enabled", environments.Environment().Config.Logging.Masking.Enabled,
	).Info("Logger initialized")

	apiServer := server.NewAPIServer()
	go apiServer.Start()

	metricsServer := server.NewMetricsServer()
	go metricsServer.Start()

	healthServer := server.NewHealthServer()
	go healthServer.Start()

	// Wait for health server to be listening before marking as ready
	if notifier, ok := healthServer.(server.ListenNotifier); ok {
		<-notifier.NotifyListening()
	}

	// Mark application as ready to receive traffic
	health.GetReadinessState().SetReady()
	logger.Info(ctx, "Application ready to receive traffic")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info(ctx, "Shutdown signal received, starting graceful shutdown...")

	// Mark application as not ready (returns 503 on /readyz)
	health.GetReadinessState().SetShuttingDown()
	logger.Info(ctx, "Marked as not ready, draining in-flight requests...")

	if err := healthServer.Stop(); err != nil {
		logger.WithError(ctx, err).Error("Failed to stop health server")
	}
	if err := apiServer.Stop(); err != nil {
		logger.WithError(ctx, err).Error("Failed to stop API server")
	}
	if err := metricsServer.Stop(); err != nil {
		logger.WithError(ctx, err).Error("Failed to stop metrics server")
	}

	if tp != nil {
		shutdownCtx, cancel := context.WithTimeout(
			context.Background(), environments.Environment().Config.Health.ShutdownTimeout,
		)
		defer cancel()
		if err := telemetry.Shutdown(shutdownCtx, tp); err != nil {
			logger.WithError(ctx, err).Error("Failed to shutdown OpenTelemetry")
		}
	}

	// Close database connections
	environments.Environment().Teardown()

	logger.Info(ctx, "Graceful shutdown completed")
}

// initLogger initializes the global slog logger from configuration
func initLogger() {
	ctx := context.Background()
	cfg := environments.Environment().Config.Logging

	level, err := logger.ParseLogLevel(cfg.Level)
	if err != nil {
		logger.With(ctx, logger.FieldLogLevel, cfg.Level).WithError(err).Warn("Invalid log level, using default")
		level = slog.LevelInfo
	}

	format, err := logger.ParseLogFormat(cfg.Format)
	if err != nil {
		logger.With(ctx, logger.FieldLogFormat, cfg.Format).WithError(err).Warn("Invalid log format, using default")
		format = logger.FormatJSON
	}

	output, err := logger.ParseLogOutput(cfg.Output)
	if err != nil {
		logger.With(ctx, logger.FieldLogOutput, cfg.Output).WithError(err).Warn("Invalid log output, using default")
		output = os.Stdout
	}

	// Use configured hostname with fallback to os.Hostname()
	hostname := environments.Environment().Config.Server.Hostname
	if hostname == "" {
		hostname, _ = os.Hostname()
	}

	logConfig := &logger.LogConfig{
		Level:     level,
		Format:    format,
		Output:    output,
		Component: "api",
		Version:   api.Version,
		Hostname:  hostname,
	}

	// Use ReconfigureGlobalLogger instead of InitGlobalLogger because
	// InitGlobalLogger was already called in main() with default config
	logger.ReconfigureGlobalLogger(logConfig)

	// Reconfigure database logger to follow global logging level
	dbSessionFactory := environments.Environment().Database.SessionFactory
	if dbSessionFactory != nil {
		gormLevel := environments.Environment().Config.Database.SetLogLevel(
			environments.Environment().Config.Logging.Level,
		)
		if reconfigurable, ok := dbSessionFactory.(db_session.LoggerReconfigurable); ok {
			reconfigurable.ReconfigureLogger(gormLevel)
		}
	}
}
