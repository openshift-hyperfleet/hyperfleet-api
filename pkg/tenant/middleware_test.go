package tenant

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
)

func testConfig() config.TenantConfig {
	return config.TenantConfig{
		Enabled:      true,
		SystemHeader: "X-HyperFleet-System",
		Dimensions: []config.TenantDimension{
			{Header: "X-HyperFleet-Org", Key: "org", Required: true},
			{Header: "X-HyperFleet-Project", Key: "project", Required: false},
		},
	}
}

func TestResolver_ResolveTenant(t *testing.T) {
	RegisterTestingT(t)

	var resolved *ResolvedTenant
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resolved = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	systemCases := []struct {
		headers map[string]string
		name    string
	}{
		{
			name:    "system header alone",
			headers: map[string]string{"X-HyperFleet-System": "true"},
		},
		{
			name: "system header takes precedence over a present dimension header",
			headers: map[string]string{
				"X-HyperFleet-System": "true",
				"X-HyperFleet-Org":    "acme", // present but must be ignored once system bypass triggers
			},
		},
		{
			name:    "system header value is case-insensitive",
			headers: map[string]string{"X-HyperFleet-System": "TRUE"},
		},
	}
	for _, tc := range systemCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			resolved = nil
			mw := NewResolver(testConfig()).ResolveTenant(next)
			rr := serve(mw, "/api/hyperfleet/v1/clusters", tc.headers)
			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(resolved).NotTo(BeNil())
			Expect(resolved.System).To(BeTrue())
			Expect(resolved.Dimensions).To(BeNil())
		})
	}

	resolveCases := []struct {
		headers  map[string]string
		wantDims map[string]string
		name     string
	}{
		{
			name: "all dimensions present",
			headers: map[string]string{
				"X-HyperFleet-Org":     "acme",
				"X-HyperFleet-Project": "platform",
			},
			wantDims: map[string]string{"org": "acme", "project": "platform"},
		},
		{
			name:     "optional dimension omitted still resolves",
			headers:  map[string]string{"X-HyperFleet-Org": "acme"},
			wantDims: map[string]string{"org": "acme"},
		},
		{
			name: "whitespace-only system header falls through to normal resolution",
			headers: map[string]string{
				"X-HyperFleet-System": "   ",
				"X-HyperFleet-Org":    "acme",
			},
			wantDims: map[string]string{"org": "acme"},
		},
		{
			name: "non-true system header value falls through to normal resolution",
			headers: map[string]string{
				"X-HyperFleet-System": "false", // placeholder/truthy-looking value must not bypass scoping
				"X-HyperFleet-Org":    "acme",
			},
			wantDims: map[string]string{"org": "acme"},
		},
	}
	for _, tc := range resolveCases {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			resolved = nil
			mw := NewResolver(testConfig()).ResolveTenant(next)
			rr := serve(mw, "/api/hyperfleet/v1/clusters", tc.headers)
			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(resolved).NotTo(BeNil())
			Expect(resolved.System).To(BeFalse())
			Expect(resolved.Dimensions).To(Equal(tc.wantDims))
		})
	}

	// Config with no required dimensions bypasses config.Validate() invariants but
	// exercises the middleware's own zero-dimensions fail-closed check directly.
	zeroDimsConfig := config.TenantConfig{
		Enabled:      true,
		SystemHeader: "X-HyperFleet-System",
		Dimensions: []config.TenantDimension{
			{Header: "X-HyperFleet-Project", Key: "project", Required: false},
		},
	}

	forbiddenCases := []struct {
		headers map[string]string
		name    string
		cfg     config.TenantConfig
	}{
		{
			name:    "missing required dimension",
			cfg:     testConfig(),
			headers: map[string]string{"X-HyperFleet-Project": "platform"},
		},
		{
			name:    "whitespace-only required dimension header",
			cfg:     testConfig(),
			headers: map[string]string{"X-HyperFleet-Org": "   "},
		},
		{
			name:    "dimension value with disallowed characters",
			cfg:     testConfig(),
			headers: map[string]string{"X-HyperFleet-Org": "acme/<script>"},
		},
		{
			name:    "dimension value exceeding max length",
			cfg:     testConfig(),
			headers: map[string]string{"X-HyperFleet-Org": strings.Repeat("a", maxDimensionValueLen+1)},
		},
		{
			name: "zero dimensions resolved",
			cfg:  zeroDimsConfig,
		},
	}
	for _, tc := range forbiddenCases {
		t.Run(tc.name+" returns 403 problem+json", func(t *testing.T) {
			RegisterTestingT(t)
			resolved = nil
			mw := NewResolver(tc.cfg).ResolveTenant(next)
			rr := serve(mw, "/api/hyperfleet/v1/clusters", tc.headers)
			Expect(rr.Code).To(Equal(http.StatusForbidden))
			Expect(resolved).To(BeNil())
			Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))

			var body map[string]any
			Expect(json.NewDecoder(rr.Body).Decode(&body)).To(Succeed())
			Expect(body["code"]).To(Equal("HYPERFLEET-AUZ-001"))
			Expect(body["status"]).To(BeNumerically("==", 403))
		})
	}

	skipPaths := []string{
		"/api/hyperfleet/v1/openapi",
		"/api/hyperfleet/v1/openapi.html",
		"/api/hyperfleet/v1/errors",
		"/api/hyperfleet/v1/errors/HYPERFLEET-AUZ-001",
	}
	for _, path := range skipPaths {
		t.Run("skips tenant enforcement for "+path, func(t *testing.T) {
			RegisterTestingT(t)
			resolved = nil
			mw := NewResolver(testConfig()).ResolveTenant(next)
			rr := serve(mw, path, nil)
			Expect(rr.Code).To(Equal(http.StatusOK))
			Expect(resolved).To(BeNil())
		})
	}
}

func serve(handler http.Handler, path string, headers map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}
