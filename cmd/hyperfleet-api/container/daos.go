package container

import (
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
)

func (c *Container) ResourceDao() dao.ResourceDao {
	if c.resourceDao == nil {
		c.resourceDao = dao.NewResourceDao(c.sessionFactory)
	}
	return c.resourceDao
}

func (c *Container) ResourceLabelDao() dao.ResourceLabelDao {
	if c.resourceLabelDao == nil {
		c.resourceLabelDao = dao.NewResourceLabelDao(c.sessionFactory)
	}
	return c.resourceLabelDao
}

func (c *Container) AdapterStatusDao() dao.AdapterStatusDao {
	if c.adapterStatusDao == nil {
		c.adapterStatusDao = dao.NewAdapterStatusDao(c.sessionFactory)
	}
	return c.adapterStatusDao
}

func (c *Container) ResourceConditionDao() dao.ResourceConditionDao {
	if c.resourceConditionDao == nil {
		c.resourceConditionDao = dao.NewResourceConditionDao(c.sessionFactory)
	}
	return c.resourceConditionDao
}

func (c *Container) GenericDao() dao.GenericDao {
	if c.genericDao == nil {
		c.genericDao = dao.NewGenericDao(c.sessionFactory)
	}
	return c.genericDao
}
