package adapters

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
)

type stubController struct {
	name string
}

func (c stubController) Name() string { return c.name }

func (stubController) Reconcile(context.Context, ClusterView) (StatusReport, error) {
	return StatusReport{}, nil
}

func TestRegistryResolvesFactoriesInChainOrder(t *testing.T) {
	RegisterTestingT(t)
	registry := NewRegistry(map[string]Factory{
		"first":  func() Controller { return stubController{name: "first"} },
		"second": func() Controller { return stubController{name: "second"} },
	})

	controllers, err := registry.Resolve([]string{"second", "first"})

	Expect(err).ToNot(HaveOccurred())
	Expect(controllers).To(HaveLen(2))
	Expect(controllers[0].Name()).To(Equal("second"))
	Expect(controllers[1].Name()).To(Equal("first"))
}

func TestRegistryUnknownNameFailsFast(t *testing.T) {
	RegisterTestingT(t)
	built := false
	registry := NewRegistry(map[string]Factory{
		"known": func() Controller {
			built = true
			return stubController{name: "known"}
		},
	})

	controllers, err := registry.Resolve([]string{"known", "missing"})

	Expect(err).To(HaveOccurred())
	Expect(err.Error()).To(ContainSubstring(`unknown adapter "missing"`))
	Expect(controllers).To(BeNil())
	Expect(built).To(BeFalse())
}
