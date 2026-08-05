package container

import (
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validators"
)

// Container lazily constructs and caches application dependencies during
// sequential startup. It is not safe for concurrent initialization.
//
// Container owns dependencies only. Assembling them into a running API server
// is the composition root's job - see BuildAPIServer in the servecmd package.
//
// TODO(HYPERFLEET-1371): Once the environments/ package is removed,
// Container should source SessionFactory directly (e.g. from config/Viper)
// rather than accepting it as a constructor parameter. Close() should also
// close the SessionFactory at that point.
type Container struct {
	cfg            *config.ApplicationConfig
	sessionFactory db.SessionFactory

	resourceDao          dao.ResourceDao
	resourceLabelDao     dao.ResourceLabelDao
	adapterStatusDao     dao.AdapterStatusDao
	resourceConditionDao dao.ResourceConditionDao
	genericDao           dao.GenericDao

	resourceService      services.ResourceService
	adapterStatusService services.AdapterStatusService
	genericService       services.GenericService

	schemaValidator *validators.SchemaValidator
	jwtHandler      *auth.JWTHandler
}

func NewContainer(cfg *config.ApplicationConfig, sessionFactory db.SessionFactory) *Container {
	return &Container{cfg: cfg, sessionFactory: sessionFactory}
}

func (c *Container) SessionFactory() db.SessionFactory {
	return c.sessionFactory
}

func (c *Container) Close() {
	if c.jwtHandler != nil {
		c.jwtHandler.Close()
	}
}
