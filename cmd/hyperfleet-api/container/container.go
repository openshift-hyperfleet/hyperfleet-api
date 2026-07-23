package container

import (
	"context"
	"net/http"

	gorillahandlers "github.com/gorilla/handlers"
	"github.com/gorilla/mux"

	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/environments"
	"github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server"
	requestlogging "github.com/openshift-hyperfleet/hyperfleet-api/cmd/hyperfleet-api/server/logging"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/auth"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/dao"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/logger"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/middleware"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/registry"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/validators"
)

// Container lazily constructs and caches application dependencies during
// sequential startup. It is not safe for concurrent initialization.
//
// TODO(HYPERFLEET-1371): Once the environments/ package is removed,
// Container should source SessionFactory directly (e.g. from config/Viper)
// rather than accepting it as a constructor parameter. Close() should also
// close the SessionFactory at that point.
type Container struct {
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
	apiServer       *server.APIServer
}

func NewContainer(sessionFactory db.SessionFactory) *Container {
	return &Container{sessionFactory: sessionFactory}
}

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

func (c *Container) SessionFactory() db.SessionFactory {
	return c.sessionFactory
}

func (c *Container) ResourceService() services.ResourceService {
	if c.resourceService == nil {
		c.resourceService = services.NewResourceService(
			c.ResourceDao(),
			c.ResourceLabelDao(),
			c.AdapterStatusDao(),
			c.ResourceConditionDao(),
			c.GenericService(),
		)
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

func (c *Container) JWTHandler() *auth.JWTHandler {
	if c.jwtHandler == nil {
		env := environments.Environment()
		var err error
		c.jwtHandler, err = auth.NewJWTHandler(
			context.Background(),
			auth.JWTHandlerConfig{
				Issuers: env.Config.Server.JWT.Configs,
			},
		)
		if err != nil {
			panic("unable to create JWT handler: " + err.Error())
		}
	}
	return c.jwtHandler
}

func (c *Container) SchemaValidator() *validators.SchemaValidator {
	if c.schemaValidator == nil {
		schemaPath := environments.Environment().Config.Server.OpenAPISchemaPath
		schemaValidator, err := validators.NewSchemaValidator(schemaPath)
		if err != nil {
			panic("unable to create schema validator: " + err.Error())
		}
		c.schemaValidator = schemaValidator
		logger.With(context.Background(), logger.FieldSchemaPath, schemaPath).Info("Schema validation enabled")
	}
	return c.schemaValidator
}

// APIServer builds and caches the API server. tracingEnabled is only
// honored on the first call; subsequent calls return the cached instance
// regardless of the value passed.
func (c *Container) APIServer(tracingEnabled bool) *server.APIServer {
	if c.apiServer == nil {
		cfg := environments.Environment().Config
		registry.Validate()

		mainMiddleware := []mux.MiddlewareFunc{logger.RequestIDMiddleware}
		if tracingEnabled {
			mainMiddleware = append(mainMiddleware, middleware.OTelMiddleware)
		}
		masker := middleware.NewMaskingMiddleware(cfg.Logging)
		mainMiddleware = append(mainMiddleware, requestlogging.RequestLoggingMiddleware(masker))

		apiMiddleware := []mux.MiddlewareFunc{
			server.MetricsMiddleware,
			middleware.SchemaValidationMiddleware(c.SchemaValidator()),
			func(next http.Handler) http.Handler {
				return db.TransactionMiddleware(next, c.SessionFactory(), cfg.Database.Pool.RequestTimeout)
			},
			gorillahandlers.CompressHandler,
		}

		var protectedMiddleware []mux.MiddlewareFunc
		if cfg.Server.JWT.Enabled {
			callerIdentityMiddleware := auth.NewCallerIdentityMiddleware()
			protectedMiddleware = append(
				protectedMiddleware,
				c.JWTHandler().Middleware,
				callerIdentityMiddleware.ResolveCallerIdentity,
			)
		}

		router, err := server.NewRouter(
			mainMiddleware,
			apiMiddleware,
			protectedMiddleware,
			[]server.RouteRegistrar{
				server.NewEntityRouteRegistrar(
					c.ResourceService(),
					c.AdapterStatusService(),
					c.SchemaValidator(),
				),
			},
		)
		if err != nil {
			panic("unable to create API router: " + err.Error())
		}

		c.apiServer = server.NewAPIServer(
			cfg.Server,
			router,
		)
	}
	return c.apiServer
}

func (c *Container) Close() {
	if c.jwtHandler != nil {
		c.jwtHandler.Close()
	}
}
