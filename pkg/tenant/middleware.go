package tenant

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/response"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// maxDimensionValueLen bounds tenant dimension header values to the RFC 1123
// DNS label length limit, matching Kubernetes label-value and namespace-name
// conventions. dimensionValuePattern restricts values to a safe charset
// before they propagate into tenancy JSON and scoped queries.
const maxDimensionValueLen = 63

var dimensionValuePattern = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)

// Resolver resolves the caller's tenant context from trusted gateway-injected
// request headers and fails closed when required identity is missing.
type Resolver interface {
	ResolveTenant(next http.Handler) http.Handler
}

type resolver struct {
	cfg config.TenantConfig
}

var _ Resolver = &resolver{}

// NewResolver returns a tenant enforcement Resolver configured from cfg.
// Callers decide whether to mount it based on cfg.Enabled; the resolver
// itself does not check it.
func NewResolver(cfg config.TenantConfig) Resolver {
	return &resolver{cfg: cfg}
}

// ResolveTenant reads the system header and configured dimension headers off
// the request. A system header value of exactly "true" (case-insensitive)
// grants an unscoped ResolvedTenant. Otherwise, every configured dimension
// header present on the request is collected into a tenancy map; a missing
// required dimension, or resolving zero dimensions, rejects the request with
// 403 before it reaches downstream handlers.
func (m *resolver) ResolveTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shouldSkipTenant(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		ctx := r.Context()

		if strings.EqualFold(strings.TrimSpace(r.Header.Get(m.cfg.SystemHeader)), "true") {
			next.ServeHTTP(w, r.WithContext(WithTenant(ctx, &ResolvedTenant{System: true})))
			return
		}

		dims, err := m.resolveDimensions(r)
		if err != nil {
			handleForbidden(ctx, w, r, "%s", err)
			return
		}

		next.ServeHTTP(w, r.WithContext(WithTenant(ctx, &ResolvedTenant{Dimensions: dims})))
	})
}

// resolveDimensions collects every configured dimension header present on the
// request into a tenancy map. It returns an error describing why resolution
// failed if a required dimension is missing, a value fails validation, or
// zero dimensions are resolved overall.
func (m *resolver) resolveDimensions(r *http.Request) (map[string]string, error) {
	dims := make(map[string]string, len(m.cfg.Dimensions))
	for _, d := range m.cfg.Dimensions {
		val := strings.TrimSpace(r.Header.Get(d.Header))
		if val == "" {
			if d.Required {
				return nil, fmt.Errorf("required tenant header %q is missing", d.Header)
			}
			continue
		}
		if len(val) > maxDimensionValueLen || !dimensionValuePattern.MatchString(val) {
			return nil, fmt.Errorf("tenant header %q has an invalid value", d.Header)
		}
		dims[d.Key] = val
	}

	if len(dims) == 0 {
		return nil, fmt.Errorf("caller resolved no tenant dimensions")
	}

	return dims, nil
}

// handleForbidden writes a 403 problem+json response for a tenant resolution failure.
// reason and values are forwarded to errors.Forbidden, which formats them.
func handleForbidden(
	ctx context.Context, w http.ResponseWriter, r *http.Request, reason string, values ...interface{},
) {
	err := errors.Forbidden(reason, values...)
	logger.WithError(ctx, err).Info("Tenant identity rejected")
	response.WriteServiceErrorResponse(ctx, w, r, err)
}

// shouldSkipTenant delegates to auth.ShouldSkipCallerIdentity so openapi/errors
// paths bypass tenant enforcement the same way they bypass caller identity
// resolution, without maintaining a second copy of the path list.
func shouldSkipTenant(path string) bool {
	return auth.ShouldSkipCallerIdentity(path)
}
