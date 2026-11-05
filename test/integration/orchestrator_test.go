package integration

import (
	"context"
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/adapters"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/chains"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/orchestrator"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

func TestOrchestratorWritesAdapterStatusForNewCluster(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	createResp, err := client.PostClusterWithResponse(ctx, openapi.PostClusterJSONRequestBody{
		Kind: "Cluster",
		Name: h.NewID(),
		Spec: openapi.ClusterSpec{"topology": "hosted", "platform": "coreshift"},
	}, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(createResp.StatusCode()).To(Equal(http.StatusCreated))
	Expect(createResp.JSON201).NotTo(BeNil())

	registry := adapters.NewRegistry(map[string]adapters.Factory{
		"coreshift": func() adapters.Controller {
			return adapters.NoopController{AdapterName: "coreshift"}
		},
	})
	executor := adapters.NewExecutor(registry, adapters.NewServiceReporter(h.Container.ResourceService()))
	loop := orchestrator.NewLoop(
		orchestrator.NewDAOClusterLister(h.Container.GenericDao()),
		executor,
		[]chains.Chain{{Topology: "hosted", Platform: "coreshift", Adapters: []string{"coreshift"}}},
		time.Second,
	)

	loop.Tick(context.Background())

	statusResp, err := client.GetClusterStatusesWithResponse(
		ctx, *createResp.JSON201.Id, nil, test.WithAuthToken(ctx),
	)
	Expect(err).NotTo(HaveOccurred())
	Expect(statusResp.StatusCode()).To(Equal(http.StatusOK))
	Expect(statusResp.JSON200).NotTo(BeNil())
	Expect(statusResp.JSON200.Items).To(HaveLen(1))
	Expect(statusResp.JSON200.Items[0].Adapter).To(Equal("coreshift"))
	Expect(statusResp.JSON200.Items[0].ObservedGeneration).To(Equal(int32(1)))
}

func TestDAOClusterListerSurfacesFinalizingClusters(t *testing.T) {
	h, client := test.RegisterIntegration(t)
	account := h.NewRandAccount()
	ctx := h.NewAuthenticatedContext(account)

	createResp, err := client.PostClusterWithResponse(ctx, openapi.PostClusterJSONRequestBody{
		Kind: "Cluster",
		Name: h.NewID(),
		Spec: openapi.ClusterSpec{"topology": "hosted", "platform": "coreshift"},
	}, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(createResp.StatusCode()).To(Equal(http.StatusCreated))
	clusterID := *createResp.JSON201.Id

	t.Cleanup(func() {
		_, _ = client.ForceDeleteClusterWithResponse(ctx, clusterID,
			openapi.ForceDeleteClusterJSONRequestBody{Reason: "test cleanup"}, test.WithAuthToken(ctx))
	})

	deleteResp, err := client.DeleteClusterByIdWithResponse(ctx, clusterID, test.WithAuthToken(ctx))
	Expect(err).NotTo(HaveOccurred())
	Expect(deleteResp.StatusCode()).To(Equal(http.StatusAccepted))

	clusters, err := orchestrator.NewDAOClusterLister(h.Container.GenericDao()).ListClusters(context.Background())
	Expect(err).NotTo(HaveOccurred())

	var finalizing *api.Resource
	for _, cluster := range clusters {
		if cluster.ID == clusterID {
			finalizing = cluster
		}
	}
	Expect(finalizing).NotTo(BeNil(), "soft-deleted cluster must stay visible to the loop")
	Expect(finalizing.DeletedTime).NotTo(BeNil())
}
