package channels

import (
	"net/http"

	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/resources"
)

const (
	kindChannel       = "Channel"
	pluralChannels    = "channels"
	specSchemaChannel = "ChannelSpec"
)

func init() {
	registry.Register(registry.EntityDescriptor{
		Kind:                   kindChannel,
		Plural:                 pluralChannels,
		SpecSchemaName:         specSchemaChannel,
		SearchDisallowedFields: []string{"spec"},
	})

	server.RegisterRoutes(pluralChannels, func(apiV1Router *mux.Router, svc server.ServicesInterface) {
		envServices := svc.(*environments.Services)
		descriptor := registry.MustGet(kindChannel)
		h := handlers.NewResourceHandler(
			descriptor,
			resources.Service(envServices),
		)

		r := apiV1Router.PathPrefix("/" + descriptor.Plural).Subrouter()
		r.HandleFunc("", h.List).Methods(http.MethodGet)
		r.HandleFunc("", h.Create).Methods(http.MethodPost)
		r.HandleFunc("/{id}", h.Get).Methods(http.MethodGet)
		r.HandleFunc("/{id}", h.Patch).Methods(http.MethodPatch)
		r.HandleFunc("/{id}", h.Delete).Methods(http.MethodDelete)
	})
}
