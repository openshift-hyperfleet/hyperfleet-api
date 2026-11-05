package orchestrator

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/adapters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/chains"
)

type fakeLister struct {
	err      error
	clusters api.ResourceList
	calls    int
}

func (f *fakeLister) ListClusters(context.Context) (api.ResourceList, error) {
	f.calls++
	return f.clusters, f.err
}

type executedChain struct {
	id    string
	chain []string
}

type fakeExecutor struct {
	errByID map[string]error
	calls   []executedChain
}

func (f *fakeExecutor) Run(_ context.Context, cluster adapters.ClusterView, chain []string) error {
	f.calls = append(f.calls, executedChain{id: cluster.ID, chain: append([]string(nil), chain...)})
	return f.errByID[cluster.ID]
}

var hostedCoreshift = []chains.Chain{{
	Topology: "hosted",
	Platform: "coreshift",
	Adapters: []string{"coreshift"},
}}

func newCluster(id string, spec string) *api.Resource {
	return &api.Resource{
		Meta:       api.Meta{ID: id},
		Kind:       api.ResourceTypeCluster,
		Name:       id,
		Spec:       []byte(spec),
		Generation: 1,
	}
}

func TestLoopRunsImmediatelyAndStopsOnCancel(t *testing.T) {
	RegisterTestingT(t)
	ticked := make(chan struct{}, 1)
	lister := &notifyingLister{ticked: ticked}
	loop := NewLoop(lister, &fakeExecutor{}, nil, time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		loop.Run(ctx)
	}()

	Eventually(ticked).Should(Receive())
	cancel()
	Eventually(done).Should(BeClosed())
}

type notifyingLister struct {
	ticked chan struct{}
}

func (l *notifyingLister) ListClusters(context.Context) (api.ResourceList, error) {
	l.ticked <- struct{}{}
	return nil, nil
}

func TestLoopSkipsClustersWithoutChainBeforeDrift(t *testing.T) {
	RegisterTestingT(t)
	cluster := newCluster("cluster-1", `{"topology": "edge", "platform": "other"}`)
	lister := &fakeLister{clusters: api.ResourceList{cluster}}
	executor := &fakeExecutor{}
	loop := NewLoop(lister, executor, hostedCoreshift, time.Second)

	loop.Tick(context.Background())

	Expect(executor.calls).To(BeEmpty())
}

func TestLoopRunsDriftedClusterChain(t *testing.T) {
	RegisterTestingT(t)
	cluster := newCluster("cluster-1", `{"topology": "hosted", "platform": "coreshift"}`)
	lister := &fakeLister{clusters: api.ResourceList{cluster}}
	executor := &fakeExecutor{}
	loop := NewLoop(lister, executor, hostedCoreshift, time.Second)

	loop.Tick(context.Background())

	Expect(executor.calls).To(Equal([]executedChain{{
		id:    "cluster-1",
		chain: []string{"coreshift"},
	}}))
}

func TestLoopSkipsUndriftedClusters(t *testing.T) {
	RegisterTestingT(t)
	now := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	cluster := newCluster("cluster-1", `{"topology": "hosted", "platform": "coreshift"}`)
	cluster.Generation = 2
	cluster.Conditions = []api.ResourceCondition{{
		Type:               api.ResourceConditionTypeReconciled,
		Status:             api.ConditionTrue,
		ObservedGeneration: 2,
		LastUpdatedTime:    now,
	}}
	lister := &fakeLister{clusters: api.ResourceList{cluster}}
	executor := &fakeExecutor{}
	loop := NewLoop(lister, executor, hostedCoreshift, time.Second)
	loop.now = func() time.Time { return now }

	loop.Tick(context.Background())

	Expect(executor.calls).To(BeEmpty())
}

func TestLoopIsolatesClusterErrors(t *testing.T) {
	RegisterTestingT(t)
	bad := newCluster("bad", `{"topology": "hosted", "platform": "coreshift"}`)
	good := newCluster("good", `{"topology": "hosted", "platform": "coreshift"}`)
	lister := &fakeLister{clusters: api.ResourceList{bad, good}}
	executor := &fakeExecutor{errByID: map[string]error{
		"bad": stderrors.New("boom"),
	}}
	loop := NewLoop(lister, executor, hostedCoreshift, time.Second)

	loop.Tick(context.Background())

	Expect(executor.calls).To(HaveLen(2))
	Expect(executor.calls[0].id).To(Equal("bad"))
	Expect(executor.calls[1].id).To(Equal("good"))
}
