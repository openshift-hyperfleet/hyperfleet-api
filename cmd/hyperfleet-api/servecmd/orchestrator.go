package servecmd

import (
	"context"
	"log/slog"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/container"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/adapters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/closer"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/orchestrator"
)

// coreshiftAdapterName is the chain name the dev config declares. The real
// adapter arrives later; the no-op controller stands in under that name.
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

	registry := adapters.NewRegistry(map[string]adapters.Factory{
		adapters.NoopAdapterName: func() adapters.Controller { return adapters.NoopController{} },
		coreshiftAdapterName: func() adapters.Controller {
			return adapters.NoopController{AdapterName: coreshiftAdapterName}
		},
	})
	executor := adapters.NewExecutor(registry, adapters.NewServiceReporter(ctr.ResourceService()))
	loop := orchestrator.NewLoop(
		orchestrator.NewDAOClusterLister(ctr.GenericDao()),
		executor,
		cfg.Chains,
		cfg.Orchestrator.PollInterval,
	)

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
