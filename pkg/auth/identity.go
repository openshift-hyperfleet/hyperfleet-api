package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

const maxCallerIdentityLen = 256

// CallerIdentityConfig controls how the caller identity is resolved for audit fields.
// Identity resolution is enabled by setting the relevant fields:
//   - HeaderName: when non-empty, the named HTTP header is checked first
//   - JWTIdentityClaim: when non-empty, the JWT claim is used as fallback (or primary when no header is configured)
type CallerIdentityConfig struct {
	JWTIdentityClaim string
	HeaderName       string
}

// CallerIdentityFromRequest resolves the caller identity with header-primary precedence.
// When the identity header is configured and present, it overrides the JWT claim.
// Both header and JWT identity values are normalized: trimmed, length-checked, and
// validated for control characters before being accepted.
func CallerIdentityFromRequest(ctx context.Context, r *http.Request, cfg CallerIdentityConfig) (string, error) {
	if cfg.HeaderName != "" {
		raw := r.Header.Get(cfg.HeaderName)
		if raw != "" {
			identity, err := normalizeIdentity(raw, "identity header")
			if err != nil {
				return "", err
			}
			if identity != "" {
				return identity, nil
			}
		}
	}

	if cfg.JWTIdentityClaim != "" {
		raw, err := GetIdentityFromContext(ctx, cfg.JWTIdentityClaim)
		if err != nil {
			return "", err
		}
		return normalizeIdentity(raw, fmt.Sprintf("JWT claim %q", cfg.JWTIdentityClaim))
	}

	return "", nil
}

// normalizeIdentity trims, length-checks, and validates a caller identity value
// regardless of source (HTTP header or JWT claim). The source parameter is used
// in error messages to distinguish the origin.
func normalizeIdentity(raw string, source string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	if len(value) > maxCallerIdentityLen {
		return "", fmt.Errorf("%s value exceeds maximum length %d", source, maxCallerIdentityLen)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return "", fmt.Errorf("%s value contains invalid characters", source)
		}
	}
	return value, nil
}

func shouldSkipCallerIdentity(path string) bool {
	return strings.HasPrefix(path, "/api/hyperfleet/v1/openapi") ||
		strings.HasPrefix(path, "/api/hyperfleet/v1/errors")
}

func isMutatingMethod(method string) bool {
	return method == http.MethodPost || method == http.MethodPatch || method == http.MethodDelete ||
		method == http.MethodPut
}
