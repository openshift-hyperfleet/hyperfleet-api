// Package orchestrator contains the in-server drift detection and
// reconciliation loop.
package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/adapters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/chains"
)

const (
	reconcileDebounce   = 10 * time.Second
	reconcileMaxAge     = 30 * time.Minute
	defaultPollInterval = 5 * time.Second
)

// ClusterLister returns the clusters the loop should inspect.
type ClusterLister interface {
	ListClusters(ctx context.Context) (api.ResourceList, error)
}

// ChainExecutor runs an ordered adapter chain against one cluster.
type ChainExecutor interface {
	Run(ctx context.Context, cluster adapters.ClusterView, chain []string) error
}

// Loop polls for drifted clusters and runs their adapter chains.
type Loop struct {
	clusters     ClusterLister
	executor     ChainExecutor
	now          func() time.Time
	chainConfig  []chains.Chain
	pollInterval time.Duration
}

// NewLoop returns a loop that polls every pollInterval.
func NewLoop(
	clusters ClusterLister, executor ChainExecutor, chainConfig []chains.Chain, pollInterval time.Duration,
) *Loop {
	if pollInterval <= 0 {
		pollInterval = defaultPollInterval
	}
	return &Loop{
		clusters:     clusters,
		executor:     executor,
		chainConfig:  chainConfig,
		pollInterval: pollInterval,
		now:          time.Now,
	}
}

// Run ticks immediately, then on every poll interval until ctx is canceled.
func (l *Loop) Run(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	l.Tick(ctx)

	ticker := time.NewTicker(l.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.Tick(ctx)
		}
	}
}

// Tick runs one pass over all clusters. Per-cluster failures are logged and
// never stop the pass.
func (l *Loop) Tick(ctx context.Context) {
	clusters, err := l.clusters.ListClusters(ctx)
	if err != nil {
		slog.ErrorContext(ctx, "Orchestrator failed to list clusters", "error", err)
		return
	}

	now := l.now()
	for _, cluster := range clusters {
		if cluster == nil {
			continue
		}
		if err := l.reconcile(ctx, cluster, now); err != nil {
			slog.ErrorContext(ctx, "Orchestrator failed to reconcile cluster",
				"cluster_id", cluster.ID, "error", err)
		}
	}
}

func (l *Loop) reconcile(ctx context.Context, cluster *api.Resource, now time.Time) error {
	spec, err := decodeSpec(cluster)
	if err != nil {
		return err
	}

	// Skip clusters with no chain before drift detection: rule 1 would re-fire
	// them on every tick forever.
	chain, ok := chains.LookupSpec(l.chainConfig, spec)
	if !ok {
		return nil
	}
	if !NeedsReconcile(cluster, now, reconcileDebounce, reconcileMaxAge) {
		return nil
	}

	return l.executor.Run(ctx, adapters.ClusterView{
		ID:         cluster.ID,
		Name:       cluster.Name,
		Generation: cluster.Generation,
		Deleting:   cluster.DeletedTime != nil,
		Spec:       spec,
	}, chain)
}

func decodeSpec(cluster *api.Resource) (map[string]interface{}, error) {
	if len(cluster.Spec) == 0 {
		return nil, nil
	}
	var spec map[string]interface{}
	if err := json.Unmarshal(cluster.Spec, &spec); err != nil {
		return nil, fmt.Errorf("decode spec for cluster %s: %w", cluster.ID, err)
	}
	return spec, nil
}
