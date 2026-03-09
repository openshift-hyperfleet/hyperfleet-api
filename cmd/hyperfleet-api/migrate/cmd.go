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

var dbConfig = config.NewDatabaseConfig()

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
	// CONFIGURATION LOADING WITH MIGRATION SUPPORT
	// ============================================================
	var finalDBConfig *config.DatabaseConfig

	if config.IsNewConfigEnabled() {
		logger.Info(ctx, "Using new configuration system for migration")

		// Load full application config using new system
		loader := config.NewConfigLoader()
		appConfig, err := loader.Load(ctx, cmd)
		if err != nil {
			logger.WithError(ctx, err).Error("Failed to load configuration")
			os.Exit(1)
		}

		finalDBConfig = appConfig.Database
		logger.Info(ctx, "Database configuration loaded via new system")
	} else {
		logger.Info(ctx, "Using legacy configuration system for migration")

		// Bind CLI flags to dbConfig (flags registered via AddDatabaseFlags)
		// This ensures CLI overrides like --db-host are honored in legacy mode
		if host, _ := cmd.Flags().GetString("db-host"); host != "" {
			dbConfig.Host = host
		}
		if port, _ := cmd.Flags().GetInt("db-port"); port != 0 {
			dbConfig.Port = port
		}
		if username, _ := cmd.Flags().GetString("db-username"); username != "" {
			dbConfig.Username = username
		}
		if password, _ := cmd.Flags().GetString("db-password"); password != "" {
			dbConfig.Password = password
		}
		if name, _ := cmd.Flags().GetString("db-name"); name != "" {
			dbConfig.Name = name
		}
		if dialect, _ := cmd.Flags().GetString("db-dialect"); dialect != "" {
			dbConfig.Dialect = dialect
		}
		if sslMode, _ := cmd.Flags().GetString("db-ssl-mode"); sslMode != "" {
			dbConfig.SSL.Mode = sslMode
		}
		if debug, _ := cmd.Flags().GetBool("db-debug"); cmd.Flags().Changed("db-debug") {
			dbConfig.Debug = debug
		}
		if maxConn, _ := cmd.Flags().GetInt("db-max-open-connections"); cmd.Flags().Changed("db-max-open-connections") {
			dbConfig.Pool.MaxConnections = maxConn
		}
		if rootCert, _ := cmd.Flags().GetString("db-root-cert-file"); rootCert != "" {
			dbConfig.SSL.RootCertFile = rootCert
		}

		// Use old configuration loading (reads from *_FILE env vars)
		// This happens AFTER flag binding, so file-based secrets take precedence
		err := dbConfig.ReadFiles()
		if err != nil {
			logger.WithError(ctx, err).Error("Failed to read database configuration files")
			os.Exit(1)
		}

		finalDBConfig = dbConfig
	}

	// Run migration with the selected configuration
	if err := runMigrateWithError(ctx, finalDBConfig); err != nil {
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
