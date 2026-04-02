package factories

import (
	"fmt"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
)

type Factories struct {
}

// NewID generates a new unique identifier using RFC4122 UUID v7.
func (f *Factories) NewID() string {
	id, err := api.NewID()
	if err != nil {
		panic(fmt.Sprintf("test factory: failed to generate UUID v7: %v", err))
	}
	return id
}
