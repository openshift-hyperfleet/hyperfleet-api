package adapters

import "fmt"

// Factory builds a controller instance for one chain position.
type Factory func() Controller

// Registry resolves adapter names from chain config to factories.
type Registry struct {
	factories map[string]Factory
}

// NewRegistry returns a registry containing the given factories.
func NewRegistry(factories map[string]Factory) *Registry {
	registered := make(map[string]Factory, len(factories))
	for name, factory := range factories {
		registered[name] = factory
	}
	return &Registry{factories: registered}
}

// Resolve returns controllers in chain order. Unknown names fail before any
// controller is built.
func (r *Registry) Resolve(names []string) ([]Controller, error) {
	for _, name := range names {
		if _, ok := r.factories[name]; !ok {
			return nil, fmt.Errorf("unknown adapter %q", name)
		}
	}

	controllers := make([]Controller, 0, len(names))
	for _, name := range names {
		controllers = append(controllers, r.factories[name]())
	}
	return controllers, nil
}
