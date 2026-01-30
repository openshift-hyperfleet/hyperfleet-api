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
		logger.With(r.Context(), logger.HTTPMethod(r.Method), logger.HTTPPath(r.URL.Path)).
			Info("Mock authz allows <any>/<any> for method/path")
		next.ServeHTTP(w, r)
	})
}
