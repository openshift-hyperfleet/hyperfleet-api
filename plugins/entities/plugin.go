package entities

import (
	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/handlers"
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/resources"
)

func init() {
	server.RegisterRoutes("entities", func(apiV1Router *mux.Router, svc server.ServicesInterface) {
		envServices := svc.(*environments.Services)
		handlers.RegisterEntityRoutes(apiV1Router, resources.Service(envServices))
	})
}
