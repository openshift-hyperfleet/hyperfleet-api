package migrate

import (
	"context"

	"github.com/golang/glog"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_session"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
)

// NewMigrateCommand migrate sub-command handles running migrations
func NewMigrateCommand() *cobra.Command {
	// Create viper instance for this command (isolated from other commands)
	v := config.NewCommandConfig()

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Run hyperfleet service data migrations",
		Long:  "Run hyperfleet service data migrations",
		Run: func(cmd *cobra.Command, args []string) {
			// v is captured in closure, available here
			runMigrateWithConfig(v, cmd, args)
		},
	}

	// Create config and configure flags (defines and binds in one step)
	migrateConfig := config.NewMigrateConfig()
	migrateConfig.ConfigureFlags(v, cmd.PersistentFlags())

	return cmd
}

func runMigrateWithConfig(v *viper.Viper, cmd *cobra.Command, args []string) {
	// Load configuration using command's viper instance
	migrateConfig, err := config.LoadMigrateConfig(v, cmd.Flags())
	if err != nil {
		glog.Fatalf("Failed to load configuration: %v", err)
	}

	glog.Infof("Running database migrations...")

	// Create database connection factory
	connection := db_session.NewProdFactory(migrateConfig.Database)

	// Run migrations
	if err := db.Migrate(connection.New(context.Background())); err != nil {
		glog.Fatalf("Migration failed: %v", err)
	}

	glog.Infof("Migrations completed successfully")
}
