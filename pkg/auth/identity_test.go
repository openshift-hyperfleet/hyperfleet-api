package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/gomega"
)

func TestCallerIdentityFromRequest(t *testing.T) {
	tests := []struct {
		claims      jwt.MapClaims
		name        string
		headerValue string
		want        string
		cfg         CallerIdentityConfig
		setHeader   bool
		wantErr     bool
	}{
		{
			name:        "header overrides JWT claim",
			claims:      jwt.MapClaims{"email": "jwt@example.com"},
			setHeader:   true,
			headerValue: "gateway-user@example.com",
			cfg: CallerIdentityConfig{
				JWTIdentityClaim: "email",
				HeaderName:       "X-HyperFleet-Identity",
			},
			want: "gateway-user@example.com",
		},
		{
			name:   "falls back to JWT when header absent",
			claims: jwt.MapClaims{"email": "jwt@example.com"},
			cfg: CallerIdentityConfig{
				JWTIdentityClaim: "email",
				HeaderName:       "X-HyperFleet-Identity",
			},
			want: "jwt@example.com",
		},
		{
			name:        "rejects invalid header value",
			setHeader:   true,
			headerValue: "bad\x00value",
			cfg: CallerIdentityConfig{
				HeaderName: "X-HyperFleet-Identity",
			},
			wantErr: true,
		},
		{
			name:        "rejects header value exceeding max length",
			setHeader:   true,
			headerValue: strings.Repeat("a", maxCallerIdentityLen+1),
			cfg: CallerIdentityConfig{
				HeaderName: "X-HyperFleet-Identity",
			},
			wantErr: true,
		},
		{
			name:        "header only without JWT claim",
			setHeader:   true,
			headerValue: "dev-user",
			cfg: CallerIdentityConfig{
				HeaderName: "X-HyperFleet-Identity",
			},
			want: "dev-user",
		},
		{
			name:   "no resolution when nothing configured",
			claims: jwt.MapClaims{"email": "jwt@example.com"},
			cfg:    CallerIdentityConfig{},
			want:   "",
		},
		{
			name:        "empty header falls back to JWT",
			claims:      jwt.MapClaims{"email": "jwt@example.com"},
			setHeader:   true,
			headerValue: "",
			cfg: CallerIdentityConfig{
				JWTIdentityClaim: "email",
				HeaderName:       "X-HyperFleet-Identity",
			},
			want: "jwt@example.com",
		},
		{
			name:        "whitespace-only header falls back to JWT",
			claims:      jwt.MapClaims{"email": "jwt@example.com"},
			setHeader:   true,
			headerValue: "   ",
			cfg: CallerIdentityConfig{
				JWTIdentityClaim: "email",
				HeaderName:       "X-HyperFleet-Identity",
			},
			want: "jwt@example.com",
		},
		{
			name:        "header at exact max length accepted",
			setHeader:   true,
			headerValue: strings.Repeat("a", maxCallerIdentityLen),
			cfg: CallerIdentityConfig{
				HeaderName: "X-HyperFleet-Identity",
			},
			want: strings.Repeat("a", maxCallerIdentityLen),
		},
		{
			name:   "rejects oversized JWT claim value",
			claims: jwt.MapClaims{"email": strings.Repeat("x", maxCallerIdentityLen+1)},
			cfg: CallerIdentityConfig{
				JWTIdentityClaim: "email",
			},
			wantErr: true,
		},
		{
			name:   "trims whitespace from JWT claim value",
			claims: jwt.MapClaims{"email": "  user@example.com  "},
			cfg: CallerIdentityConfig{
				JWTIdentityClaim: "email",
			},
			want: "user@example.com",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			RegisterTestingT(t)
			r := httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/clusters", nil)
			if tc.claims != nil {
				r = r.WithContext(contextWithClaims(tc.claims))
			}
			if tc.setHeader {
				headerName := tc.cfg.HeaderName
				if headerName == "" {
					headerName = "X-HyperFleet-Identity"
				}
				r.Header.Set(headerName, tc.headerValue)
			}

			identity, err := CallerIdentityFromRequest(r.Context(), r, tc.cfg)
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

	t.Run("rejects forbidden header name", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := NewCallerIdentityMiddleware(CallerIdentityConfig{
			HeaderName: "Authorization",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not allowed"))
	})

	t.Run("returns middleware when header validation passes", func(t *testing.T) {
		RegisterTestingT(t)
		mw, err := NewCallerIdentityMiddleware(CallerIdentityConfig{
			HeaderName: "X-HyperFleet-Identity",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(mw).NotTo(BeNil())
	})

	t.Run("returns middleware with no config", func(t *testing.T) {
		RegisterTestingT(t)
		mw, err := NewCallerIdentityMiddleware(CallerIdentityConfig{})
		Expect(err).NotTo(HaveOccurred())
		Expect(mw).NotTo(BeNil())
	})
}

func TestResolveCallerIdentityMiddleware(t *testing.T) {
	RegisterTestingT(t)

	t.Run("skips openapi paths", func(t *testing.T) {
		RegisterTestingT(t)
		mw, err := NewCallerIdentityMiddleware(CallerIdentityConfig{JWTIdentityClaim: "email"})
		Expect(err).NotTo(HaveOccurred())

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
		mw, err := NewCallerIdentityMiddleware(CallerIdentityConfig{JWTIdentityClaim: "email"})
		Expect(err).NotTo(HaveOccurred())

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
		mw, err := NewCallerIdentityMiddleware(CallerIdentityConfig{JWTIdentityClaim: "email"})
		Expect(err).NotTo(HaveOccurred())

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
		mw, err := NewCallerIdentityMiddleware(CallerIdentityConfig{HeaderName: "X-HyperFleet-Identity"})
		Expect(err).NotTo(HaveOccurred())

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
		mw, err := NewCallerIdentityMiddleware(CallerIdentityConfig{HeaderName: "X-HyperFleet-Identity"})
		Expect(err).NotTo(HaveOccurred())

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

	t.Run("allows POST with identity from header", func(t *testing.T) {
		RegisterTestingT(t)
		mw, err := NewCallerIdentityMiddleware(CallerIdentityConfig{HeaderName: "X-HyperFleet-Identity"})
		Expect(err).NotTo(HaveOccurred())

		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			Expect(GetUsernameFromContext(r.Context())).To(Equal("user@example.com"))
		})

		r := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", nil)
		r.Header.Set("X-HyperFleet-Identity", "user@example.com")
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(called).To(BeTrue())
		Expect(w.Code).To(Equal(http.StatusOK))
	})

	t.Run("returns 401 on PUT without identity", func(t *testing.T) {
		RegisterTestingT(t)
		mw, err := NewCallerIdentityMiddleware(CallerIdentityConfig{HeaderName: "X-HyperFleet-Identity"})
		Expect(err).NotTo(HaveOccurred())

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

	t.Run("returns 401 on POST with oversized header", func(t *testing.T) {
		RegisterTestingT(t)
		mw, err := NewCallerIdentityMiddleware(CallerIdentityConfig{HeaderName: "X-HyperFleet-Identity"})
		Expect(err).NotTo(HaveOccurred())

		nextCalled := false
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			nextCalled = true
		})

		r := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", nil)
		r.Header.Set("X-HyperFleet-Identity", strings.Repeat("a", maxCallerIdentityLen+1))
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(w.Code).To(Equal(http.StatusUnauthorized))
		Expect(nextCalled).To(BeFalse())
	})

	t.Run("POST with empty header falls back to JWT", func(t *testing.T) {
		RegisterTestingT(t)
		mw, err := NewCallerIdentityMiddleware(CallerIdentityConfig{
			JWTIdentityClaim: "email",
			HeaderName:       "X-HyperFleet-Identity",
		})
		Expect(err).NotTo(HaveOccurred())

		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			Expect(GetUsernameFromContext(r.Context())).To(Equal("jwt@example.com"))
		})

		r := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", nil)
		r = r.WithContext(contextWithClaims(jwt.MapClaims{"email": "jwt@example.com"}))
		r.Header.Set("X-HyperFleet-Identity", "")
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(called).To(BeTrue())
		Expect(w.Code).To(Equal(http.StatusOK))
	})

	t.Run("header identity takes precedence over JWT on POST", func(t *testing.T) {
		RegisterTestingT(t)
		mw, err := NewCallerIdentityMiddleware(CallerIdentityConfig{
			JWTIdentityClaim: "email",
			HeaderName:       "X-HyperFleet-Identity",
		})
		Expect(err).NotTo(HaveOccurred())

		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			Expect(GetUsernameFromContext(r.Context())).To(Equal("header@gateway.com"))
		})

		r := httptest.NewRequest(http.MethodPost, "/api/hyperfleet/v1/clusters", nil)
		r = r.WithContext(contextWithClaims(jwt.MapClaims{"email": "jwt@example.com"}))
		r.Header.Set("X-HyperFleet-Identity", "header@gateway.com")
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(called).To(BeTrue())
		Expect(w.Code).To(Equal(http.StatusOK))
	})
}
