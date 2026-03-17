package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/migrate"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/servecmd"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"

	// Import plugins to trigger their init() functions
	// _ "github.com/openshift-hyperfleet/hyperfleet-api/plugins/events" // REMOVED: Events plugin no longer exists
	_ "github.com/openshift-hyperfleet/hyperfleet-api/plugins/adapterStatus"
	_ "github.com/openshift-hyperfleet/hyperfleet-api/plugins/clusters"
	_ "github.com/openshift-hyperfleet/hyperfleet-api/plugins/generic"
	_ "github.com/openshift-hyperfleet/hyperfleet-api/plugins/nodePools"
)

// nolint

func main() {
	// Initialize logger first (before any logging occurs)
	initDefaultLogger()
	ctx := context.Background()

	rootCmd := &cobra.Command{
		Use:  "hyperfleet",
		Long: "hyperfleet serves as a template for new microservices",
	}

	// All subcommands under root
	migrateCmd := migrate.NewMigrateCommand()
	serveCmd := servecmd.NewServeCommand()
	versionCmd := newVersionCommand()

	// Add subcommand(s)
	rootCmd.AddCommand(migrateCmd, serveCmd, versionCmd)

	if err := rootCmd.Execute(); err != nil {
		logger.WithError(ctx, err).Error("Error running command")
		os.Exit(1)
	}
}

// initDefaultLogger initializes a default logger with INFO level
// This ensures logging works before environment/config is loaded
// Reads HYPERFLEET_LOGGING_* from environment variables if set
func initDefaultLogger() {
	// Read log level from environment with default fallback
	level := slog.LevelInfo
	if levelStr := os.Getenv("HYPERFLEET_LOGGING_LEVEL"); levelStr != "" {
		if parsed, err := logger.ParseLogLevel(levelStr); err == nil {
			level = parsed
		}
	}

	// Read log format from environment with default fallback
	format := logger.FormatJSON
	if formatStr := os.Getenv("HYPERFLEET_LOGGING_FORMAT"); formatStr != "" {
		if parsed, err := logger.ParseLogFormat(formatStr); err == nil {
			format = parsed
		}
	}

	// Read log output from environment with default fallback
	var output io.Writer = os.Stdout
	if outputStr := os.Getenv("HYPERFLEET_LOGGING_OUTPUT"); outputStr != "" {
		if parsed, err := logger.ParseLogOutput(outputStr); err == nil {
			output = parsed
		}
	}

	cfg := &logger.LogConfig{
		Level:     level,
		Format:    format,
		Output:    output,
		Component: "hyperfleet-api",
		Version:   api.Version,
		Hostname:  getHostname(),
	}
	logger.InitGlobalLogger(cfg)
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "unknown"
	}
	return hostname
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("Version:    %s\n", api.Version)
			fmt.Printf("Commit:     %s\n", api.Commit)
			fmt.Printf("Build Date: %s\n", api.BuildTime)
		},
	}
}
