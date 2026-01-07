package servecmd

import (
	"context"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
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
		logger.Error(ctx, "Unable to add environment flags to serve command", "error", err)
		os.Exit(1)
	}

	return cmd
}

func runServe(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	// Initialize environment (loads configuration)
	err := environments.Environment().Initialize()
	if err != nil {
		logger.Error(ctx, "Unable to initialize environment", "error", err)
		os.Exit(1)
	}

	// Bind environment variables for advanced configuration (OTel, Masking)
	environments.Environment().Config.Logging.BindEnv()

	// Initialize slog logger
	initLogger()

	// Initialize OpenTelemetry (if enabled)
	if environments.Environment().Config.Logging.OTel.Enabled {
		samplingRate := environments.Environment().Config.Logging.OTel.SamplingRate
		tp, err := telemetry.InitTraceProvider("hyperfleet-api", api.Version, samplingRate)
		if err != nil {
			logger.Warn(ctx, "Failed to initialize OpenTelemetry", "error", err)
		} else {
			defer func() {
				if err := telemetry.Shutdown(context.Background(), tp); err != nil {
					logger.Error(ctx, "Failed to shutdown OpenTelemetry", "error", err)
				}
			}()
			logger.Info(ctx, "OpenTelemetry initialized", "sampling_rate", samplingRate)
		}
	} else {
		logger.Info(ctx, "OpenTelemetry disabled", "otel_enabled", false)
	}

	// Log configuration
	logger.Info(ctx, "Logger initialized",
		"log_level", environments.Environment().Config.Logging.Level,
		"log_format", environments.Environment().Config.Logging.Format,
		"log_output", environments.Environment().Config.Logging.Output,
		"masking_enabled", environments.Environment().Config.Logging.Masking.Enabled,
	)

	// Run the servers
	go func() {
		apiserver := server.NewAPIServer()
		apiserver.Start()
	}()

	go func() {
		metricsServer := server.NewMetricsServer()
		metricsServer.Start()
	}()

	go func() {
		healthcheckServer := server.NewHealthCheckServer()
		healthcheckServer.Start()
	}()

	select {}
}

// initLogger initializes the global slog logger from configuration
func initLogger() {
	ctx := context.Background()
	cfg := environments.Environment().Config.Logging

	// Parse log level
	level, err := logger.ParseLogLevel(cfg.Level)
	if err != nil {
		logger.Warn(ctx, "Invalid log level, using default", "level", cfg.Level, "error", err)
		level = slog.LevelInfo // Default to info
	}

	// Parse log format
	format, err := logger.ParseLogFormat(cfg.Format)
	if err != nil {
		logger.Warn(ctx, "Invalid log format, using default", "format", cfg.Format, "error", err)
		format = logger.FormatJSON // Default to JSON
	}

	// Parse log output
	output, err := logger.ParseLogOutput(cfg.Output)
	if err != nil {
		logger.Warn(ctx, "Invalid log output, using default", "output", cfg.Output, "error", err)
		output = os.Stdout // Default to stdout
	}

	// Get hostname
	hostname, _ := os.Hostname()

	// Create logger config
	logConfig := &logger.LogConfig{
		Level:     level,
		Format:    format,
		Output:    output,
		Component: "api",
		Version:   api.Version,
		Hostname:  hostname,
	}

	// Reconfigure global logger with environment config
	// Use ReconfigureGlobalLogger instead of InitGlobalLogger because
	// InitGlobalLogger was already called in main() with default config
	logger.ReconfigureGlobalLogger(logConfig)
}
