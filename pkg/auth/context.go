package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const (
	ContextUsernameKey contextKey = "username"
	ContextJWTTokenKey contextKey = "jwt_token"
)

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

func GetAuthPayload(r *http.Request) (*Payload, error) {
	return GetAuthPayloadFromContext(r.Context())
}
