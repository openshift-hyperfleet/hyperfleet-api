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
				JWTEnabled:       true,
				JWTIdentityClaim: "email",
				HeaderEnabled:    true,
				HeaderName:       "X-HyperFleet-Identity",
			},
			want: "gateway-user@example.com",
		},
		{
			name:   "falls back to JWT when header absent",
			claims: jwt.MapClaims{"email": "jwt@example.com"},
			cfg: CallerIdentityConfig{
				JWTEnabled:       true,
				JWTIdentityClaim: "email",
				HeaderEnabled:    true,
				HeaderName:       "X-HyperFleet-Identity",
			},
			want: "jwt@example.com",
		},
		{
			name:        "rejects invalid header value",
			setHeader:   true,
			headerValue: "bad\x00value",
			cfg: CallerIdentityConfig{
				HeaderEnabled: true,
				HeaderName:    "X-HyperFleet-Identity",
			},
			wantErr: true,
		},
		{
			name:        "rejects header value exceeding max length",
			setHeader:   true,
			headerValue: strings.Repeat("a", maxCallerIdentityLen+1),
			cfg: CallerIdentityConfig{
				HeaderEnabled: true,
				HeaderName:    "X-HyperFleet-Identity",
			},
			wantErr: true,
		},
		{
			name:        "header only when JWT disabled",
			setHeader:   true,
			headerValue: "dev-user",
			cfg: CallerIdentityConfig{
				JWTEnabled:    false,
				HeaderEnabled: true,
				HeaderName:    "X-HyperFleet-Identity",
			},
			want: "dev-user",
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

	t.Run("rejects forbidden header name when enabled", func(t *testing.T) {
		RegisterTestingT(t)
		_, err := NewCallerIdentityMiddleware(CallerIdentityConfig{
			HeaderEnabled: true,
			HeaderName:    "Authorization",
		})
		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("not allowed"))
	})

	t.Run("returns middleware when header validation passes", func(t *testing.T) {
		RegisterTestingT(t)
		mw, err := NewCallerIdentityMiddleware(CallerIdentityConfig{
			HeaderEnabled: true,
			HeaderName:    "X-HyperFleet-Identity",
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(mw).NotTo(BeNil())
	})
}

func TestResolveCallerIdentityMiddleware(t *testing.T) {
	RegisterTestingT(t)

	t.Run("skips openapi paths", func(t *testing.T) {
		RegisterTestingT(t)
		mw, err := NewCallerIdentityMiddleware(CallerIdentityConfig{JWTEnabled: true, JWTIdentityClaim: "email"})
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

	t.Run("returns 401 when JWT enabled and identity missing", func(t *testing.T) {
		RegisterTestingT(t)
		mw, err := NewCallerIdentityMiddleware(CallerIdentityConfig{JWTEnabled: true, JWTIdentityClaim: "email"})
		Expect(err).NotTo(HaveOccurred())

		nextCalled := false
		next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			nextCalled = true
		})

		r := httptest.NewRequest(http.MethodGet, "/api/hyperfleet/v1/clusters", nil)
		w := httptest.NewRecorder()
		mw.ResolveCallerIdentity(next).ServeHTTP(w, r)

		Expect(w.Code).To(Equal(http.StatusUnauthorized))
		Expect(nextCalled).To(BeFalse())
	})
}
