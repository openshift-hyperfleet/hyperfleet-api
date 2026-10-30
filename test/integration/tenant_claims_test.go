package integration

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/tenant"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

// TestTenantClaimsFromWristband chains the real JWT validation and the tenant
// resolver over HTTP, the way BuildAPIServer mounts them in edge+api mode.
func TestTenantClaimsFromWristband(t *testing.T) {
	h, _ := test.RegisterIntegration(t)

	resolver := tenant.NewResolver(config.TenantConfig{
		Enabled:       true,
		ClaimsEnabled: true,
		SystemHeader:  "x-hyperfleet-system",
		Dimensions: []config.TenantDimension{
			{Header: "x-tenant-org", Claim: "org_id", Key: "org", Required: true},
			{Header: "x-tenant-project", Claim: "project_id", Key: "project", Required: false},
		},
	})

	var resolved *tenant.ResolvedTenant
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resolved = tenant.FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	handler := h.JWTHandler().Middleware(resolver.ResolveTenant(next))
	srv := httptest.NewServer(handler)
	defer srv.Close()

	do := func(t *testing.T, wristband string, headers map[string]string) *http.Response {
		t.Helper()
		req, err := http.NewRequest(http.MethodGet, srv.URL+"/api/hyperfleet/v1/clusters", nil)
		Expect(err).NotTo(HaveOccurred())
		if wristband != "" {
			req.Header.Set("Authorization", "Bearer "+wristband)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}
		resp, err := srv.Client().Do(req)
		Expect(err).NotTo(HaveOccurred())
		t.Cleanup(func() { _ = resp.Body.Close() })
		return resp
	}

	t.Run("wristband claims scope the request and forged headers are ignored", func(t *testing.T) {
		RegisterTestingT(t)
		wristband := h.SignTokenWithClaims(jwt.MapClaims{
			"hf_system": "false", "org_id": "acme", "project_id": "platform",
		})
		resp := do(t, wristband, map[string]string{
			"x-tenant-org":        "forged-org",
			"x-tenant-project":    "forged-project",
			"x-hyperfleet-system": "false",
		})
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resolved).NotTo(BeNil())
		Expect(resolved.System).To(BeFalse())
		Expect(resolved.Dimensions).To(Equal(map[string]string{"org": "acme", "project": "platform"}))
	})

	t.Run("system identity comes from the hf_system claim", func(t *testing.T) {
		RegisterTestingT(t)
		wristband := h.SignTokenWithClaims(jwt.MapClaims{"hf_system": "true"})
		resp := do(t, wristband, map[string]string{"x-hyperfleet-system": "false"})
		Expect(resp.StatusCode).To(Equal(http.StatusOK))
		Expect(resolved).NotTo(BeNil())
		Expect(resolved.System).To(BeTrue())
		Expect(resolved.Dimensions).To(BeNil())
	})

	t.Run("wristband missing a required claim fails closed with 403", func(t *testing.T) {
		RegisterTestingT(t)
		wristband := h.SignTokenWithClaims(jwt.MapClaims{"hf_system": "false"})
		// A forged header must not rescue a wristband without the claim.
		resp := do(t, wristband, map[string]string{"x-tenant-org": "forged-org"})
		Expect(resp.StatusCode).To(Equal(http.StatusForbidden))
		Expect(resp.Header.Get("Content-Type")).To(ContainSubstring("application/problem+json"))
	})
}
