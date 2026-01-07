package servecmd

import (
	"context"
	"log/slog"
	"os"

	"github.com/golang/glog"
	"github.com/spf13/cobra"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

func NewServeCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Serve the hyperfleet",
		Long:  "Serve the hyperfleet.",
		Run:   runServe,
	}
	err := environments.Environment().AddFlags(cmd.PersistentFlags())
	if err != nil {
		glog.Fatalf("Unable to add environment flags to serve command: %s", err.Error())
	}

	return cmd
}

func runServe(cmd *cobra.Command, args []string) {
	err := environments.Environment().Initialize()
	if err != nil {
		glog.Fatalf("Unable to initialize environment: %s", err.Error())
	}

	// Initialize slog logger (demonstration only, full migration in PR 3)
	ctx := context.Background()
	hostname, _ := os.Hostname()
	logConfig := &logger.LogConfig{
		Level:     slog.LevelInfo,
		Format:    logger.FormatJSON,
		Output:    os.Stdout,
		Component: "api",
		Version:   "dev",
		Hostname:  hostname,
	}
	logger.InitGlobalLogger(logConfig)
	logger.Info(ctx, "New slog logger initialized (example)", "log_level", "info", "log_format", "json")

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

	select {}
}
