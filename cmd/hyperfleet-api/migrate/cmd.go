package migrate

import (
	"context"
	"log/slog"
	"os"

	hfl "github.com/openshift-hyperfleet/hyperfleet-logger"
	"github.com/spf13/cobra"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
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
		slog.ErrorContext(ctx, "Failed to load configuration", "error", err)
		os.Exit(1)
	}
	initLogger(appConfig.Logging)

	// Run migration with the loaded configuration
	if err := runMigrateWithError(ctx, appConfig.Database); err != nil {
		os.Exit(1)
	}
}

func initLogger(loggingCfg *config.LoggingConfig) {
	level, err := hfl.ParseLevel(loggingCfg.Level)
	if err != nil {
		level = slog.LevelInfo
	}
	format, err := hfl.ParseFormat(loggingCfg.Format)
	if err != nil {
		format = hfl.FormatJSON
	}
	output, err := hfl.ParseOutput(loggingCfg.Output)
	if err != nil {
		output = os.Stdout
	}
	slog.SetDefault(logger.NewLogger(api.Version, logger.HandlerConfig{
		Level: level, Format: format, Output: output,
	}))
}

func runMigrateWithError(ctx context.Context, dbConfig *config.DatabaseConfig) error {
	connection := db_session.NewProdFactory(dbConfig)
	defer func() {
		if closeErr := connection.Close(); closeErr != nil {
			slog.ErrorContext(ctx, "Failed to close database connection", "error", closeErr)
		}
	}()

	// Use MigrateWithLock to prevent concurrent migrations from multiple pods
	if err := db.MigrateWithLock(ctx, connection); err != nil {
		slog.ErrorContext(ctx, "Migration failed", "error", err)
		return err
	}

	return nil
}
