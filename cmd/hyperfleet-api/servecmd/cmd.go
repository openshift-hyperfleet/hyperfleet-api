package servecmd

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	hfl "github.com/openshift-hyperfleet/hyperfleet-logger"
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
	// (API server drain) + metricsDrainTimeout + healthDrainTimeout +
	// otelFlushTimeout + container.dbCloseTimeout.
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
	// container.Container's accessors panic by design; log the stack here since main.go can't see it.
	defer func() {
		if r := recover(); r != nil {
			slog.ErrorContext(context.Background(),
				fmt.Sprintf("recovered from panic in runServe: %v", r), "panic_stack", string(debug.Stack()),
			)
			runErr = fmt.Errorf("%v", r)
		}
	}()

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
			slog.InfoContext(context.Background(), "Graceful shutdown completed")
		}
	}()

	initLogger(cfg)

	ctr := container.NewContainer(cfg, c)

	sf := ctr.SessionFactory()
	configureDBLogger(cfg, sf)

	slog.InfoContext(ctx, "Starting HyperFleet API with configuration (sensitive values redacted):")
	slog.InfoContext(ctx, config.DumpConfig(cfg))

	// OTel registered first so it flushes last - teardown spans are preserved.
	if cfg.Tracing.Enabled {
		traceProvider, traceErr := telemetry.InitTraceProvider(ctx, cfg.Tracing.ServiceName, api.Version)
		if traceErr != nil {
			slog.WarnContext(ctx, "Failed to initialize OpenTelemetry", "error", traceErr)
		} else {
			slog.InfoContext(ctx, "OpenTelemetry initialized", logger.FieldServiceName, cfg.Tracing.ServiceName)
			c.Add(func() error {
				flushCtx, cancel := context.WithTimeout(context.Background(), otelFlushTimeout)
				defer cancel()
				return telemetry.Shutdown(flushCtx, traceProvider)
			})
		}
	} else {
		slog.InfoContext(ctx, "OpenTelemetry disabled", logger.FieldOTelEnabled, false)
	}
	slog.InfoContext(ctx,
		"Logger initialized", "log_level", cfg.Logging.Level,
		"log_format", cfg.Logging.Format,
		"log_output", cfg.Logging.Output,
		"masking_enabled", cfg.Logging.Masking.Enabled,
	)

	if collectorErr := metrics.RegisterReconciliationCollector(
		ctr.SessionFactory().DirectDB(),
		cfg.Metrics.ReconciliationStuckThreshold,
	); collectorErr != nil {
		slog.ErrorContext(ctx, "Failed to register reconciliation collector", "error", collectorErr)
	}

	apiServer, err := BuildAPIServer(
		cfg,
		ctr.ResourceService(),
		ctr.AdapterStatusService(),
		ctr.SchemaValidator(),
		ctr.JWTHandler(),
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
		slog.InfoContext(context.Background(), "Marked as not ready, draining in-flight requests...")
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
		select {
		case triggerErr = <-serverResults:
		default:
		}
	}
	if triggerErr == nil && !shutdown {
		health.GetReadinessState().SetReady()
		slog.InfoContext(ctx, "Application ready to receive traffic")
		select {
		case <-ctx.Done():
		case <-signals:
		case triggerErr = <-serverResults:
		}
	}

	slog.InfoContext(context.Background(), "Shutdown requested, starting graceful shutdown...")
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

	level, err := hfl.ParseLevel(loggingCfg.Level)
	if err != nil {
		slog.WarnContext(ctx, "Invalid log level, using default", logger.FieldLogLevel, loggingCfg.Level, "error", err)
		level = slog.LevelInfo
	}

	format, err := hfl.ParseFormat(loggingCfg.Format)
	if err != nil {
		slog.WarnContext(ctx, "Invalid log format, using default", logger.FieldLogFormat, loggingCfg.Format, "error", err)
		format = hfl.FormatJSON
	}

	output, err := hfl.ParseOutput(loggingCfg.Output)
	if err != nil {
		slog.WarnContext(ctx, "Invalid log output, using default", logger.FieldLogOutput, loggingCfg.Output, "error", err)
		output = os.Stdout
	}

	slog.SetDefault(logger.NewLogger(api.Version, logger.HandlerConfig{
		Level: level, Format: format, Output: output,
	}))
}

func configureDBLogger(cfg *config.ApplicationConfig, sessionFactory db.SessionFactory) {
	gormLevel := cfg.Database.SetLogLevel(cfg.Logging.Level)
	if reconfigurable, ok := sessionFactory.(db_session.LoggerReconfigurable); ok {
		reconfigurable.ReconfigureLogger(gormLevel)
	}
}
