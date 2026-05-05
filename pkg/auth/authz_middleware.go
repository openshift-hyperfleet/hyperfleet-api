package auth

import (
	"net/http"
)

type AuthorizationMiddleware interface {
	AuthorizeAPI(next http.Handler) http.Handler
}
