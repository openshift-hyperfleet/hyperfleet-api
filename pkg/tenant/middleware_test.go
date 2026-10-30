package tenant

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
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

// claimsTestConfig mirrors the deployed edge+api shape: claims enabled, and
// the onprem model where the claim names differ from the tenancy keys.
func claimsTestConfig() config.TenantConfig {
	cfg := testConfig()
	cfg.ClaimsEnabled = true
	cfg.Dimensions = []config.TenantDimension{
		{Header: "X-HyperFleet-Org", Claim: "org_id", Key: "org", Required: true},
		{Header: "X-HyperFleet-Project", Claim: "project_id", Key: "project", Required: false},
	}
	return cfg
}

// serveWithToken is serve with a validated token already in the request
// context, as the JWT middleware would have set it.
func serveWithToken(
	handler http.Handler, path string, headers map[string]string, claims jwt.MapClaims,
) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	req = req.WithContext(auth.SetJWTTokenContext(req.Context(), token))
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	return rr
}

func TestResolver_ResolveTenantClaimsMode(t *testing.T) {
	RegisterTestingT(t)

	var resolved *ResolvedTenant
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resolved = FromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	t.Run("valid wristband scopes by claims and ignores forged headers", func(t *testing.T) {
		RegisterTestingT(t)
		resolved = nil
		mw := NewResolver(claimsTestConfig()).ResolveTenant(next)
		rr := serveWithToken(mw, "/api/hyperfleet/v1/clusters",
			map[string]string{
				"X-HyperFleet-Org":     "forged-org",
				"X-HyperFleet-Project": "forged-project",
				"X-HyperFleet-System":  "false",
			},
			jwt.MapClaims{"hf_system": "false", "org_id": "acme", "project_id": "platform"})
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(resolved).NotTo(BeNil())
		Expect(resolved.System).To(BeFalse())
		Expect(resolved.Dimensions).To(Equal(map[string]string{"org": "acme", "project": "platform"}))
	})

	t.Run("system identity comes from the hf_system claim, not the header", func(t *testing.T) {
		RegisterTestingT(t)
		resolved = nil
		mw := NewResolver(claimsTestConfig()).ResolveTenant(next)
		rr := serveWithToken(mw, "/api/hyperfleet/v1/clusters",
			map[string]string{"X-HyperFleet-System": "false"},
			jwt.MapClaims{"hf_system": "true"})
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(resolved).NotTo(BeNil())
		Expect(resolved.System).To(BeTrue())
		Expect(resolved.Dimensions).To(BeNil())
	})

	t.Run("optional claim omitted still resolves", func(t *testing.T) {
		RegisterTestingT(t)
		resolved = nil
		mw := NewResolver(claimsTestConfig()).ResolveTenant(next)
		rr := serveWithToken(mw, "/api/hyperfleet/v1/clusters", nil,
			jwt.MapClaims{"hf_system": "false", "org_id": "acme"})
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(resolved.Dimensions).To(Equal(map[string]string{"org": "acme"}))
	})

	t.Run("dimension claim defaults to the key when not configured", func(t *testing.T) {
		RegisterTestingT(t)
		resolved = nil
		// oracle-style: claim name equals the key, claim left unset.
		cfg := config.TenantConfig{
			Enabled:       true,
			ClaimsEnabled: true,
			SystemHeader:  "X-HyperFleet-System",
			Dimensions: []config.TenantDimension{
				{Header: "X-HyperFleet-Tenancy", Key: "tenancy_ocid", Required: true},
			},
		}
		mw := NewResolver(cfg).ResolveTenant(next)
		rr := serveWithToken(mw, "/api/hyperfleet/v1/clusters", nil,
			jwt.MapClaims{"hf_system": "false", "tenancy_ocid": "ocid1.tenancy"})
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(resolved.Dimensions).To(Equal(map[string]string{"tenancy_ocid": "ocid1.tenancy"}))
	})

	forbiddenCases := []struct {
		claims jwt.MapClaims
		name   string
	}{
		{claims: jwt.MapClaims{"hf_system": "false", "project_id": "platform"}, name: "required claim missing"},
		{claims: jwt.MapClaims{"hf_system": "false", "org_id": ""}, name: "required claim present but empty"},
		{claims: jwt.MapClaims{"hf_system": "false", "org_id": "acme/<script>"},
			name: "claim value with disallowed characters"},
		{claims: jwt.MapClaims{"hf_system": "false", "org_id": strings.Repeat("a", maxDimensionValueLen+1)},
			name: "claim value exceeding max length"},
		{claims: jwt.MapClaims{"hf_system": "false"}, name: "zero claims resolved"},
	}
	for _, tc := range forbiddenCases {
		t.Run(tc.name+" returns 403 problem+json", func(t *testing.T) {
			RegisterTestingT(t)
			resolved = nil
			mw := NewResolver(claimsTestConfig()).ResolveTenant(next)
			rr := serveWithToken(mw, "/api/hyperfleet/v1/clusters", nil, tc.claims)
			Expect(rr.Code).To(Equal(http.StatusForbidden))
			Expect(resolved).To(BeNil())
			Expect(rr.Header().Get("Content-Type")).To(ContainSubstring("application/problem+json"))

			var body map[string]any
			Expect(json.NewDecoder(rr.Body).Decode(&body)).To(Succeed())
			Expect(body["code"]).To(Equal("HYPERFLEET-AUZ-001"))
			Expect(body["status"]).To(BeNumerically("==", 403))
		})
	}

	t.Run("claims enabled but no validated token falls back to headers (edge)", func(t *testing.T) {
		RegisterTestingT(t)
		resolved = nil
		mw := NewResolver(claimsTestConfig()).ResolveTenant(next)
		rr := serve(mw, "/api/hyperfleet/v1/clusters",
			map[string]string{"X-HyperFleet-Org": "acme", "X-HyperFleet-Project": "platform"})
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(resolved.Dimensions).To(Equal(map[string]string{"org": "acme", "project": "platform"}))
	})

	t.Run("claims disabled with a validated token uses headers (api mode)", func(t *testing.T) {
		RegisterTestingT(t)
		resolved = nil
		mw := NewResolver(testConfig()).ResolveTenant(next)
		rr := serveWithToken(mw, "/api/hyperfleet/v1/clusters",
			map[string]string{"X-HyperFleet-Org": "acme"},
			jwt.MapClaims{"email": "user@example.com"})
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(resolved.Dimensions).To(Equal(map[string]string{"org": "acme"}))
	})
}
