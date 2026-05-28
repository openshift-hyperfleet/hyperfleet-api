package auth

import (
	"fmt"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validation"
)

// CallerIdentityMiddleware resolves and attaches the caller identity used for audit fields.
type CallerIdentityMiddleware interface {
	ResolveCallerIdentity(next http.Handler) http.Handler
}

type callerIdentityMiddleware struct {
	cfg CallerIdentityConfig
}

var _ CallerIdentityMiddleware = &callerIdentityMiddleware{}

func NewCallerIdentityMiddleware(cfg CallerIdentityConfig) (CallerIdentityMiddleware, error) {
	if cfg.JWTIdentityClaim == "" {
		cfg.JWTIdentityClaim = DefaultJWTIdentityClaim
	}
	if cfg.HeaderEnabled {
		if cfg.HeaderName == "" {
			return nil, fmt.Errorf("identity header name is required when identity header is enabled")
		}
		if validation.IsForbiddenIdentityHeaderName(cfg.HeaderName) {
			return nil, fmt.Errorf("identity header name %q is not allowed", cfg.HeaderName)
		}
	}
	return &callerIdentityMiddleware{cfg: cfg}, nil
}

// ResolveCallerIdentity attaches the resolved caller identity to the request context.
// JWT validation is performed by JWTHandler; this middleware only resolves attribution.
func (m *callerIdentityMiddleware) ResolveCallerIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shouldSkipCallerIdentity(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		ctx := r.Context()
		identity, err := CallerIdentityFromRequest(ctx, r, m.cfg)
		if err != nil {
			handleError(
				ctx, w, r, errors.CodeAuthNoCredentials,
				fmt.Sprintf("Unable to resolve caller identity: %s", err),
			)
			return
		}

		if identity != "" {
			ctx = SetUsernameContext(ctx, identity)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
			return
		}

		if m.cfg.JWTEnabled {
			handleError(
				ctx, w, r, errors.CodeAuthNoCredentials,
				"Unable to resolve caller identity from JWT token or identity header",
			)
			return
		}

		next.ServeHTTP(w, r)
	})
}
