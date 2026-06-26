package auth

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const (
	ContextUsernameKey              contextKey = "username"
	ContextJWTTokenKey              contextKey = "jwt_token"
	ContextMatchedIdentityConfigKey contextKey = "matched_identity_cfg"

	// DefaultJWTIdentityClaim is used when server.jwt.identity_claim is unset.
	DefaultJWTIdentityClaim = "email"
)

type matchedIdentityContext struct {
	cfg     *CallerIdentityConfig
	pattern *regexp.Regexp
}

// SetMatchedIdentityConfig stores the per-issuer identity config resolved by JWTHandler
// in the request context so CallerIdentityMiddleware can read it.
func SetMatchedIdentityConfig(ctx context.Context, cfg CallerIdentityConfig, pattern *regexp.Regexp) context.Context {
	return context.WithValue(ctx, ContextMatchedIdentityConfigKey, matchedIdentityContext{cfg: &cfg, pattern: pattern})
}

// GetMatchedIdentityConfig retrieves the per-issuer identity config stored by JWTHandler.
// Returns false if no config was set (JWT disabled or public path).
func GetMatchedIdentityConfig(ctx context.Context) (CallerIdentityConfig, *regexp.Regexp, bool) {
	v, ok := ctx.Value(ContextMatchedIdentityConfigKey).(matchedIdentityContext)
	if !ok {
		return CallerIdentityConfig{}, nil, false
	}
	return *v.cfg, v.pattern, true
}

// Payload defines the structure of the JWT payload we expect
type Payload struct {
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Issuer    string `json:"iss"`
	ClientID  string `json:"clientId"`
}

func SetUsernameContext(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, ContextUsernameKey, username)
}

func GetUsernameFromContext(ctx context.Context) string {
	username := ctx.Value(ContextUsernameKey)
	if username == nil {
		return ""
	}
	if str, ok := username.(string); ok {
		return str
	}
	return ""
}

func SetJWTTokenContext(ctx context.Context, token *jwt.Token) context.Context {
	return context.WithValue(ctx, ContextJWTTokenKey, token)
}

func GetJWTTokenFromContext(ctx context.Context) *jwt.Token {
	token, ok := ctx.Value(ContextJWTTokenKey).(*jwt.Token)
	if !ok {
		return nil
	}
	return token
}

func GetAuthPayloadFromContext(ctx context.Context) (*Payload, error) {
	userToken := GetJWTTokenFromContext(ctx)
	if userToken == nil {
		return nil, fmt.Errorf("JWT token in context is nil, unauthorized")
	}

	claims, ok := userToken.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("unable to parse JWT token claims: unexpected type %T", userToken.Claims)
	}

	payload := &Payload{}
	if username, ok := claims["username"].(string); ok {
		payload.Username = username
	}
	if firstName, ok := claims["first_name"].(string); ok {
		payload.FirstName = firstName
	}
	if lastName, ok := claims["last_name"].(string); ok {
		payload.LastName = lastName
	}
	if email, ok := claims["email"].(string); ok {
		payload.Email = email
	}
	if issuer, ok := claims["iss"].(string); ok {
		payload.Issuer = issuer
	}
	if clientID, ok := claims["clientId"].(string); ok {
		payload.ClientID = clientID
	}

	if payload.Username == "" {
		if username, ok := claims["preferred_username"].(string); ok {
			payload.Username = username
		}
	}

	if payload.FirstName == "" {
		if firstName, ok := claims["given_name"].(string); ok {
			payload.FirstName = firstName
		}
	}

	if payload.LastName == "" {
		if lastName, ok := claims["family_name"].(string); ok {
			payload.LastName = lastName
		}
	}

	if payload.FirstName == "" || payload.LastName == "" {
		if name, ok := claims["name"].(string); ok {
			names := strings.Split(name, " ")
			if len(names) > 1 {
				payload.FirstName = names[0]
				payload.LastName = names[1]
			} else {
				payload.FirstName = names[0]
			}
		}
	}

	return payload, nil
}

// GetIdentityFromContext returns the configured JWT claim value used as the request identity.
func GetIdentityFromContext(ctx context.Context, identityClaim string) (string, error) {
	if identityClaim == "" {
		identityClaim = DefaultJWTIdentityClaim
	}

	userToken := GetJWTTokenFromContext(ctx)
	if userToken == nil {
		return "", fmt.Errorf("JWT token in context is nil, unauthorized")
	}

	claims, ok := userToken.Claims.(jwt.MapClaims)
	if !ok {
		return "", fmt.Errorf("unable to parse JWT token claims: unexpected type %T", userToken.Claims)
	}

	if identity, ok := stringClaim(claims, identityClaim); ok {
		return identity, nil
	}

	payload, err := GetAuthPayloadFromContext(ctx)
	if err != nil {
		return "", fmt.Errorf("identity claim %q not found: %w", identityClaim, err)
	}

	switch identityClaim {
	case "email":
		if payload.Email != "" {
			return payload.Email, nil
		}
	case "username", "preferred_username":
		if payload.Username != "" {
			return payload.Username, nil
		}
	case "first_name", "given_name":
		if payload.FirstName != "" {
			return payload.FirstName, nil
		}
	case "last_name", "family_name":
		if payload.LastName != "" {
			return payload.LastName, nil
		}
	case "clientId":
		if payload.ClientID != "" {
			return payload.ClientID, nil
		}
	case "iss":
		if payload.Issuer != "" {
			return payload.Issuer, nil
		}
	}

	return "", fmt.Errorf("identity claim %q not found or empty", identityClaim)
}

func stringClaim(claims jwt.MapClaims, key string) (string, bool) {
	val, ok := claims[key]
	if !ok {
		return "", false
	}
	s, ok := val.(string)
	return s, ok && s != ""
}
