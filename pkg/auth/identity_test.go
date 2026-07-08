package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
)

func TestCallerIdentityFromRequest(t *testing.T) {
	tests := []struct {
		claims      jwt.MapClaims
		issuerCfg   *config.JWTIssuerConfig
		name        string
		want        string
		headerValue string
		setHeader   bool
		wantErr     bool
	}{
		{
			name:   "resolves identity from JWT email claim",
			claims: jwt.MapClaims{"email": "jwt@example.com"},
			issuerCfg: &config.JWTIssuerConfig{
				IdentityClaim: "email",
			},
			want: "jwt@example.com",
		},
		{
			name:   "resolves identity from custom claim",
			claims: jwt.MapClaims{"sub": "subject-id", "email": "jwt@example.com"},
			issuerCfg: &config.JWTIssuerConfig{
				IdentityClaim: "sub",
			},
			want: "subject-id",
		},
		{
			name:      "no issuer config in context returns empty",
			claims:    jwt.MapClaims{"email": "jwt@example.com"},
			issuerCfg: nil,
			want:      "",
		},
		{
			name:   "empty identity_claim returns empty",
			claims: jwt.MapClaims{"email": "jwt@example.com"},
			issuerCfg: &config.JWTIssuerConfig{
				IdentityClaim: "",
			},
			want: "",
		},
		{
			name:   "missing claim returns error",
			claims: jwt.MapClaims{"email": "jwt@example.com"},
			issuerCfg: &config.JWTIssuerConfig{
				IdentityClaim: "missing_claim",
			},
			wantErr: true,
		},
		{
			name:   "rejects oversized JWT claim value",
			claims: jwt.MapClaims{"email": strings.Repeat("x", maxCallerIdentityLen+1)},
			issuerCfg: &config.JWTIssuerConfig{
				IdentityClaim: "email",
			},
			wantErr: true,
		},
		{
			name:   "trims whitespace from JWT claim value",
			claims: jwt.MapClaims{"email": "  user@example.com  "},
			issuerCfg: &config.JWTIssuerConfig{
				IdentityClaim: "email",
			},
			want: "user@example.com",
		},
		{
			name:   "identity_claim_pattern match passes",
			claims: jwt.MapClaims{"email": "user@example.com"},
			issuerCfg: &config.JWTIssuerConfig{
				IdentityClaim:        "email",
				IdentityClaimPattern: `^[^@]+@example\.com$`,
				CompiledPattern:      regexp.MustCompile(`^[^@]+@example\.com$`),
			},
			want: "user@example.com",
		},
		{
			name:   "identity_claim_pattern mismatch returns error",
			claims: jwt.MapClaims{"email": "user@other.com"},
			issuerCfg: &config.JWTIssuerConfig{
				IdentityClaim:        "email",
				IdentityClaimPattern: `^[^@]+@example\.com$`,
				CompiledPattern:      regexp.MustCompile(`^[^@]+@example\.com$`),
			},
			wantErr: true,
		},
		{
			name:        "header overrides JWT claim",
			claims:      jwt.MapClaims{"email": "jwt@example.com"},
			setHeader:   true,
			headerValue: "gateway-user@example.com",
			issuerCfg:   &config.JWTIssuerConfig{IdentityClaim: "email", IdentityHeader: "X-HyperFleet-Identity"},
			want:        "gateway-user@example.com",
		},
		{
			name:        "empty header falls back to JWT",
			claims:      jwt.MapClaims{"email": "jwt@example.com"},
			setHeader:   true,
			headerValue: "",
			issuerCfg:   &config.JWTIssuerConfig{IdentityClaim: "email", IdentityHeader: "X-HyperFleet-Identity"},
			want:        "jwt@example.com",
		},
		{
			name:        "rejects invalid header value",
			setHeader:   true,
			headerValue: "bad\x00value",
			issuerCfg:   &config.JWTIssuerConfig{IdentityHeader: "X-HyperFleet-Identity"},
			wantErr:     true,
		},
		{
			name:        "rejects header value exceeding max length",
			setHeader:   true,
			headerValue: strings.Repeat("a", maxCallerIdentityLen+1),
			issuerCfg:   &config.JWTIssuerConfig{IdentityHeader: "X-HyperFleet-Identity"},
			wantErr:     true,
		},
		{
			name:        "header at exact max length accepted",
			setHeader:   true,
			headerValue: strings.Repeat("a", maxCallerIdentityLen),
			issuerCfg:   &config.JWTIssuerConfig{IdentityHeader: "X-HyperFleet-Identity"},
			want:        strings.Repeat("a", maxCallerIdentityLen),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			r := httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/clusters", nil)
			ctx := r.Context()
			if tc.claims != nil {
				ctx = contextWithClaims(tc.claims)
			}
			if tc.issuerCfg != nil {
				ctx = SetJWTIssuerConfigContext(ctx, *tc.issuerCfg)
			}
			r = r.WithContext(ctx)
			if tc.setHeader && tc.issuerCfg != nil {
				r.Header.Set(tc.issuerCfg.IdentityHeader, tc.headerValue)
			}

			identity, err := CallerIdentityFromRequest(r.Context(), r)
			if tc.wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).NotTo(HaveOccurred())
			Expect(identity).To(Equal(tc.want))
		})
	}
}

