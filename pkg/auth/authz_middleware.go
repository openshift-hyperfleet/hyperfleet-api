package auth

/*
   The goal of this simple authz middlewre is to provide a way for access review
   parameters to be declared for each route in a microservice. This is not meant
   to handle more complex access review calls in particular scopes, but rather
   just authz calls at the application scope

  This is a big TODO, not ready for consumption
*/

import (
	"net/http"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/client/ocm"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
)

type AuthorizationMiddleware interface {
	AuthorizeAPI(next http.Handler) http.Handler
}

type authzMiddleware struct {
	ocmClient    *ocm.Client
	action       string
	resourceType string
}

var _ AuthorizationMiddleware = &authzMiddleware{}

func NewAuthzMiddleware(ocmClient *ocm.Client, action, resourceType string) AuthorizationMiddleware {
	return &authzMiddleware{
		ocmClient:    ocmClient,
		action:       action,
		resourceType: resourceType,
	}
}

func (a authzMiddleware) AuthorizeAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Get username from context
		username := GetUsernameFromContext(ctx)
		if username == "" {
			logger.Error(ctx, "authenticated username not present in request context")
			// TODO
			// body := api.E500.Format(r, "Authentication details not present in context")
			// api.SendError(w, r, &body)
			return
		}

		allowed, err := a.ocmClient.Authorization.AccessReview(
			ctx, username, a.action, a.resourceType, "", "", "")
		if err != nil {
			logger.WithError(ctx, err).Error("unable to make authorization request")
			// TODO
			// body := api.E500.Format(r, "Unable to make authorization request")
			// api.SendError(w, r, &body)
			return
		}

		if allowed {
			next.ServeHTTP(w, r)
		}

		// TODO
		// body := api.E403.Format(r, "")
		// api.SendError(w, r, &body)
	})
}
