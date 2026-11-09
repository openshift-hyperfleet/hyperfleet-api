package services

import (
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/chains"
)

func TestRequiredAdaptersUsesClusterChain(t *testing.T) {
	RegisterTestingT(t)
	svc := &sqlResourceService{chains: []chains.Chain{
		{Topology: "hosted", Platform: "coreshift", Adapters: []string{"coreshift"}},
		{Topology: "hosted", Platform: "etcd", Adapters: []string{"etcd"}},
	}}

	coreshift := &api.Resource{
		Kind: api.ResourceTypeCluster,
		Spec: []byte(`{"topology":"hosted","platform":"coreshift"}`),
	}
	Expect(svc.requiredAdapters(coreshift)).To(Equal([]string{"coreshift"}))

	etcd := &api.Resource{
		Kind: api.ResourceTypeCluster,
		Spec: []byte(`{"topology":"hosted","platform":"etcd"}`),
	}
	Expect(svc.requiredAdapters(etcd)).To(Equal([]string{"etcd"}))
}
