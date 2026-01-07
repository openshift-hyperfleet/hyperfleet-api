package auth

import (
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

type authzMiddlewareMock struct{}

var _ AuthorizationMiddleware = &authzMiddlewareMock{}

func NewAuthzMiddlewareMock() AuthorizationMiddleware {
	return &authzMiddlewareMock{}
}

func (a authzMiddlewareMock) AuthorizeApi(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		logger.Info(r.Context(), "Mock authz allows <any>/<any> for method/url", "method", r.Method, "url", r.URL.String())
		next.ServeHTTP(w, r)
	})
}
