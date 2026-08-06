package container

import (
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_session"
)

func (c *Container) SessionFactory() db.SessionFactory {
	if c.sessionFactory == nil {
		c.sessionFactory = db_session.NewProdFactory(c.cfg.Database)
		c.closer.Add(c.sessionFactory.Close)
	}
	return c.sessionFactory
}
