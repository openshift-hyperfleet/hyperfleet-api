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
	ctx := context.Background()
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the hyperfleet",
		Long:  "Serve the hyperfleet.",
		Run:   runServe,
	}

	// Add new configuration system flags FIRST (for migration)
	// These will be used when HYPERFLEET_USE_NEW_CONFIG=true
	// Must be added before legacy system to allow flag name resolution
	config.AddAllConfigFlags(cmd)

	// Add legacy configuration flags (old system)
	// Note: Use cmd.Flags() instead of cmd.PersistentFlags() to match new system
	err := environments.Environment().AddFlags(cmd.Flags())
	if err != nil {
		logger.WithError(ctx, err).Error("Unable to add environment flags to serve command")
		os.Exit(1)
	}

	return cmd
}

func runServe(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	// ============================================================
	// CONFIGURATION LOADING WITH MIGRATION SUPPORT
	// ============================================================
	// Phase 1: Select configuration system and load config
	var finalConfig *config.ApplicationConfig
	var oldConfig *config.ApplicationConfig

	// For old config system: load flag values into config struct
	if !config.IsNewConfigEnabled() {
		if err := environments.Environment().Config.LoadFromFlags(cmd.Flags()); err != nil {
			logger.WithError(ctx, err).Error("Failed to load configuration from flags")
			os.Exit(1)
		}
	}

	if config.IsNewConfigEnabled() {
		logger.Info(ctx, "Using new Viper-based configuration system")

		// Load old config for comparison (without full initialization)
		oldConfig = environments.Environment().Config

		// Load configuration using new system
		loader := config.NewConfigLoader()
		newConfig, err := loader.Load(ctx, cmd)
		if err != nil {
			logger.WithError(ctx, err).Error("Failed to load configuration with new system")
			os.Exit(1)
		}

		// Apply environment-specific configuration overrides (e.g., development disables JWT/TLS)
		// This must happen BEFORE comparison to match old system behavior
		if err := environments.ApplyEnvironmentOverrides(newConfig); err != nil {
			logger.WithError(ctx, err).Error("Failed to apply environment overrides")
			os.Exit(1)
		}

		// Verify configuration equivalence for critical fields
		if err := config.VerifyConfigEquivalence(ctx, oldConfig, newConfig); err != nil {
			logger.WithError(ctx, err).Warn("Configuration equivalence check failed - differences detected")
			// Log but don't fail - allow new config to be used
		} else {
			logger.Info(ctx, "Configuration equivalence verified - old and new systems produce same config")
		}

		finalConfig = newConfig
	} else {
		logger.Info(ctx, "Using legacy configuration system (set HYPERFLEET_USE_NEW_CONFIG=true to use new system)")
		// Old system needs to read config from flags first
		// This is handled by Initialize() which calls ReadFiles() and LoadAdapters()
		finalConfig = environments.Environment().Config
	}

	// Phase 2: Set the selected configuration and initialize environment
	// IMPORTANT: Set config BEFORE calling Initialize() to avoid double initialization
	// and ensure SessionFactory, clients, services, handlers all use the correct config
	environments.Environment().Config = finalConfig

	// Initialize environment with the selected configuration
	// This creates SessionFactory, loads clients, services, and handlers
	err := environments.Environment().Initialize()
	if err != nil {
		logger.WithError(ctx, err).Error("Unable to initialize environment")
		os.Exit(1)
	}

	if config.IsNewConfigEnabled() {
		logger.Info(ctx, "Environment initialized successfully with new configuration")
	}

	// Phase 4: Initialize logger with configured settings
	initLogger()

	// Log effective configuration (with sensitive values redacted)
	// This happens AFTER initLogger() so it uses the configured logger settings
	logger.Debug(ctx, config.DumpConfig(environments.Environment().Config))

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

	hostname, _ := os.Hostname()

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

	// Reconfigure database logger to follow LOG_LEVEL
	dbSessionFactory := environments.Environment().Database.SessionFactory
	if dbSessionFactory != nil {
		gormLevel := environments.Environment().Config.Database.SetLogLevel()
		if reconfigurable, ok := dbSessionFactory.(db_session.LoggerReconfigurable); ok {
			reconfigurable.ReconfigureLogger(gormLevel)
		}
	}
}
