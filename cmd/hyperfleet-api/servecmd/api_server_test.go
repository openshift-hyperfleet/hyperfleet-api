package servecmd

import (
	"context"
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
)

// TestBuildAPIServer_TenantMiddlewareWiredWhenEnabled guards the tenant
// middleware's wiring into the composition root, since pkg/tenant's own tests
// never exercise it there and wouldn't catch a dropped/reordered append.
//
// resourceService, adapterStatusService, schemaValidator, and sessionFactory
// are nil: tenant middleware runs ahead of the handlers that would need them,
// so a rejected request never reaches code that dereferences them.
func TestBuildAPIServer_TenantMiddlewareWiredWhenEnabled(t *testing.T) {
	RegisterTestingT(t)
	registry.Reset()
	t.Cleanup(registry.Reset)
	registry.Register(registry.EntityDescriptor{Kind: "Channel", Plural: "channels"})

	cfg := config.NewApplicationConfig()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 0 // ephemeral port
	cfg.Server.JWT.Enabled = false
	cfg.Server.Tenant = config.TenantConfig{
		Enabled:      true,
		SystemHeader: "X-HyperFleet-System",
		Dimensions: []config.TenantDimension{
			{Header: "X-HyperFleet-Org", Key: "org", Required: true},
		},
	}

	apiServer, err := BuildAPIServer(cfg, nil, nil, nil, nil, nil)
	Expect(err).NotTo(HaveOccurred())

	listener, err := apiServer.Listen()
	Expect(err).NotTo(HaveOccurred())

	// served closes once Serve returns, so cleanup can block until the
	// goroutine has actually exited instead of racing the next test.
	served := make(chan struct{})
	go func() {
		defer close(served)
		apiServer.Serve(listener)
	}()
	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(t.Context(), time.Second)
		defer cancel()
		Expect(apiServer.Shutdown(shutdownCtx)).To(Succeed())
		<-served
	})

	baseURL := "http://" + listener.Addr().String()

	// Missing the required tenant dimension header: tenant middleware must
	// reject with 403 before the request reaches any entity handler.
	var resp *http.Response
	Eventually(func() error {
		var getErr error
		resp, getErr = http.Get(baseURL + "/api/hyperfleet/v1/channels")
		return getErr
	}, "2s", "25ms").Should(Succeed())
	defer resp.Body.Close()

	Expect(resp.StatusCode).To(Equal(http.StatusForbidden),
		"request missing a required tenant header must be rejected by the tenant middleware")
}
