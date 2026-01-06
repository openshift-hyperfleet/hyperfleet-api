package main

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/migrate"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/servecmd"

	// Import plugins to trigger their init() functions
	// _ "github.com/openshift-hyperfleet/hyperfleet-api/plugins/events" // REMOVED: Events plugin no longer exists
	_ "github.com/openshift-hyperfleet/hyperfleet-api/plugins/adapterStatus"
	_ "github.com/openshift-hyperfleet/hyperfleet-api/plugins/clusters"
	_ "github.com/openshift-hyperfleet/hyperfleet-api/plugins/generic"
	_ "github.com/openshift-hyperfleet/hyperfleet-api/plugins/nodePools"
)

// nolint

func main() {
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
		slog.Error("Error running command", "error", err)
		os.Exit(1)
	}
}
