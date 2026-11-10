package servecmd

import (
	"context"
	"log/slog"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/container"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/adapters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/closer"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/fleet"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/orchestrator"
)

// coreshiftAdapterName is the chain name the dev config declares.
const coreshiftAdapterName = "coreshift"

// startOrchestrator starts the drift loop unless disabled, and registers a
// closer step that stops it and waits for the goroutine to exit.
func startOrchestrator(
	ctx context.Context, c *closer.Closer, cfg *config.ApplicationConfig, ctr *container.Container,
) {
	if !cfg.Orchestrator.Enabled {
		slog.InfoContext(ctx, "Orchestrator disabled")
		return
	}

	coreshift := adapters.NewCoreshift(adapters.CoreshiftOptions{
		Binary:         cfg.Coreshift.Binary,
		PullSecretFile: cfg.Coreshift.PullSecretFile,
		StateDir:       cfg.Coreshift.StateDir,
	})
	etcd := adapters.NewEtcdController(
		func() ([]fleet.MC, error) { return fleet.List(cfg.Etcd.FleetDir) },
		adapters.NewKubeconfigTransport(fleetKubeconfigs(ctx, cfg.Etcd.FleetDir)),
		adapters.EtcdOptions{
			Image:        cfg.Etcd.Image,
			StorageClass: cfg.Etcd.StorageClass,
			StorageSize:  cfg.Etcd.StorageSize,
			PreferredMC:  cfg.Etcd.PreferredMC,
			StateDir:     cfg.Etcd.StateDir,
		},
	).WithLiveMembership()
	registry := adapters.NewRegistry(map[string]adapters.Factory{
		adapters.NoopAdapterName: func() adapters.Controller { return adapters.NoopController{} },
		coreshiftAdapterName:     func() adapters.Controller { return coreshift },
		adapters.EtcdAdapterName: func() adapters.Controller { return etcd },
	})
	executor := adapters.NewExecutor(registry, adapters.NewServiceReporter(ctr.ResourceService()))
	loop := orchestrator.NewLoop(
		orchestrator.NewDAOClusterLister(ctr.GenericDao()),
		executor,
		cfg.Chains,
		cfg.Orchestrator.PollInterval,
	).WithMaxAge(cfg.Orchestrator.MaxAge)

	loopCtx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	c.Add(func() error {
		cancel()
		<-done
		return nil
	})
	go func() {
		defer close(done)
		loop.Run(loopCtx)
	}()

	slog.InfoContext(ctx, "Orchestrator loop started", "poll_interval", cfg.Orchestrator.PollInterval)
}

func fleetKubeconfigs(ctx context.Context, dir string) map[string]string {
	mcs, err := fleet.List(dir)
	if err != nil {
		slog.WarnContext(ctx, "Failed to read management cluster fleet", "error", err)
		return nil
	}
	kubeconfigs := make(map[string]string, len(mcs))
	for _, mc := range mcs {
		kubeconfigs[mc.Name] = mc.Kubeconfig
	}
	return kubeconfigs
}
