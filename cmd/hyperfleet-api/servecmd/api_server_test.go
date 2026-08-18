package servecmd

import (
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
)

// TestBuildAPIServer_TenantMiddlewareWiredWhenEnabled guards the composition-root
// wiring in BuildAPIServer: when cfg.Server.Tenant.Enabled is true, the tenant
// middleware must actually be appended to the auth chain and enforced. Without
// this test, a broken or dropped wiring (e.g. reordered/removed the append)
// would ship silently since pkg/tenant's own tests only exercise the middleware
// in isolation, never through BuildAPIServer.
//
// resourceService, adapterStatusService, schemaValidator, and sessionFactory
// are all nil: the tenant middleware runs in authMiddleware, ahead of
// protectedAPIMiddleware and the entity handlers, so a request rejected for a
// missing required tenant header never reaches code that would dereference them.
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

	apiServer, err := BuildAPIServer(cfg, nil, nil, nil, nil, nil, false)
	Expect(err).NotTo(HaveOccurred())

	listener, err := apiServer.Listen()
	Expect(err).NotTo(HaveOccurred())
	go apiServer.Serve(listener)
	t.Cleanup(func() { _ = apiServer.Stop() })

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