func TestNewCallerIdentityMiddleware(t *testing.T) {
	RegisterTestingT(t)

	t.Run("returns middleware", func(t *testing.T) {
		RegisterTestingT(t)
		mw := NewCallerIdentityMiddleware()
		Expect(mw).NotTo(BeNil())
	})
}

func TestResolveCallerIdentityMiddleware(t *testing.T) {
	RegisterTestingT(t)

	issuerCfg := config.JWTIssuerConfig{IdentityClaim: "email"}
	issuerCfgWithHeader := config.JWTIssuerConfig{
		IdentityClaim:  "email",
		IdentityHeader: "X-HyperFleet-Identity",
	}

	contextWithIssuerAndClaims := func(claims jwt.MapClaims) context.Context {
		ctx := contextWithClaims(claims)
		return SetJWTIssuerConfigContext(ctx, issuerCfg)
	}

	contextWithIssuerHeaderAndClaims := func(claims jwt.MapClaims) context.Context {
		ctx := contextWithClaims(claims)
		return SetJWTIssuerConfigContext(ctx, issuerCfgWithHeader)
	}

	newMW := NewCallerIdentityMiddleware

	t.Run("skips openapi paths", func(t *testing.T) {
		RegisterTestingT(t)
		mw := newMW()

		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			Expect(GetUsernameFromContext(r.Context())).To(BeEmpty())
		})

		r := httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/openapi", nil)
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(called).To(BeTrue())
		Expect(w.Code).To(Equal(http.StatusOK))
	})

	t.Run("allows GET without identity", func(t *testing.T) {
		RegisterTestingT(t)
		mw := newMW()

		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			Expect(GetUsernameFromContext(r.Context())).To(BeEmpty())
		})

		r := httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/clusters", nil)
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(called).To(BeTrue())
		Expect(w.Code).To(Equal(http.StatusOK))
	})

	t.Run("returns 401 on POST without identity", func(t *testing.T) {
		RegisterTestingT(t)
		mw := newMW()

		nextCalled := false
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			nextCalled = true
		})

		r := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", nil)
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(w.Code).To(Equal(http.StatusUnauthorized))
		Expect(nextCalled).To(BeFalse())
	})

	t.Run("returns 401 on PATCH without identity", func(t *testing.T) {
		RegisterTestingT(t)
		mw := newMW()

		nextCalled := false
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			nextCalled = true
		})

		r := httptest.NewRequest(http.MethodPatch, "/api/hyperfleet/v1/clusters/123", nil)
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(w.Code).To(Equal(http.StatusUnauthorized))
		Expect(nextCalled).To(BeFalse())
	})

	t.Run("returns 401 on DELETE without identity", func(t *testing.T) {
		RegisterTestingT(t)
		mw := newMW()

		nextCalled := false
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			nextCalled = true
		})

		r := httptest.NewRequest(http.MethodDelete, "/api/hyperfleet/v1/clusters/123", nil)
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(w.Code).To(Equal(http.StatusUnauthorized))
		Expect(nextCalled).To(BeFalse())
	})

	t.Run("returns 401 on PUT without identity", func(t *testing.T) {
		RegisterTestingT(t)
		mw := newMW()

		nextCalled := false
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			nextCalled = true
		})

		r := httptest.NewRequest(http.MethodPut, "/api/hyperfleet/v1/clusters/123", nil)
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(w.Code).To(Equal(http.StatusUnauthorized))
		Expect(nextCalled).To(BeFalse())
	})

	t.Run("allows POST with identity from JWT", func(t *testing.T) {
		RegisterTestingT(t)
		mw := newMW()

		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			Expect(GetUsernameFromContext(r.Context())).To(Equal("jwt@example.com"))
		})

		r := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", nil)
		r = r.WithContext(contextWithIssuerAndClaims(jwt.MapClaims{"email": "jwt@example.com"}))
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(called).To(BeTrue())
		Expect(w.Code).To(Equal(http.StatusOK))
	})

	t.Run("allows POST with identity from header", func(t *testing.T) {
		RegisterTestingT(t)
		mw := newMW()

		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			Expect(GetUsernameFromContext(r.Context())).To(Equal("user@example.com"))
		})

		r := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", nil)
		r = r.WithContext(SetJWTIssuerConfigContext(r.Context(), issuerCfgWithHeader))
		r.Header.Set("X-HyperFleet-Identity", "user@example.com")
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(called).To(BeTrue())
		Expect(w.Code).To(Equal(http.StatusOK))
	})

	t.Run("header identity takes precedence over JWT on POST", func(t *testing.T) {
		RegisterTestingT(t)
		mw := newMW()

		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			Expect(GetUsernameFromContext(r.Context())).To(Equal("header@gateway.com"))
		})

		r := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", nil)
		r = r.WithContext(contextWithIssuerHeaderAndClaims(jwt.MapClaims{"email": "jwt@example.com"}))
		r.Header.Set("X-HyperFleet-Identity", "header@gateway.com")
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(called).To(BeTrue())
		Expect(w.Code).To(Equal(http.StatusOK))
	})

	t.Run("POST with empty header falls back to JWT", func(t *testing.T) {
		RegisterTestingT(t)
		mw := newMW()

		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			Expect(GetUsernameFromContext(r.Context())).To(Equal("jwt@example.com"))
		})

		r := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", nil)
		r = r.WithContext(contextWithIssuerHeaderAndClaims(jwt.MapClaims{"email": "jwt@example.com"}))
		r.Header.Set("X-HyperFleet-Identity", "")
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(called).To(BeTrue())
		Expect(w.Code).To(Equal(http.StatusOK))
	})

	t.Run("returns 401 on POST with oversized header", func(t *testing.T) {
		RegisterTestingT(t)
		mw := newMW()

		nextCalled := false
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			nextCalled = true
		})

		r := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", nil)
		r = r.WithContext(SetJWTIssuerConfigContext(r.Context(), issuerCfgWithHeader))
		r.Header.Set("X-HyperFleet-Identity", strings.Repeat("a", maxCallerIdentityLen+1))
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(w.Code).To(Equal(http.StatusUnauthorized))
		Expect(nextCalled).To(BeFalse())
	})

	t.Run("returns 401 on POST with oversized JWT claim", func(t *testing.T) {
		RegisterTestingT(t)
		mw := newMW()

		nextCalled := false
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			nextCalled = true
		})

		r := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", nil)
		r = r.WithContext(contextWithIssuerAndClaims(jwt.MapClaims{
			"email": strings.Repeat("a", maxCallerIdentityLen+1),
		}))
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(w.Code).To(Equal(http.StatusUnauthorized))
		Expect(nextCalled).To(BeFalse())
	})
}
