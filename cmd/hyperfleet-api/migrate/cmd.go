package migrate

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_session"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// NewMigrateCommand migrate sub-command handles running migrations
func NewMigrateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run hyperfleet service data migrations",
		Long:  "Run hyperfleet service data migrations",
		Run:   runMigrate,
	}

	// Add new configuration system flags (for migration)
	// Database flags are the primary concern for migrate command
	config.AddConfigFlag(cmd)
	config.AddDatabaseFlags(cmd)
	config.AddLoggingFlags(cmd) // For logging during migration

	return cmd
}

func runMigrate(cmd *cobra.Command, _ []string) {
	ctx := context.Background()

	// ============================================================
	// CONFIGURATION LOADING
	// ============================================================
	// Load full application config using Viper-based system
	loader := config.NewConfigLoader()
	appConfig, err := loader.Load(ctx, cmd)
	if err != nil {
		logger.WithError(ctx, err).Error("Failed to load configuration")
		os.Exit(1)
	}

	// Run migration with the loaded configuration
	if err := runMigrateWithError(ctx, appConfig.Database); err != nil {
		os.Exit(1)
	}
}

func runMigrateWithError(ctx context.Context, dbConfig *config.DatabaseConfig) error {
	connection := db_session.NewProdFactory(dbConfig)
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			logger.WithError(ctx, closeErr).Error("Failed to close database connection")
		}
	}()

	if err := db.Migrate(connection.New(ctx)); err != nil {
		logger.WithError(ctx, err).Error("Migration failed")
		return err
	}

	logger.Info(ctx, "Migration completed successfully")
	return nil
}
