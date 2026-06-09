package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/configloader"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/dryrun"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/executor"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/hyperfleetapi"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/k8sclient"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/maestroclient"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/internal/transportclient"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/health"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/metrics"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/telemetry"
	"github.com/openshift-hyperfleet/hyperfleet-adapter/pkg/version"
	"github.com/openshift-hyperfleet/hyperfleet-broker/broker"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"gopkg.in/yaml.v3"
)

// Command-line flags
var (
	configPath     string // Path to deployment config (adapter-config.yaml)
	taskConfigPath string // Path to task config (adapter-task-config.yaml)
	logLevel       string
	logFormat      string
	logOutput      string

	// Dry-run flags
	dryRunEvent        string // Path to CloudEvent JSON file
	dryRunAPIResponses string // Path to mock API responses JSON file
	dryRunDiscovery    string // Path to mock discovery responses JSON file
	dryRunVerbose      bool   // Show verbose dry-run output
	dryRunOutput       string // Output format: text or json
)

// Timeout constants
const (
	// OTelShutdownTimeout is the timeout for gracefully shutting down the OpenTelemetry TracerProvider
	OTelShutdownTimeout = 5 * time.Second
	// HealthServerShutdownTimeout is the timeout for gracefully shutting down the health server
	HealthServerShutdownTimeout = 5 * time.Second
)

// Server port constants
const (
	// HealthServerPort is the port for /healthz and /readyz endpoints
	HealthServerPort = "8080"
	// MetricsServerPort is the port for /metrics endpoint
	MetricsServerPort = "9090"
)

func main() {
	// Root command
	rootCmd := &cobra.Command{
		Use:   "adapter",
		Short: "HyperFleet Adapter - event-driven Kubernetes resource manager",
		Long: `HyperFleet Adapter listens for events from a message broker and
executes configured actions including Kubernetes resource management
and HyperFleet API calls.`,
		// Disable default completion command
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: true,
		},
	}

	// Add flags to root command (so they work on all subcommands)
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)

	// Serve command
	serveCmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the adapter and begin processing events",
		Long: `Start the HyperFleet adapter in serve mode. The adapter will:
- Connect to the configured message broker
- Subscribe to the specified topic
- Process incoming events according to the adapter configuration
- Execute Kubernetes operations and HyperFleet API calls

Dry-run mode:
  Pass --dry-run-event to process a single CloudEvent from a JSON file
  using mock transport clients. No broker, cluster, or API is required.
  Optionally pass --dry-run-api-responses to configure mock API responses.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if isDryRun() {
				return runDryRun(cmd.Flags())
			}
			return runServe(cmd.Flags())
		},
	}
	addConfigPathFlags(serveCmd)
	addOverrideFlags(serveCmd)
	serveCmd.Flags().Bool("debug-config", false,
		"Log the full merged configuration after load. Env: HYPERFLEET_DEBUG_CONFIG")
	serveCmd.Flags().StringVar(&logLevel, "log-level", "",
		"Log level (debug, info, warn, error). Env: LOG_LEVEL")
	serveCmd.Flags().StringVar(&logFormat, "log-format", "",
		"Log format (text, json). Env: LOG_FORMAT")
	serveCmd.Flags().StringVar(&logOutput, "log-output", "",
		"Log output (stdout, stderr). Env: LOG_OUTPUT")
	serveCmd.Flags().StringVar(&dryRunEvent, "dry-run-event", "",
		"Path to CloudEvent JSON file for dry-run mode")
	serveCmd.Flags().StringVar(&dryRunAPIResponses, "dry-run-api-responses", "",
		"Path to mock API responses JSON file for dry-run mode (defaults to 200 OK)")
	serveCmd.Flags().StringVar(&dryRunDiscovery, "dry-run-discovery", "",
		"Path to mock discovery responses JSON file for dry-run mode (overrides applied resources)")
	serveCmd.Flags().BoolVar(&dryRunVerbose, "dry-run-verbose", false,
		"Show rendered manifests, API request/response bodies in dry-run output")
	serveCmd.Flags().StringVar(&dryRunOutput, "dry-run-output", "text",
		"Dry-run output format: text or json")

	// Config-dump command: loads config and prints the merged result as YAML, then exits.
	// Useful for debugging and verifying that config files, env vars, and CLI flags load correctly.
	configDumpCmd := &cobra.Command{
		Use:   "config-dump",
		Short: "Load and print the merged adapter configuration as YAML",
		Long: `Load the adapter configuration from config files, environment variables,
