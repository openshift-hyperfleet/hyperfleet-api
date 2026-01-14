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
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_session"
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
	err := environments.Environment().AddFlags(cmd.PersistentFlags())
	if err != nil {
		logger.WithError(ctx, err).Error("Unable to add environment flags to serve command")
		os.Exit(1)
	}

	return cmd
}

func runServe(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	// Bind database environment variables BEFORE Initialize (database is initialized inside)
	environments.Environment().Config.Database.BindEnv(cmd.PersistentFlags())

	err := environments.Environment().Initialize()
	if err != nil {
		logger.WithError(ctx, err).Error("Unable to initialize environment")
		os.Exit(1)
	}

	// Bind logging environment variables AFTER Initialize (logger is reconfigured later in initLogger)
	environments.Environment().Config.Logging.BindEnv(cmd.PersistentFlags())

	initLogger()

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

	healthcheckServer := server.NewHealthCheckServer()
	go healthcheckServer.Start()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info(ctx, "Shutdown signal received, starting graceful shutdown...")

	if err := healthcheckServer.Stop(); err != nil {
		logger.WithError(ctx, err).Error("Failed to stop healthcheck server")
	}
	if err := apiServer.Stop(); err != nil {
		logger.WithError(ctx, err).Error("Failed to stop API server")
	}
	if err := metricsServer.Stop(); err != nil {
		logger.WithError(ctx, err).Error("Failed to stop metrics server")
	}

	if tp != nil {
		if err := telemetry.Shutdown(context.Background(), tp); err != nil {
			logger.WithError(ctx, err).Error("Failed to shutdown OpenTelemetry")
		}
	}

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
		gormLevel := environments.Environment().Config.Database.GetGormLogLevel(
			environments.Environment().Config.Logging.Level,
		)
		if reconfigurable, ok := dbSessionFactory.(db_session.LoggerReconfigurable); ok {
			reconfigurable.ReconfigureLogger(gormLevel)
		}
	}
}
