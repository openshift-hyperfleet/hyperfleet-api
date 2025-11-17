package adapterStatus

import (
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

// ServiceLocator Service Locator
type ServiceLocator func() services.AdapterStatusService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() services.AdapterStatusService {
		return services.NewAdapterStatusService(
			dao.NewAdapterStatusDao(&env.Database.SessionFactory),
		)
	}
}

// Service helper function to get the adapter status service from the registry
func Service(s *environments.Services) services.AdapterStatusService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("AdapterStatus"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	// Service registration
	registry.RegisterService("AdapterStatus", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})
}
