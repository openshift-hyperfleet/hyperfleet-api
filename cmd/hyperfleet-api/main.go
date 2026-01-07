package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/migrate"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/servecmd"
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

	// Parse flags (needed for cobra compatibility)
	if err := flag.CommandLine.Parse([]string{}); err != nil {
		logger.Error(ctx, "Failed to parse flags", "error", err)
		os.Exit(1)
	}

	rootCmd := &cobra.Command{
		Use:  "hyperfleet",
		Long: "hyperfleet serves as a template for new microservices",
	}

	// All subcommands under root
	migrateCmd := migrate.NewMigrateCommand()
	serveCmd := servecmd.NewServeCommand()

	// Add subcommand(s)
	rootCmd.AddCommand(migrateCmd, serveCmd)

	if err := rootCmd.Execute(); err != nil {
		logger.Error(ctx, "Error running command", "error", err)
		os.Exit(1)
	}
}

// initDefaultLogger initializes a default logger with INFO level
// This ensures logging works before environment/config is loaded
func initDefaultLogger() {
	cfg := &logger.LogConfig{
		Level:     slog.LevelInfo,
		Format:    logger.FormatJSON,
		Output:    os.Stdout,
		Component: "hyperfleet-api",
		Version:   "unknown",
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
