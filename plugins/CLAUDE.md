# Claude Code Guidelines for Plugins

## Registration Pattern

Each resource plugin registers itself in an `init()` function. Reference: `clusters/plugin.go`

Four registrations per plugin:

1. **Service**: `registry.RegisterService("Clusters", func(env interface{}) interface{} { ... })`
2. **Routes**: `server.RegisterRoutes("clusters", func(router, services, ...) { ... })`
3. **Path**: `presenters.RegisterPath(api.Cluster{}, "clusters")` — maps type to URL path segment
4. **Kind**: `presenters.RegisterKind(api.Cluster{}, "Cluster")` — maps type to kind string

## ServiceLocator

Each plugin defines a `ServiceLocator` type (func returning the service) and a `Service()` helper that looks up the service from the environment's service registry.

```
type ServiceLocator func() services.ClusterService
func NewServiceLocator(env *environments.Env) ServiceLocator
func Service(s *environments.Services) services.ClusterService
```

## Route Setup

Inside `RegisterRoutes`, use gorilla/mux router methods:
- `router.HandleFunc("/clusters", handler.List).Methods("GET")`
- `router.HandleFunc("/clusters/{id}", handler.Get).Methods("GET")`
- Nested resources: `/clusters/{id}/nodepools`, `/clusters/{id}/statuses`

## Adding a New Resource

1. Create the DAO interface + implementation in `pkg/dao/`
2. Create the service interface + implementation in `pkg/services/`
3. Create the handler in `pkg/handlers/`
4. Create the plugin in `plugins/<resource>/plugin.go` with all 4 registrations
5. Run `make generate-mocks` to generate service mocks

## Related CLAUDE.md Files

- `pkg/handlers/CLAUDE.md` — Handler pipeline
- `pkg/services/CLAUDE.md` — Service patterns
- `pkg/dao/CLAUDE.md` — DAO patterns
