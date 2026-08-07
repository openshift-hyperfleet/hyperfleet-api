package container

import (
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

func (c *Container) ResourceService() services.ResourceService {
	if c.resourceService == nil {
		svc, err := services.NewResourceService(
			c.ResourceDao(),
			c.ResourceLabelDao(),
			c.AdapterStatusDao(),
			c.ResourceConditionDao(),
			c.GenericService(),
		)
		if err != nil {
			panic("failed to create resource service: " + err.Error())
		}
		c.resourceService = svc
	}
	return c.resourceService
}

func (c *Container) AdapterStatusService() services.AdapterStatusService {
	if c.adapterStatusService == nil {
		c.adapterStatusService = services.NewAdapterStatusService(c.AdapterStatusDao())
	}
	return c.adapterStatusService
}

func (c *Container) GenericService() services.GenericService {
	if c.genericService == nil {
		c.genericService = services.NewGenericService(c.GenericDao())
	}
	return c.genericService
}
