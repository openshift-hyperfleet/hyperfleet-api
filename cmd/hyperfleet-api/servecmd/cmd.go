package servecmd

import (
	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
)

func NewServeCommand() *cobra.Command {
	// Create viper instance for this command (isolated from other commands)
	v := config.NewCommandConfig()

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the hyperfleet",
		Long:  "Serve the hyperfleet.",
		Run: func(cmd *cobra.Command, args []string) {
			// v is captured in closure, available here
			runServeWithViper(v, cmd, args)
		},
	}

	// Create config and configure flags (defines and binds in one step)
	serveConfig := config.NewServeConfig()
	serveConfig.ConfigureFlags(v, cmd.PersistentFlags())

	return cmd
}

func runServeWithViper(v *viper.Viper, cmd *cobra.Command, args []string) {
	// Load configuration using command's viper instance
	serveConfig, err := config.LoadServeConfig(v, cmd.Flags())
	if err != nil {
		glog.Fatalf("Failed to load configuration: %v", err)
	}

	// Display merged configuration
	serveConfig.DisplayConfig()

	// Convert to ApplicationConfig for environment initialization
	appConfig := serveConfig.ToApplicationConfig()

	// Initialize environment with loaded config
	err = environments.Environment().Initialize(appConfig)
	if err != nil {
		glog.Fatalf("Unable to initialize environment: %s", err.Error())
	}

	// Run the servers
	go func() {
		apiserver := server.NewAPIServer()
		apiserver.Start()
	}()

	go func() {
		metricsServer := server.NewMetricsServer()
		metricsServer.Start()
	}()

	go func() {
		healthcheckServer := server.NewHealthCheckServer()
		healthcheckServer.Start()
	}()

	// REMOVED: ControllersServer - Sentinel handles orchestration
	// Controllers are no longer run inside the API service

	select {}
}
