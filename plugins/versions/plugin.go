package versions

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/resources"
)

const (
	kindVersion       = "Version"
	pluralVersions    = "versions"
	specSchemaVersion = "VersionSpec"
)

func init() {
	registry.Register(registry.EntityDescriptor{
		Kind:                   kindVersion,
		Plural:                 pluralVersions,
		ParentKind:             "Channel",
		SpecSchemaName:         specSchemaVersion,
		OnParentDelete:         registry.OnParentDeleteRestrict,
		SearchDisallowedFields: []string{"spec"},
	})

	server.RegisterRoutes(pluralVersions, func(
		apiV1Router *mux.Router,
		svc server.ServicesInterface,
		authMiddleware auth.JWTMiddleware,
	) {
		envServices := svc.(*environments.Services)
		channelDescriptor := registry.MustGet("Channel")
		descriptor := registry.MustGet(kindVersion)
		h := handlers.NewResourceHandler(
			descriptor,
			resources.Service(envServices),
		)

		r := apiV1Router.PathPrefix("/" + channelDescriptor.Plural + "/{parent_id}/" + descriptor.Plural).Subrouter()
		r.HandleFunc("", h.ListByOwner).Methods(http.MethodGet)
		r.HandleFunc("", h.CreateWithOwner).Methods(http.MethodPost)
		r.HandleFunc("/{id}", h.GetByOwner).Methods(http.MethodGet)
		r.HandleFunc("/{id}", h.PatchByOwner).Methods(http.MethodPatch)
		r.HandleFunc("/{id}", h.DeleteByOwner).Methods(http.MethodDelete)

		r.Use(authMiddleware.AuthenticateAccountJWT)
	})
}
