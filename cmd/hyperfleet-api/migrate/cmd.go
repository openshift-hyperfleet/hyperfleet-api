package migrate

import (
	"context"
	"flag"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_session"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

var dbConfig = config.NewDatabaseConfig()

// NewMigrateCommand migrate sub-command handles running migrations
func NewMigrateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run hyperfleet service data migrations",
		Long:  "Run hyperfleet service data migrations",
		Run:   runMigrate,
	}

	dbConfig.AddFlags(cmd.PersistentFlags())
	cmd.PersistentFlags().AddGoFlagSet(flag.CommandLine)
	return cmd
}

func runMigrate(_ *cobra.Command, _ []string) {
	ctx := context.Background()
	err := dbConfig.ReadFiles()
	if err != nil {
		logger.Error(ctx, "Fatal error", "error", err)
		os.Exit(1)
	}

	connection := db_session.NewProdFactory(dbConfig)
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			logger.Error(ctx, "Failed to close database connection", "error", closeErr)
		}
	}()

	if err := db.Migrate(connection.New(ctx)); err != nil {
		logger.Error(ctx, "Migration failed", "error", err)
		// Close connection before exit to avoid resource leak
		if closeErr := connection.Close(); closeErr != nil {
			logger.Error(ctx, "Failed to close database connection", "error", closeErr)
		}
		os.Exit(1)
	}

	logger.Info(ctx, "Migration completed successfully")
}
