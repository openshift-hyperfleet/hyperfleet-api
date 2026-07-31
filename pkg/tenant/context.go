package tenant

import (
	"context"
	"encoding/json"

	"gorm.io/datatypes"
)

type contextKey struct{}

// ResolvedTenant holds the tenant identity resolved from gateway-injected
// request headers.
type ResolvedTenant struct {
	// Dimensions maps tenancy keys to their resolved values
	// (e.g. "org" -> "acme").
	Dimensions map[string]string
	// System marks a trusted internal identity (Sentinel, adapters). System
	// callers bypass tenant scoping.
	System bool
}

// WithTenant stores resolved tenant information in the request context.
func WithTenant(ctx context.Context, t *ResolvedTenant) context.Context {
	return context.WithValue(ctx, contextKey{}, t)
}

// FromContext retrieves the resolved tenant from the context. Returns nil if
// tenant enforcement is not active for this request.
func FromContext(ctx context.Context) *ResolvedTenant {
	if t, ok := ctx.Value(contextKey{}).(*ResolvedTenant); ok {
		return t
	}
	return nil
}

// Scoped reports whether the context carries a tenant that queries must be
// scoped to. System identities and absent tenants are unscoped.
func Scoped(ctx context.Context) bool {
	t := FromContext(ctx)
	return t != nil && !t.System && len(t.Dimensions) > 0
}

// TenancyJSON returns the caller's tenancy map as a JSONB value for storage
// on created resources. System and unscoped callers get an empty map, which
// no tenant-scoped query can ever match.
func TenancyJSON(ctx context.Context) datatypes.JSON {
	t := FromContext(ctx)
	if t == nil || t.System || len(t.Dimensions) == 0 {
		return datatypes.JSON([]byte("{}"))
	}
	b, err := json.Marshal(t.Dimensions)
	if err != nil {
		return datatypes.JSON([]byte("{}"))
	}
	return datatypes.JSON(b)
}

// ContainmentJSON returns the caller's tenancy map serialized for use as a
// JSONB containment (@>) query argument, and whether scoping applies at all.
func ContainmentJSON(ctx context.Context) (string, bool) {
	t := FromContext(ctx)
	if t == nil || t.System || len(t.Dimensions) == 0 {
		return "", false
	}
	b, err := json.Marshal(t.Dimensions)
	if err != nil {
		return "", false
	}
	return string(b), true
}
