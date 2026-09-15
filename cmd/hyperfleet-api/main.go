package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"

	hfl "github.com/openshift-hyperfleet/hyperfleet-logger"
	"github.com/spf13/cobra"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/migrate"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/servecmd"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
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
		slog.ErrorContext(ctx, "Error running command", "error", err)
		os.Exit(1)
	}
}

// initDefaultLogger initializes a default logger with INFO level
// This ensures logging works before environment/config is loaded
// Reads HYPERFLEET_LOGGING_* variables if set.
func initDefaultLogger() {
	// Read log level from environment with default fallback
	level := slog.LevelInfo
	if levelStr := os.Getenv("HYPERFLEET_LOGGING_LEVEL"); levelStr != "" {
		if parsed, err := hfl.ParseLevel(levelStr); err == nil {
			level = parsed
		}
	}

	// Read log format from environment with default fallback
	format := hfl.FormatJSON
	if formatStr := os.Getenv("HYPERFLEET_LOGGING_FORMAT"); formatStr != "" {
		if parsed, err := hfl.ParseFormat(formatStr); err == nil {
			format = parsed
		}
	}

	// Read log output from environment with default fallback
	var output io.Writer = os.Stdout
	if outputStr := os.Getenv("HYPERFLEET_LOGGING_OUTPUT"); outputStr != "" {
		if parsed, err := hfl.ParseOutput(outputStr); err == nil {
			output = parsed
		}
	}

	slog.SetDefault(logger.NewLogger(api.Version, logger.HandlerConfig{
		Level: level, Format: format, Output: output,
	}))
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
