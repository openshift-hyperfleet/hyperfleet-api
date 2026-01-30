package auth

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v4"
	"github.com/openshift-online/ocm-sdk-go/authentication"
)

// Context key type defined to avoid collisions in other pkgs using context
// See https://golang.org/pkg/context/#WithValue
type contextKey string

const (
	ContextUsernameKey contextKey = "username"

	// Does not use contextKey type because the jwt middleware improperly updates context with string key type
	// See https://github.com/auth0/go-jwt-middleware/blob/master/jwtmiddleware.go#L232
	ContextAuthKey string = "user"
)

// AuthPayload defines the structure of the JWT payload we expect from
// RHD JWT tokens
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

// GetAuthPayloadFromContext Get authorization payload api object from context
func GetAuthPayloadFromContext(ctx context.Context) (*Payload, error) {
	// Get user token from request context and validate
	userToken, err := authentication.TokenFromContext(ctx)
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve JWT token from request context: %v", err)
	}

	if userToken == nil {
		return nil, fmt.Errorf("JWT token in context is nil, unauthorized")
	}

	// Username is stored in token claim with key 'sub'
	claims, ok := userToken.Claims.(jwt.MapClaims)
	if !ok {
		err := fmt.Errorf("unable to parse JWT token claims: %#v", userToken.Claims)
		return nil, err
	}

	// TODO figure out how to unmarshal jwt.mapclaims into the struct to avoid all the
	// type assertions
	//
	// var accountAuth api.AuthPayload
	// err := json.Unmarshal([]byte(claims), &accountAuth)
	// if err != nil {
	// 	err := fmt.Errorf("Unable to parse JWT token claims")
	// 	return nil, err
	// }

	payload := &Payload{}
	// default to the values we expect from RHSSO
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

	// Check values, if empty, use alternative claims from RHD
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

	// If given and family names are not present, use the name field
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
