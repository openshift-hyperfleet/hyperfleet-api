package factories

import (
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
)

type Factories struct {
	resourceService services.ResourceService
}

func New(resourceService services.ResourceService) Factories {
	return Factories{resourceService: resourceService}
}

// NewID generates a new unique identifier using RFC4122 UUID v7.
func (f *Factories) NewID() string {
	id, err := api.NewID()
	if err != nil {
		panic(fmt.Sprintf("test factory: failed to generate UUID v7: %v", err))
	}
	return id
}
