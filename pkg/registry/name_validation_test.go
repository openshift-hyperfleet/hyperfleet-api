package registry

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestValidate_NameMaxLenLessThanMinLen_Panics(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{
		Kind:       "TestEntity",
		Plural:     "testentities",
		NameMinLen: 10,
		NameMaxLen: 5,
	})

	Expect(func() {
		Validate()
	}).To(PanicWith(ContainSubstring("NameMaxLen")))
}

func TestDescriptorFields_NameMinMaxLen(t *testing.T) {
	RegisterTestingT(t)
	Reset()

	Register(EntityDescriptor{
		Kind:       "Cluster",
		Plural:     "clusters",
		NameMinLen: 3,
		NameMaxLen: 53,
	})

	d := MustGet("Cluster")
	Expect(d.NameMinLen).To(Equal(3))
	Expect(d.NameMaxLen).To(Equal(53))
}
