package auth

import (
	"fmt"
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
)

// CallerIdentityMiddleware resolves and attaches the caller identity used for audit fields.
type CallerIdentityMiddleware interface {
	ResolveCallerIdentity(next http.Handler) http.Handler
}

type callerIdentityMiddleware struct{}

var _ CallerIdentityMiddleware = &callerIdentityMiddleware{}

func NewCallerIdentityMiddleware() CallerIdentityMiddleware {
	return &callerIdentityMiddleware{}
}

// ResolveCallerIdentity attaches the resolved caller identity to the request context.
// JWT validation is performed by JWTHandler; this middleware only resolves attribution.
// The matched issuer config (identity_claim, identity_claim_pattern) is read from context.
// If an identity header is configured, it takes precedence over JWT claims.
func (m *callerIdentityMiddleware) ResolveCallerIdentity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shouldSkipCallerIdentity(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		ctx := r.Context()
		identity, err := CallerIdentityFromRequest(ctx, r)

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
