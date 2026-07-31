package tenant

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/onsi/gomega"
)

func testConfig() TenantConfig {
	return TenantConfig{
		Enabled:      true,
		SystemHeader: "X-HyperFleet-System",
		Dimensions: []DimensionConfig{
			{Header: "X-Tenant-Org", Key: "org", Required: true},
			{Header: "X-Tenant-Project", Key: "project", Required: false},
		},
	}
}

// serve runs a request through the middleware and captures the resolved
// tenant seen by the downstream handler, if it was reached.
func serve(cfg TenantConfig, mutate func(*http.Request)) (*httptest.ResponseRecorder, *ResolvedTenant, bool) {
	var got *ResolvedTenant
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		got = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/clusters", nil)
	if mutate != nil {
		mutate(req)
	}
	rec := httptest.NewRecorder()
	NewMiddleware(cfg).EnforceTenant(next).ServeHTTP(rec, req)
	return rec, got, reached
}

func TestEnforceTenantResolvesDimensions(t *testing.T) {
	RegisterTestingT(t)

	rec, got, reached := serve(testConfig(), func(r *http.Request) {
		r.Header.Set("X-Tenant-Org", "acme")
		r.Header.Set("X-Tenant-Project", "proj-1")
	})

	Expect(reached).To(BeTrue())
	Expect(rec.Code).To(Equal(http.StatusOK))
	Expect(got).ToNot(BeNil())
	Expect(got.System).To(BeFalse())
	Expect(got.Dimensions).To(Equal(map[string]string{"org": "acme", "project": "proj-1"}))
}

func TestEnforceTenantOptionalDimensionMayBeAbsent(t *testing.T) {
	RegisterTestingT(t)

	_, got, reached := serve(testConfig(), func(r *http.Request) {
		r.Header.Set("X-Tenant-Org", "acme")
	})

	Expect(reached).To(BeTrue())
	Expect(got.Dimensions).To(Equal(map[string]string{"org": "acme"}))
}

func TestEnforceTenantMissingRequiredHeaderForbidden(t *testing.T) {
	RegisterTestingT(t)

	rec, _, reached := serve(testConfig(), func(r *http.Request) {
		r.Header.Set("X-Tenant-Project", "proj-1")
	})

	Expect(reached).To(BeFalse())
	Expect(rec.Code).To(Equal(http.StatusForbidden))
	Expect(rec.Header().Get("Content-Type")).To(Equal("application/problem+json"))

	var body map[string]interface{}
	Expect(json.Unmarshal(rec.Body.Bytes(), &body)).To(Succeed())
	Expect(body["code"]).To(Equal("HYPERFLEET-AUT-004"))
	Expect(body["status"]).To(BeEquivalentTo(http.StatusForbidden))
	Expect(body["detail"]).To(ContainSubstring("X-Tenant-Org"))
}

func TestEnforceTenantNoDimensionsResolvedForbidden(t *testing.T) {
	RegisterTestingT(t)

	// All dimensions optional: a request with no tenant headers must still be
	// rejected, an empty tenancy map would contain-match every resource.
	cfg := testConfig()
	cfg.Dimensions = []DimensionConfig{
		{Header: "X-Tenant-Org", Key: "org", Required: false},
	}

	rec, _, reached := serve(cfg, nil)

	Expect(reached).To(BeFalse())
	Expect(rec.Code).To(Equal(http.StatusForbidden))
}

func TestEnforceTenantSystemIdentityBypasses(t *testing.T) {
	RegisterTestingT(t)

	rec, got, reached := serve(testConfig(), func(r *http.Request) {
		r.Header.Set("X-HyperFleet-System", "true")
	})

	Expect(reached).To(BeTrue())
	Expect(rec.Code).To(Equal(http.StatusOK))
	Expect(got).ToNot(BeNil())
	Expect(got.System).To(BeTrue())
	Expect(got.Dimensions).To(BeEmpty())
}

func TestEnforceTenantSystemHeaderMustBeLiteralTrue(t *testing.T) {
	RegisterTestingT(t)

	rec, _, reached := serve(testConfig(), func(r *http.Request) {
		r.Header.Set("X-HyperFleet-System", "false")
	})

	// Non-"true" system header falls through to tenant resolution, which
	// fails because the required org header is missing.
	Expect(reached).To(BeFalse())
	Expect(rec.Code).To(Equal(http.StatusForbidden))
}

func TestEnforceTenantWhitespaceOnlyHeaderIsMissing(t *testing.T) {
	RegisterTestingT(t)

	rec, _, reached := serve(testConfig(), func(r *http.Request) {
		r.Header.Set("X-Tenant-Org", "   ")
	})

	Expect(reached).To(BeFalse())
	Expect(rec.Code).To(Equal(http.StatusForbidden))
}

func TestEnforceTenantSkipsOpenAPIPath(t *testing.T) {
	RegisterTestingT(t)

	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		Expect(FromContext(r.Context())).To(BeNil())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/openapi", nil)
	rec := httptest.NewRecorder()
	NewMiddleware(testConfig()).EnforceTenant(next).ServeHTTP(rec, req)

	Expect(reached).To(BeTrue())
	Expect(rec.Code).To(Equal(http.StatusOK))
}
