package tenant

import (
	"context"
	"encoding/json"

	"gorm.io/datatypes"
)

type contextKey struct{}

// ResolvedTenant holds the tenant identity resolved from gateway-injected request headers.
type ResolvedTenant struct {
	Dimensions map[string]string
	System     bool
}

// WithTenant attaches a resolved tenant identity to the context.
func WithTenant(ctx context.Context, t *ResolvedTenant) context.Context {
	return context.WithValue(ctx, contextKey{}, t)
}

// FromContext returns the tenant identity attached to ctx, or nil if none was resolved.
func FromContext(ctx context.Context) *ResolvedTenant {
	if t, ok := ctx.Value(contextKey{}).(*ResolvedTenant); ok {
		return t
	}
	return nil
}

// TenancyJSON returns the caller's tenancy map as JSONB for storage on created resources.
// System and unscoped/absent callers get an empty map, which no tenant-scoped query can ever match.
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