and CLI flags, then print the merged result as YAML to stdout.
Sensitive fields (certificates, keys) are redacted.
Exits with code 0 on success, non-zero on error.

Priority order (lowest to highest): config file < env vars < CLI flags`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runConfigDump(cmd.Flags())
		},
	}
	addConfigPathFlags(configDumpCmd)
	addOverrideFlags(configDumpCmd)
	configDumpCmd.Flags().Bool("debug-config", false,
		"Include debug_config field in output. Env: HYPERFLEET_DEBUG_CONFIG")
	configDumpCmd.Flags().StringVar(&logLevel, "log-level", "",
		"Log level (debug, info, warn, error). Env: LOG_LEVEL")
	configDumpCmd.Flags().StringVar(&logFormat, "log-format", "",
		"Log format (text, json). Env: LOG_FORMAT")
	configDumpCmd.Flags().StringVar(&logOutput, "log-output", "",
		"Log output (stdout, stderr). Env: LOG_OUTPUT")

	// Version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			info := version.Info()
			fmt.Printf("Version:    %s\n", info.Version)
			fmt.Printf("Commit:     %s\n", info.Commit)
			fmt.Printf("Build Date: %s\n", info.BuildDate)
		},
	}

	// Add subcommands
	rootCmd.AddCommand(serveCmd)
	rootCmd.AddCommand(configDumpCmd)
	rootCmd.AddCommand(versionCmd)

	// Execute
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// isDryRun returns true when dry-run flags are present.
func isDryRun() bool {
	return dryRunEvent != "" || dryRunAPIResponses != ""
}

// -----------------------------------------------------------------------------
// Configuration loading (shared between serve and dry-run)
// -----------------------------------------------------------------------------

// buildLoggerConfig creates a logger configuration with the following priority
// (lowest to highest): config file < LOG_* env vars < --log-* CLI flags.
// Pass logCfg=nil for the bootstrap logger (before config is loaded).
func buildLoggerConfig(component string, logCfg *configloader.LogConfig) logger.Config {
	cfg := logger.DefaultConfig()

	// Apply config file values (lowest priority)
	if logCfg != nil {
		if logCfg.Level != "" {
			cfg.Level = logCfg.Level
		}
		if logCfg.Format != "" {
			cfg.Format = logCfg.Format
		}
		if logCfg.Output != "" {
			cfg.Output = logCfg.Output
		}
	}

	// Apply environment variables (override config file)
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		cfg.Level = strings.ToLower(level)
	}
	if format := os.Getenv("LOG_FORMAT"); format != "" {
		cfg.Format = strings.ToLower(format)
	}
	if output := os.Getenv("LOG_OUTPUT"); output != "" {
		cfg.Output = output
	}

	// Apply CLI flags (highest priority)
	if logLevel != "" {
		cfg.Level = logLevel
	}
	if logFormat != "" {
		cfg.Format = logFormat
	}
	if logOutput != "" {
		cfg.Output = logOutput
	}

	cfg.Component = component
	cfg.Version = version.Version

	return cfg
}

// loadConfig loads the unified adapter configuration from both config files.
func loadConfig(ctx context.Context, log logger.Logger, flags *pflag.FlagSet) (*configloader.Config, error) {
	log.Info(ctx, "Loading adapter configuration...")
	config, err := configloader.LoadConfig(
		configloader.WithAdapterConfigPath(configPath),
		configloader.WithTaskConfigPath(taskConfigPath),
		configloader.WithAdapterVersion(version.Version),
		configloader.WithFlags(flags),
		configloader.WithContext(ctx),
		configloader.WithLogger(log),
	)
	if err != nil {
		errCtx := logger.WithErrorField(ctx, err)
		log.Errorf(errCtx, "Failed to load adapter configuration")
		return nil, fmt.Errorf("failed to load adapter configuration: %w", err)
	}
	return config, nil
}

// -----------------------------------------------------------------------------
// Client creation (shared between serve and dry-run)
// -----------------------------------------------------------------------------

// createAPIClient creates a HyperFleet API client from the config
func createAPIClient(apiConfig configloader.HyperfleetAPIConfig, log logger.Logger) (hyperfleetapi.Client, error) {
	var opts []hyperfleetapi.ClientOption

	// Set base URL if configured (env fallback handled in NewClient)
	if apiConfig.BaseURL != "" {
		opts = append(opts, hyperfleetapi.WithBaseURL(apiConfig.BaseURL))
	}

	// Set timeout if configured (0 means use default)
	if apiConfig.Timeout > 0 {
		opts = append(opts, hyperfleetapi.WithTimeout(apiConfig.Timeout))
	}

	// Set retry attempts
	if apiConfig.RetryAttempts > 0 {
		opts = append(opts, hyperfleetapi.WithRetryAttempts(apiConfig.RetryAttempts))
	}

	// Set retry backoff strategy
	if apiConfig.RetryBackoff != "" {
		switch apiConfig.RetryBackoff {
		case hyperfleetapi.BackoffExponential, hyperfleetapi.BackoffLinear, hyperfleetapi.BackoffConstant:
			opts = append(opts, hyperfleetapi.WithRetryBackoff(apiConfig.RetryBackoff))
		default:
			return nil, fmt.Errorf(
				"invalid retry backoff strategy %q (supported: exponential, linear, constant)",
				apiConfig.RetryBackoff,
			)
		}
	}

	// Set retry base delay
	if apiConfig.BaseDelay > 0 {
		opts = append(opts, hyperfleetapi.WithBaseDelay(apiConfig.BaseDelay))
	}

	// Set retry max delay
	if apiConfig.MaxDelay > 0 {
		opts = append(opts, hyperfleetapi.WithMaxDelay(apiConfig.MaxDelay))
	}

	// Set default headers
	for key, value := range apiConfig.DefaultHeaders {
		opts = append(opts, hyperfleetapi.WithDefaultHeader(key, value))
	}

	return hyperfleetapi.NewClient(log, opts...)
}

// createTransportClient creates the appropriate transport client based on config.
func createTransportClient(
	ctx context.Context,
	config *configloader.Config,
	log logger.Logger,
) (transportclient.TransportClient, error) {
	if config.Clients.Maestro != nil {
		log.Info(ctx, "Creating Maestro transport client...")
		client, err := createMaestroClient(ctx, config.Clients.Maestro, log)
		if err != nil {
			return nil, fmt.Errorf("failed to create Maestro client: %w", err)
		}
		log.Info(ctx, "Maestro transport client created successfully")
		return client, nil
	}

	log.Info(ctx, "Creating Kubernetes transport client...")
	client, err := createK8sClient(ctx, config.Clients.Kubernetes, log)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	log.Info(ctx, "Kubernetes transport client created successfully")
	return client, nil
}

// createK8sClient creates a Kubernetes client from the config
func createK8sClient(
	ctx context.Context,
	k8sConfig configloader.KubernetesConfig,
	log logger.Logger,
) (*k8sclient.Client, error) {
	clientConfig := k8sclient.ClientConfig{
		KubeConfigPath: k8sConfig.KubeConfigPath,
		QPS:            k8sConfig.QPS,
		Burst:          k8sConfig.Burst,
	}
	return k8sclient.NewClient(ctx, clientConfig, log)
}

// createMaestroClient creates a Maestro client from the config
func createMaestroClient(
	ctx context.Context,
	maestroConfig *configloader.MaestroClientConfig,
	log logger.Logger,
) (*maestroclient.Client, error) {
	config := &maestroclient.Config{
		MaestroServerAddr: maestroConfig.HTTPServerAddress,
		GRPCServerAddr:    maestroConfig.GRPCServerAddress,
		SourceID:          maestroConfig.SourceID,
		Insecure:          maestroConfig.Insecure,
	}

	if maestroConfig.Timeout != "" {
		d, err := time.ParseDuration(maestroConfig.Timeout)
		if err != nil {
			return nil, fmt.Errorf("invalid maestro timeout %q: %w", maestroConfig.Timeout, err)
		}
		config.HTTPTimeout = d
	}

	if maestroConfig.ServerHealthinessTimeout != "" {
		d, err := time.ParseDuration(maestroConfig.ServerHealthinessTimeout)
		if err != nil {
			return nil, fmt.Errorf(
				"invalid maestro serverHealthinessTimeout %q: %w",
				maestroConfig.ServerHealthinessTimeout,
				err,
			)
		}
		config.ServerHealthinessTimeout = d
	}

	if maestroConfig.Auth.TLSConfig != nil {
		config.CAFile = maestroConfig.Auth.TLSConfig.CAFile
		config.ClientCertFile = maestroConfig.Auth.TLSConfig.CertFile
		config.ClientKeyFile = maestroConfig.Auth.TLSConfig.KeyFile
		config.HTTPCAFile = maestroConfig.Auth.TLSConfig.HTTPCAFile
	}

	return maestroclient.NewMaestroClient(ctx, config, log)
}

// buildExecutor creates the executor with the given clients.
func buildExecutor(
	config *configloader.Config,
	apiClient hyperfleetapi.Client,
	tc transportclient.TransportClient,
	log logger.Logger,
	metricsRecorder *metrics.Recorder,
) (*executor.Executor, error) {
	return executor.NewBuilder().
		WithConfig(config).
		WithAPIClient(apiClient).
		WithTransportClient(tc).
		WithLogger(log).
		WithMetricsRecorder(metricsRecorder).
		Build()
}

// -----------------------------------------------------------------------------
// Serve mode (normal operation)
// -----------------------------------------------------------------------------

// runServe contains the main application logic for the serve command
func runServe(flags *pflag.FlagSet) error {
	// Create context that cancels on system signals
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create bootstrap logger (before config is loaded)
	log, err := logger.NewLogger(buildLoggerConfig("hyperfleet-adapter", nil))
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	log.Infof(ctx, "Starting Hyperfleet Adapter version=%s commit=%s built=%s",
		version.Version, version.Commit, version.BuildDate)

	// Load unified configuration (deployment + task configs)
	config, err := loadConfig(ctx, log, flags)
	if err != nil {
		return err
	}

	// Recreate logger with component name and log settings from config
	log, err = logger.NewLogger(buildLoggerConfig(config.Adapter.Name, &config.Log))
	if err != nil {
		return fmt.Errorf("failed to create logger with adapter config: %w", err)
	}

	log.Infof(ctx, "Adapter configuration loaded successfully: name=%s ", config.Adapter.Name)
	log.Infof(ctx, "HyperFleet API client configured: timeout=%s retry_attempts=%d",
		config.Clients.HyperfleetAPI.Timeout.String(), config.Clients.HyperfleetAPI.RetryAttempts)
	var redactedConfigBytes []byte
	if config.DebugConfig {
		var data []byte
		data, err = yaml.Marshal(config.Redacted())
		if err != nil {
			errCtx := logger.WithErrorField(ctx, err)
			log.Warnf(errCtx, "Failed to marshal adapter configuration for logging")
		} else {
			redactedConfigBytes = data
			log.Infof(ctx, "Loaded adapter configuration:\n%s", string(redactedConfigBytes))
		}
	}

	// Initialize OpenTelemetry
	tracingEnabled := true
	if tracingEnv := os.Getenv("HYPERFLEET_TRACING_ENABLED"); tracingEnv != "" {
		var enabled bool
		if enabled, err = strconv.ParseBool(tracingEnv); err == nil {
			tracingEnabled = enabled
		} else {
			log.Warnf(ctx, "Invalid HYPERFLEET_TRACING_ENABLED value %q, defaulting to true", tracingEnv)
		}
	}

	var tp *sdktrace.TracerProvider
	if tracingEnabled {
		serviceName := config.Adapter.Name
		if svcName := os.Getenv("OTEL_SERVICE_NAME"); svcName != "" {
			serviceName = svcName
		}

		var traceProvider *sdktrace.TracerProvider
		traceProvider, err = telemetry.InitTraceProvider(ctx, log, serviceName, version.Version)
		if err != nil {
			errCtx := logger.WithErrorField(ctx, err)
			log.Errorf(errCtx, "Failed to initialize OpenTelemetry")
			return fmt.Errorf("failed to initialize OpenTelemetry: %w", err)
		}
		tp = traceProvider
		log.Infof(ctx, "OpenTelemetry initialized: service_name=%s", serviceName)
	} else {
		log.Infof(ctx, "OpenTelemetry tracing disabled")
	}
	defer func() {
		if tp != nil {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), OTelShutdownTimeout)
			defer shutdownCancel()
			if shutdownErr := tp.Shutdown(shutdownCtx); shutdownErr != nil {
				log.Warnf(ctx, "Failed to shutdown OpenTelemetry: %v", shutdownErr)
			}
		}
	}()

	// Start health server
	healthServer := health.NewServer(log, HealthServerPort, config.Adapter.Name)
	err = healthServer.Start(ctx)
	if err != nil {
		errCtx := logger.WithErrorField(ctx, err)
		log.Errorf(errCtx, "Failed to start health server")
		return fmt.Errorf("failed to start health server: %w", err)
	}
	healthServer.SetConfigLoaded()
	if len(redactedConfigBytes) > 0 {
		healthServer.SetConfig(redactedConfigBytes)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), HealthServerShutdownTimeout)
		defer shutdownCancel()
		if shutdownErr := healthServer.Shutdown(shutdownCtx); shutdownErr != nil {
			errCtx := logger.WithErrorField(shutdownCtx, shutdownErr)
			log.Warnf(errCtx, "Failed to shutdown health server")
		}
	}()

	// Start metrics server
	metricsServer := health.NewMetricsServer(log, MetricsServerPort, health.MetricsConfig{
		Component: config.Adapter.Name,
		Version:   version.Version,
		Commit:    version.Commit,
	})
	err = metricsServer.Start(ctx)
	if err != nil {
		errCtx := logger.WithErrorField(ctx, err)
		log.Errorf(errCtx, "Failed to start metrics server")
		return fmt.Errorf("failed to start metrics server: %w", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), HealthServerShutdownTimeout)
		defer shutdownCancel()
		if shutdownErr := metricsServer.Shutdown(shutdownCtx); shutdownErr != nil {
			errCtx := logger.WithErrorField(shutdownCtx, shutdownErr)
			log.Warnf(errCtx, "Failed to shutdown metrics server")
		}
	}()

	// Create adapter metrics recorder
	adapterName := metrics.ExtractAdapterName(config.Adapter.Name)
	metricsRecorder := metrics.NewRecorder(config.Adapter.Name, version.Version, adapterName, nil)

	// Create real clients
	log.Info(ctx, "Creating HyperFleet API client...")
	apiClient, err := createAPIClient(config.Clients.HyperfleetAPI, log)
	if err != nil {
		errCtx := logger.WithErrorField(ctx, err)
		log.Errorf(errCtx, "Failed to create HyperFleet API client")
		return fmt.Errorf("failed to create HyperFleet API client: %w", err)
	}

	tc, err := createTransportClient(ctx, config, log)
	if err != nil {
		errCtx := logger.WithErrorField(ctx, err)
		log.Errorf(errCtx, "Failed to create transport client")
		return err
	}

	// Build executor
	log.Info(ctx, "Creating event executor...")
	exec, err := buildExecutor(config, apiClient, tc, log, metricsRecorder)
	if err != nil {
		errCtx := logger.WithErrorField(ctx, err)
		log.Errorf(errCtx, "Failed to create executor")
		return fmt.Errorf("failed to create executor: %w", err)
	}

	// Create the event handler and subscribe to broker
	handler := executor.AlwaysAck(executor.WithMetrics(exec.CreateHandler(), metricsRecorder, log))

	// Handle signals for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Infof(ctx, "Received signal %s, initiating graceful shutdown...", sig)
		log.Info(ctx, "Shutdown initiated, marking not ready")
		healthServer.SetShuttingDown(true)
		cancel()

		// Second signal forces immediate exit
		sig = <-sigCh
		log.Infof(ctx, "Received second signal %s, forcing immediate exit", sig)
		os.Exit(1)
	}()

	// Get broker config
	subscriptionID := config.Clients.Broker.SubscriptionID
	if subscriptionID == "" {
		err = fmt.Errorf("clients.broker.subscription_id is required")
		errCtx := logger.WithErrorField(ctx, err)
		log.Errorf(errCtx, "Missing required broker configuration")
		return err
	}

	topic := config.Clients.Broker.Topic
	if topic == "" {
		err = fmt.Errorf("clients.broker.topic is required")
		errCtx := logger.WithErrorField(ctx, err)
		log.Errorf(errCtx, "Missing required broker configuration")
		return err
	}

	// Create broker metrics recorder
	brokerMetrics := broker.NewMetricsRecorder(config.Adapter.Name, version.Version, nil)

	// Create broker subscriber and subscribe
	log.Info(ctx, "Creating broker subscriber...")
	subscriber, err := broker.NewSubscriber(log, subscriptionID, brokerMetrics)
	if err != nil {
		errCtx := logger.WithErrorField(ctx, err)
		log.Errorf(errCtx, "Failed to create subscriber")
		return fmt.Errorf("failed to create subscriber: %w", err)
	}
	log.Info(ctx, "Broker subscriber created successfully")

	log.Info(ctx, "Subscribing to broker topic...")
	err = subscriber.Subscribe(ctx, topic, handler)
	if err != nil {
		errCtx := logger.WithErrorField(ctx, err)
		log.Errorf(errCtx, "Failed to subscribe to topic")
		return fmt.Errorf("failed to subscribe to topic: %w", err)
	}
	log.Info(ctx, "Successfully subscribed to broker topic")

	// Mark as ready
	healthServer.SetBrokerReady(true)
	log.Info(ctx, "Adapter is ready to process events")

	// Monitor subscription errors
	fatalErrCh := make(chan error, 1)
	go func() {
		for subErr := range subscriber.Errors() {
			errCtx := logger.WithErrorField(ctx, subErr)
			log.Errorf(errCtx, "Subscription error")
			select {
			case fatalErrCh <- subErr:
			default:
			}
		}
	}()

	log.Info(ctx, "Adapter started, waiting for events...")

	// Wait for shutdown signal or fatal subscription error
	select {
	case <-ctx.Done():
		log.Info(ctx, "Context canceled, shutting down...")
	case err := <-fatalErrCh:
		errCtx := logger.WithErrorField(ctx, err)
		log.Errorf(errCtx, "Fatal subscription error, shutting down")
		healthServer.SetShuttingDown(true)
		cancel()
	}

	// Close subscriber gracefully
	log.Info(ctx, "Closing broker subscriber...")
	shutdownCtx, shutdownCancel := context.WithTimeout(
		context.Background(), 30*time.Second,
	)
	defer shutdownCancel()

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- subscriber.Close()
	}()

	select {
	case err := <-closeDone:
		if err != nil {
			errCtx := logger.WithErrorField(ctx, err)
			log.Errorf(errCtx, "Error closing subscriber")
		} else {
			log.Info(ctx, "Subscriber closed successfully")
		}
	case <-shutdownCtx.Done():
		err := fmt.Errorf("subscriber close timed out after 30 seconds")
		errCtx := logger.WithErrorField(ctx, err)
		log.Error(errCtx, "Subscriber close timed out")
	}

	log.Info(ctx, "Adapter shutdown complete")

	return nil
}

// -----------------------------------------------------------------------------
// Dry-run mode
// -----------------------------------------------------------------------------

// runDryRun processes a single CloudEvent from file using mock clients.
func runDryRun(flags *pflag.FlagSet) error {
	ctx := context.Background()

	// Create logger on stderr so stdout is reserved for trace output
	log, err := logger.NewLogger(logger.Config{
		Level:     "warn",
		Format:    "text",
		Output:    "stderr",
		Component: "dry-run",
	})
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	// Load config (same path as serve)
	config, err := loadConfig(ctx, log, flags)
	if err != nil {
		return err
	}

	// Load CloudEvent from file
	if dryRunEvent == "" {
		return fmt.Errorf("--dry-run-event is required for dry-run mode")
	}
	evt, err := dryrun.LoadCloudEvent(dryRunEvent)
	if err != nil {
		return fmt.Errorf("failed to load event: %w", err)
	}

	// Create dryrun API client
	var dryrunResponsesFile *dryrun.DryrunResponsesFile
	if dryRunAPIResponses != "" {
		dryrunResponsesFile, err = dryrun.LoadDryrunResponses(dryRunAPIResponses)
		if err != nil {
			return fmt.Errorf("failed to load dryrun responses: %w", err)
		}
	}
	dryrunAPI, err := dryrun.NewDryrunAPIClient(dryrunResponsesFile)
	if err != nil {
		return fmt.Errorf("failed to create dryrun API client: %w", err)
	}

	// Create recording transport client
	var dryrunClient *dryrun.DryrunTransportClient
	if dryRunDiscovery != "" {
		var overrides dryrun.DiscoveryOverrides
		overrides, err = dryrun.LoadDiscoveryOverrides(dryRunDiscovery)
		if err != nil {
			return fmt.Errorf("failed to load discovery overrides: %w", err)
		}
		dryrunClient = dryrun.NewDryrunTransportClientWithOverrides(overrides)
	} else {
		dryrunClient = dryrun.NewDryrunTransportClient()
	}

	// Build executor with mock clients (same builder as serve, no metrics in dry-run)
	exec, err := buildExecutor(config, dryrunAPI, dryrunClient, log, nil)
	if err != nil {
		return fmt.Errorf("failed to create executor: %w", err)
	}

	// Execute with event data
	result := exec.Execute(ctx, evt.Data())

	// Build and output execution trace
	trace := &dryrun.ExecutionTrace{
		EventID:   evt.ID(),
		EventType: evt.Type(),
		Result:    result,
		APIClient: dryrunAPI,
		Transport: dryrunClient,
		Verbose:   dryRunVerbose,
	}

	switch dryRunOutput {
	case "json":
		data, err := trace.FormatJSON()
		if err != nil {
			return fmt.Errorf("failed to format trace as JSON: %w", err)
		}
		fmt.Println(string(data))
	default:
		fmt.Print(trace.FormatText())
	}

	if result.Status == executor.StatusFailed {
		for phase, err := range result.Errors {
			fmt.Fprintf(os.Stderr, "Error in %s: %v\n", phase, err)
		}
	}

	return nil
}

// -----------------------------------------------------------------------------
// Config-dump mode
// -----------------------------------------------------------------------------

// runConfigDump loads the full adapter configuration and prints it as YAML to stdout.
// Sensitive fields are redacted. Exits 0 on success.
func runConfigDump(flags *pflag.FlagSet) error {
	ctx := context.Background()
	log, err := logger.NewLogger(buildLoggerConfig("config-dump", nil))
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}

	config, err := loadConfig(ctx, log, flags)
	if err != nil {
		return err
	}

	data, err := yaml.Marshal(config.Redacted())
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	fmt.Print(string(data))
	return nil
}

// -----------------------------------------------------------------------------
// Flag registration helpers (shared between serve and config-dump)
// -----------------------------------------------------------------------------

// addConfigPathFlags registers the --config and --task-config path flags.
func addConfigPathFlags(cmd *cobra.Command) {
	cmd.Flags().StringVarP(&configPath, "config", "c", "",
		fmt.Sprintf("Path to adapter deployment config file (can also use %s env var)",
			configloader.EnvAdapterConfig))
	cmd.Flags().StringVarP(&taskConfigPath, "task-config", "t", "",
		fmt.Sprintf("Path to adapter task config file (can also use %s env var)",
			configloader.EnvTaskConfigPath))
}

// addOverrideFlags registers all configuration override flags (Maestro, API, broker, Kubernetes).
// These flags are available on both the serve and config-dump commands.
func addOverrideFlags(cmd *cobra.Command) {
	// Maestro override flags
	cmd.Flags().String("maestro-grpc-server-address", "",
		"Maestro gRPC server address. Env: HYPERFLEET_MAESTRO_GRPC_SERVER_ADDRESS")
	cmd.Flags().String("maestro-http-server-address", "",
		"Maestro HTTP server address. Env: HYPERFLEET_MAESTRO_HTTP_SERVER_ADDRESS")
	cmd.Flags().String("maestro-source-id", "", "Maestro source ID. Env: HYPERFLEET_MAESTRO_SOURCE_ID")
	cmd.Flags().String("maestro-client-id", "", "Maestro client ID. Env: HYPERFLEET_MAESTRO_CLIENT_ID")
	cmd.Flags().String("maestro-auth-type", "", "Maestro auth type (tls, none). Env: HYPERFLEET_MAESTRO_AUTH_TYPE")
	cmd.Flags().String("maestro-ca-file", "", "Maestro gRPC CA certificate file. Env: HYPERFLEET_MAESTRO_CA_FILE")
	cmd.Flags().String("maestro-cert-file", "", "Maestro gRPC client certificate file. Env: HYPERFLEET_MAESTRO_CERT_FILE")
	cmd.Flags().String("maestro-key-file", "", "Maestro gRPC client key file. Env: HYPERFLEET_MAESTRO_KEY_FILE")
	cmd.Flags().String("maestro-http-ca-file", "",
		"Maestro HTTP CA certificate file. Env: HYPERFLEET_MAESTRO_HTTP_CA_FILE")
	cmd.Flags().String("maestro-timeout", "",
		"Maestro client timeout (e.g. 10s). Env: HYPERFLEET_MAESTRO_TIMEOUT")
	cmd.Flags().String("maestro-server-healthiness-timeout", "",
		"Maestro server healthiness check timeout (e.g. 20s). Env: HYPERFLEET_MAESTRO_SERVER_HEALTHINESS_TIMEOUT")
	cmd.Flags().Int("maestro-retry-attempts", 0,
		"Maestro retry attempts. Env: HYPERFLEET_MAESTRO_RETRY_ATTEMPTS")
	cmd.Flags().String("maestro-keepalive-time", "",
		"Maestro gRPC keepalive ping interval (e.g. 30s). Env: HYPERFLEET_MAESTRO_KEEPALIVE_TIME")
	cmd.Flags().String("maestro-keepalive-timeout", "",
		"Maestro gRPC keepalive ping timeout (e.g. 10s). Env: HYPERFLEET_MAESTRO_KEEPALIVE_TIMEOUT")
	cmd.Flags().Bool("maestro-insecure", false,
		"Use insecure connection to Maestro. Env: HYPERFLEET_MAESTRO_INSECURE")

	// HyperFleet API override flags
	cmd.Flags().String("hyperfleet-api-base-url", "", "HyperFleet API base URL. Env: HYPERFLEET_API_BASE_URL")
	cmd.Flags().String("hyperfleet-api-version", "", "HyperFleet API version (e.g. v1). Env: HYPERFLEET_API_VERSION")
	cmd.Flags().String("hyperfleet-api-timeout", "",
		"HyperFleet API timeout (e.g. 10s). Env: HYPERFLEET_API_TIMEOUT")
	cmd.Flags().Int("hyperfleet-api-retry", 0,
		"HyperFleet API retry attempts. Env: HYPERFLEET_API_RETRY_ATTEMPTS")
	cmd.Flags().String("hyperfleet-api-retry-backoff", "",
		"HyperFleet API retry backoff strategy (exponential, linear, constant). Env: HYPERFLEET_API_RETRY_BACKOFF")
	cmd.Flags().String("hyperfleet-api-base-delay", "",
		"HyperFleet API retry base delay (e.g. 1s). Env: HYPERFLEET_API_BASE_DELAY")
	cmd.Flags().String("hyperfleet-api-max-delay", "",
		"HyperFleet API retry max delay (e.g. 30s). Env: HYPERFLEET_API_MAX_DELAY")

	// Broker override flags
	cmd.Flags().String("broker-subscription-id", "", "Broker subscription ID. Env: HYPERFLEET_BROKER_SUBSCRIPTION_ID")
	cmd.Flags().String("broker-topic", "", "Broker topic. Env: HYPERFLEET_BROKER_TOPIC")

	// Kubernetes override flags
	cmd.Flags().String("kubernetes-kube-config-path", "",
		"Path to kubeconfig file (empty = in-cluster auth). Env: HYPERFLEET_KUBERNETES_KUBE_CONFIG_PATH")
	cmd.Flags().String("kubernetes-api-version", "", "Kubernetes API version. Env: HYPERFLEET_KUBERNETES_API_VERSION")
	cmd.Flags().Float64("kubernetes-qps", 0, "Kubernetes client QPS rate limit. Env: HYPERFLEET_KUBERNETES_QPS")
	cmd.Flags().Int("kubernetes-burst", 0, "Kubernetes client burst rate limit. Env: HYPERFLEET_KUBERNETES_BURST")
}
