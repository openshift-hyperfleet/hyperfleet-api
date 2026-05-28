package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

const maxCallerIdentityLen = 256

// CallerIdentityConfig controls how the caller identity is resolved for audit fields.
type CallerIdentityConfig struct {
	JWTIdentityClaim string
	HeaderName       string
	JWTEnabled       bool
	HeaderEnabled    bool
}

// CallerIdentityFromRequest resolves the caller identity with header-primary precedence.
// When the identity header is set and non-empty, it overrides the JWT claim.
func CallerIdentityFromRequest(ctx context.Context, r *http.Request, cfg CallerIdentityConfig) (string, error) {
	if cfg.HeaderEnabled {
		raw := r.Header.Get(cfg.HeaderName)
		if raw != "" {
			identity, err := normalizeIdentityHeaderValue(raw)
			if err != nil {
				return "", err
			}
			if identity != "" {
				return identity, nil
			}
		}
	}

	if cfg.JWTEnabled {
		return GetIdentityFromContext(ctx, cfg.JWTIdentityClaim)
	}

	return "", nil
}

func normalizeIdentityHeaderValue(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if len(value) > maxCallerIdentityLen {
		return "", fmt.Errorf("identity header value exceeds maximum length %d", maxCallerIdentityLen)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("identity header value contains invalid characters")
		}
	}
	return value, nil
}

func shouldSkipCallerIdentity(path string) bool {
	return strings.HasPrefix(path, "/api/hyperfleet/v1/openapi") ||
		strings.HasPrefix(path, "/api/hyperfleet/v1/errors")
}
