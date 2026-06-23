package auth

import (
	"fmt"
	"net/http"
	"regexp"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validation"
)

// CallerIdentityMiddleware resolves and attaches the caller identity used for audit fields.
type CallerIdentityMiddleware interface {
	ResolveCallerIdentity(next http.Handler) http.Handler
}

type callerIdentityMiddleware struct {
	compiledPattern *regexp.Regexp
	cfg             CallerIdentityConfig
}

var _ CallerIdentityMiddleware = &callerIdentityMiddleware{}

func NewCallerIdentityMiddleware(cfg CallerIdentityConfig) (CallerIdentityMiddleware, error) {
	if cfg.HeaderName != "" {
		if validation.IsForbiddenIdentityHeaderName(cfg.HeaderName) {
			return nil, fmt.Errorf("identity header name %q is not allowed", cfg.HeaderName)
		}
	}
	var compiledPattern *regexp.Regexp
	if cfg.IdentityClaimPattern != "" {
		var err error
		compiledPattern, err = regexp.Compile(cfg.IdentityClaimPattern)
		if err != nil {
			return nil, fmt.Errorf("identity_claim_pattern is not a valid regex: %w", err)
		}
	}
	return &callerIdentityMiddleware{cfg: cfg, compiledPattern: compiledPattern}, nil
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
		identity, err := CallerIdentityFromRequest(ctx, r, m.cfg, m.compiledPattern)

		if identity != "" {
			ctx = SetUsernameContext(ctx, identity)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
			return
		}

		if isMutatingMethod(r.Method) {
			msg := "Caller identity is required for mutating requests but could not be resolved"
			if err != nil {
				msg = fmt.Sprintf("Unable to resolve caller identity: %s", err)
			}
			handleError(ctx, w, r, errors.CodeAuthNoCredentials, msg)
			return
		}

		next.ServeHTTP(w, r)
	})
}
