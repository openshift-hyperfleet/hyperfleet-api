package migrate

import (
	"context"
	"flag"

	"github.com/golang/glog"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/db/db_session"
	"github.com/spf13/cobra"

	"github.com/openshift-hyperfleet/hyperfleet/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/db"
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
	err := dbConfig.ReadFiles()
	if err != nil {
		glog.Fatal(err)
	}

	connection := db_session.NewProdFactory(dbConfig)
	if err := db.Migrate(connection.New(context.Background())); err != nil {
		glog.Fatal(err)
	}
}
