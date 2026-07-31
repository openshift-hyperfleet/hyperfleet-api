package tenant

import (
	"net/http"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api/response"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

// Middleware resolves tenant identity from gateway-injected request headers
// and stores it in the request context for the DAO layer to scope queries.
// It trusts its headers: the deployment must guarantee they can only be set
// by the authorization gateway (the gateway strips client-supplied copies
// before authorization runs).
type Middleware struct {
	config TenantConfig
}

// NewMiddleware creates a tenant enforcement middleware from validated config.
func NewMiddleware(cfg TenantConfig) *Middleware {
	return &Middleware{config: cfg}
}

// EnforceTenant resolves the caller's tenant identity. System identities pass
// through unscoped. Non-system callers must resolve every required dimension,
// and at least one dimension overall; otherwise the request is rejected with
// 403. An empty tenancy map must never reach the query layer because empty
// JSONB containment matches every row.
func (m *Middleware) EnforceTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if shouldSkipTenant(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		ctx := r.Context()

		if strings.EqualFold(r.Header.Get(m.config.SystemHeader), "true") {
			ctx = WithTenant(ctx, &ResolvedTenant{System: true})
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		resolved := &ResolvedTenant{
			Dimensions: make(map[string]string, len(m.config.Dimensions)),
		}
		for _, dim := range m.config.Dimensions {
			value := strings.TrimSpace(r.Header.Get(dim.Header))
			if value == "" {
				if dim.Required {
					logger.With(ctx,
						"header", dim.Header,
						"tenancy_key", dim.Key,
					).Warn("Missing required tenant header")
					writeTenantForbidden(w, r, "missing required tenant identity: header "+dim.Header)
					return
				}
				continue
			}
			resolved.Dimensions[dim.Key] = value
		}

		if len(resolved.Dimensions) == 0 {
			logger.With(ctx).Warn("No tenant dimensions resolved for non-system caller")
			writeTenantForbidden(w, r, "no tenant identity resolved from request")
			return
		}

		logger.With(ctx, "tenancy", resolved.Dimensions).Debug("Tenant identity resolved")
		ctx = WithTenant(ctx, resolved)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeTenantForbidden(w http.ResponseWriter, r *http.Request, reason string) {
	traceID, ok := logger.GetRequestID(r.Context())
	if !ok {
		traceID = "unknown"
	}
	err := errors.New(errors.CodeAuthTenantRequired, "%s", reason)
	response.WriteProblemDetailsResponse(w, r, err.HTTPCode, err.AsProblemDetails(r.URL.Path, traceID))
}

func shouldSkipTenant(path string) bool {
	return strings.HasPrefix(path, "/api/hyperfleet/v1/openapi") ||
		strings.HasPrefix(path, "/api/hyperfleet/v1/errors")
}
