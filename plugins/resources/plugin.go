package resources

import (
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/plugins/generic"
)

type ServiceLocator func() services.ResourceService

// NewServiceLocator creates a ServiceLocator that builds a ResourceService from the environment.
func NewServiceLocator(env *environments.Env) ServiceLocator {
	return func() services.ResourceService {
		return services.NewResourceService(
			dao.NewResourceDao(env.Database.SessionFactory),
			dao.NewAdapterStatusDao(env.Database.SessionFactory),
			dao.NewResourceConditionDao(env.Database.SessionFactory),
			dao.NewResourceLabelDao(env.Database.SessionFactory),
			generic.Service(&env.Services),
		)
	}
}

// Service retrieves the ResourceService from the service registry.
func Service(s *environments.Services) services.ResourceService {
	if s == nil {
		return nil
	}
	if obj := s.GetService("Resources"); obj != nil {
		locator := obj.(ServiceLocator)
		return locator()
	}
	return nil
}

func init() {
	registry.RegisterService("Resources", func(env interface{}) interface{} {
		return NewServiceLocator(env.(*environments.Env))
	})
}
