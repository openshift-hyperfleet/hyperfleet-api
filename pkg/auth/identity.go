package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
)

const maxCallerIdentityLen = 256

// CallerIdentityFromRequest resolves the caller identity.
// Priority: identity header (per-issuer, if configured and present) > JWT claim from matched issuer config.
// The issuer config is set in context by JWTHandler when a token is validated.
func CallerIdentityFromRequest(ctx context.Context, r *http.Request) (string, error) {
	issuerCfg, ok := GetJWTIssuerConfigFromContext(ctx)
	if !ok {
		return "", nil
	}

	if issuerCfg.IdentityHeader != "" {
		headerVal := r.Header.Get(issuerCfg.IdentityHeader)
		identity, err := normalizeIdentity(headerVal, fmt.Sprintf("header %q", issuerCfg.IdentityHeader))
		if err != nil {
			return "", err
		}
		if identity != "" {
			if err := matchIdentityPattern(identity, "identity header value", issuerCfg); err != nil {
				return "", err
			}
			return identity, nil
		}
	}

	identityClaim := issuerCfg.IdentityClaim
	if identityClaim == "" {
		return "", nil
	}

	raw, err := GetIdentityFromContext(ctx, identityClaim)
	if err != nil {
		return "", err
	}

	identity, err := normalizeIdentity(raw, fmt.Sprintf("JWT claim %q", identityClaim))
	if err != nil {
		return "", err
	}

	if identity != "" {
		if err := matchIdentityPattern(identity, "identity claim value", issuerCfg); err != nil {
			return "", err
		}
	}

	return identity, nil
}

func matchIdentityPattern(identity, source string, cfg config.JWTIssuerConfig) error {
	if cfg.CompiledPattern != nil && !cfg.CompiledPattern.MatchString(identity) {
		return fmt.Errorf("%s does not match required pattern", source)
	}
	return nil
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
