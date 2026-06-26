package auth

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

const maxCallerIdentityLen = 256

// CallerIdentityConfig controls how the caller identity is resolved for audit fields.
// Identity resolution is enabled by setting the relevant fields:
//   - HeaderName: when non-empty, the named HTTP header is checked first
//   - JWTIdentityClaim: when non-empty, the JWT claim is used as fallback (or primary when no header is configured)
//   - IdentityClaimPattern: when non-empty, the resolved identity must match this regex
type CallerIdentityConfig struct {
	JWTIdentityClaim     string
	IdentityClaimPattern string
	HeaderName           string
}

// CallerIdentityFromRequest resolves the caller identity with header-primary precedence.
// When the identity header is configured and present, it overrides the JWT claim.
// Both header and JWT identity values are normalized: trimmed, length-checked, and
// validated for control characters before being accepted.
// compiledPattern is the pre-compiled form of CallerIdentityConfig.IdentityClaimPattern;
// when non-nil the resolved JWT identity must match it.
func CallerIdentityFromRequest(
	ctx context.Context, r *http.Request, cfg CallerIdentityConfig, compiledPattern *regexp.Regexp,
) (string, error) {
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
		identity, err := normalizeIdentity(raw, fmt.Sprintf("JWT claim %q", cfg.JWTIdentityClaim))
		if err != nil {
			return "", err
		}
		if identity != "" && compiledPattern != nil {
			if !compiledPattern.MatchString(identity) {
				return "", fmt.Errorf("identity claim value does not match required pattern %q", cfg.IdentityClaimPattern)
			}
		}
		return identity, nil
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
