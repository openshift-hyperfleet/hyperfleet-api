package container

import (
	"fmt"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/db_session"
)

func (c *Container) SessionFactory() db.SessionFactory {
	if c.sessionFactory == nil {
		c.sessionFactory = db_session.NewProdFactory(c.cfg.Database)
		sf := c.sessionFactory
		c.closer.Add(func() error {
			const dbCloseTimeout = 5 * time.Second
			done := make(chan error, 1)
			go func() { done <- sf.Close() }()
			select {
			case err := <-done:
				return err
			case <-time.After(dbCloseTimeout):
				return fmt.Errorf("session factory close timed out after %s", dbCloseTimeout)
			}
		})
	}
	return c.sessionFactory
}
