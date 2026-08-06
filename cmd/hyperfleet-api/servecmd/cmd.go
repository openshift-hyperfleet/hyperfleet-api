package servecmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/container"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/closer"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_session"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/health"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/metrics"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/telemetry"
)

const (
	// Fixed drain budgets for lightweight servers and OTel flush.
	// terminationGracePeriodSeconds must be > preStop (5s) + shutdown_timeout
	// + metricsDrainTimeout + healthDrainTimeout + otelFlushTimeout.
	metricsDrainTimeout = 2 * time.Second
	healthDrainTimeout  = 2 * time.Second
	otelFlushTimeout    = 5 * time.Second
)

func NewServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the hyperfleet",
		Long:  "Serve the hyperfleet.",
		RunE:  runServe,
		// runServe errors are runtime failures, not CLI misuse - don't dump usage on them.
		SilenceUsage: true,
	}

	// Add configuration system flags
	config.AddAllConfigFlags(cmd)

	return cmd
}

func runServe(cmd *cobra.Command, args []string) (runErr error) {
	ctx := cmd.Context()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)

	loader := config.NewConfigLoader()
	cfg, err := loader.Load(ctx, cmd)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	registry.LoadDescriptors(cfg.Entities)
	registry.Validate()

	c := closer.New()
	defer func() {
		closeErr := c.Close()
		runErr = errors.Join(runErr, closeErr)
		if runErr == nil {
			logger.Info(context.Background(), "Graceful shutdown completed")
		}
	}()

	ctr := container.NewContainer(cfg, c)

	initLogger(cfg)

	sf := ctr.SessionFactory()
	configureDBLogger(cfg, sf)

	logger.Info(ctx, "Starting HyperFleet API with configuration (sensitive values redacted):")
	logger.Info(ctx, config.DumpConfig(cfg))

	// OTel registered first so it flushes last - teardown spans are preserved.
	if cfg.Tracing.Enabled {
		traceProvider, traceErr := telemetry.InitTraceProvider(ctx, cfg.Tracing.ServiceName, api.Version)
		if traceErr != nil {
			logger.WithError(ctx, traceErr).Warn("Failed to initialize OpenTelemetry")
		} else {
			logger.With(ctx, logger.FieldServiceName, cfg.Tracing.ServiceName).Info("OpenTelemetry initialized")
			c.Add(func() error {
				flushCtx, cancel := context.WithTimeout(context.Background(), otelFlushTimeout)
				defer cancel()
				return telemetry.Shutdown(flushCtx, traceProvider)
			})
		}
	} else {
		logger.With(ctx, logger.FieldOTelEnabled, false).Info("OpenTelemetry disabled")
	}

	logger.With(ctx,
		"log_level", cfg.Logging.Level,
		"log_format", cfg.Logging.Format,
		"log_output", cfg.Logging.Output,
		"masking_enabled", cfg.Logging.Masking.Enabled,
	).Info("Logger initialized")

	if collectorErr := metrics.RegisterReconciliationCollector(
		ctr.SessionFactory().DirectDB(),
		cfg.Metrics.ReconciliationStuckThreshold,
	); collectorErr != nil {
		logger.WithError(ctx, collectorErr).Error("Failed to register reconciliation collector")
	}

	jwtHandler := ctr.JWTHandler()

	apiServer, err := BuildAPIServer(
		cfg,
		ctr.ResourceService(),
		ctr.AdapterStatusService(),
		ctr.SchemaValidator(),
		jwtHandler,
		ctr.SessionFactory(),
	)
	if err != nil {
		return fmt.Errorf("build API server: %w", err)
	}
	// Do NOT register srv.Close bare - it severs in-flight requests without
	// draining. addDrain uses Shutdown with a budget, falling back to Close.
	addDrain(c, apiServer, cfg.Health.ShutdownTimeout)

	metricsServer := server.NewMetricsServer(cfg.Metrics)
	addDrain(c, metricsServer, metricsDrainTimeout)

	healthServer := server.NewHealthServer(cfg.Health, ctr.SessionFactory())
	addDrain(c, healthServer, healthDrainTimeout)

	// Readyz registered last so it runs first - immediately fails the probe.
	c.Add(func() error {
		health.GetReadinessState().SetShuttingDown()
		logger.Info(context.Background(), "Marked as not ready, draining in-flight requests...")
		return nil
	})

	serverResults := make(chan error, 3)
	start := func(name string, srv server.Server) {
		go func() {
			if err := srv.Start(); err != nil {
				serverResults <- fmt.Errorf("%s server failed: %w", name, err)
			}
		}()
	}
	start("API", apiServer)
	start("metrics", metricsServer)
	start("health", healthServer)

	allListening := make(chan struct{})
	go func() {
		<-apiServer.NotifyListening()
		<-metricsServer.NotifyListening()
		<-healthServer.NotifyListening()
		close(allListening)
	}()

	var triggerErr error
	shutdown := false
	select {
	case <-ctx.Done():
		shutdown = true
	case <-signals:
		shutdown = true
	case triggerErr = <-serverResults:
	case <-allListening:
	}
	if triggerErr == nil && !shutdown {
		health.GetReadinessState().SetReady()
		logger.Info(ctx, "Application ready to receive traffic")
		select {
		case <-ctx.Done():
		case <-signals:
		case triggerErr = <-serverResults:
		}
	}

	logger.Info(context.Background(), "Shutdown requested, starting graceful shutdown...")
	runErr = triggerErr
	return runErr
}

func addDrain(c *closer.Closer, srv server.Server, budget time.Duration) {
	c.Add(func() error {
		drainCtx, cancel := context.WithTimeout(context.Background(), budget)
		defer cancel()
		if err := srv.Shutdown(drainCtx); err != nil {
			return errors.Join(err, srv.Close())
		}
		return nil
	})
}

func initLogger(cfg *config.ApplicationConfig) {
	ctx := context.Background()
	loggingCfg := cfg.Logging

	level, err := logger.ParseLogLevel(loggingCfg.Level)
	if err != nil {
		logger.With(ctx, logger.FieldLogLevel, loggingCfg.Level).WithError(err).Warn("Invalid log level, using default")
		level = slog.LevelInfo
	}

	format, err := logger.ParseLogFormat(loggingCfg.Format)
	if err != nil {
		logger.With(ctx, logger.FieldLogFormat, loggingCfg.Format).WithError(err).Warn("Invalid log format, using default")
		format = logger.FormatJSON
	}

	output, err := logger.ParseLogOutput(loggingCfg.Output)
	if err != nil {
		logger.With(ctx, logger.FieldLogOutput, loggingCfg.Output).WithError(err).Warn("Invalid log output, using default")
		output = os.Stdout
	}

	hostname := cfg.Server.Hostname
	if hostname == "" {
		hostname, _ = os.Hostname() //nolint:errcheck // empty string is acceptable fallback
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
}

func configureDBLogger(cfg *config.ApplicationConfig, sessionFactory db.SessionFactory) {
	gormLevel := cfg.Database.SetLogLevel(cfg.Logging.Level)
	if reconfigurable, ok := sessionFactory.(db_session.LoggerReconfigurable); ok {
		reconfigurable.ReconfigureLogger(gormLevel)
	}
}
