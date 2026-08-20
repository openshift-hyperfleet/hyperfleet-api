package container

import (
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/closer"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validators"
)

// Container lazily constructs and caches application dependencies during
// sequential startup. It is not safe for concurrent initialization.
type Container struct {
	cfg            *config.ApplicationConfig
	closer         *closer.Closer
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

func NewContainer(cfg *config.ApplicationConfig, c *closer.Closer) *Container {
	return &Container{cfg: cfg, closer: c}
}
