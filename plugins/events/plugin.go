package events

import (
	"github.com/openshift-hyperfleet/hyperfleet/cmd/hyperfleet/environments"
	"github.com/openshift-hyperfleet/hyperfleet/cmd/hyperfleet/environments/registry"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet/pkg/services"
)

// ServiceLocator Service Locator
type ServiceLocator func() services.EventService

func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() services.EventService {
		return services.NewEventService(dao.NewEventDao(&env.Database.SessionFactory))
	}
}

// Service helper function to get the event service from the registry
func Service(s *environments.Services) services.EventService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("Events"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	// Service registration
	registry.RegisterService("Events", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})
}
