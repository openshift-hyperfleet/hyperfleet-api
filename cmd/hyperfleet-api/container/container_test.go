package container

import (
	"testing"

	. "github.com/onsi/gomega"

	dbmocks "github.com/openshift-hyperfleet/hyperfleet-api/pkg/db/mocks"
)

func newTestContainer(t *testing.T) *Container {
	t.Helper()

	sessionFactory := dbmocks.NewMockSessionFactory()
	t.Cleanup(func() { _ = sessionFactory.Close() })

	return NewContainer(sessionFactory)
}

func TestContainerCachesDAOsAndServices(t *testing.T) {
	RegisterTestingT(t)

	c := newTestContainer(t)

	Expect(c.SessionFactory()).NotTo(BeNil())

	Expect(c.ResourceDao()).NotTo(BeNil())
	Expect(c.ResourceDao()).To(BeIdenticalTo(c.ResourceDao()))
	Expect(c.ResourceLabelDao()).NotTo(BeNil())
	Expect(c.ResourceLabelDao()).To(BeIdenticalTo(c.ResourceLabelDao()))
	Expect(c.AdapterStatusDao()).NotTo(BeNil())
	Expect(c.AdapterStatusDao()).To(BeIdenticalTo(c.AdapterStatusDao()))
	Expect(c.ResourceConditionDao()).NotTo(BeNil())
	Expect(c.ResourceConditionDao()).To(BeIdenticalTo(c.ResourceConditionDao()))
	Expect(c.GenericDao()).NotTo(BeNil())
	Expect(c.GenericDao()).To(BeIdenticalTo(c.GenericDao()))

	Expect(c.GenericService()).NotTo(BeNil())
	Expect(c.GenericService()).To(BeIdenticalTo(c.GenericService()))
	Expect(c.AdapterStatusService()).NotTo(BeNil())
	Expect(c.AdapterStatusService()).To(BeIdenticalTo(c.AdapterStatusService()))
	Expect(c.ResourceService()).NotTo(BeNil())
	Expect(c.ResourceService()).To(BeIdenticalTo(c.ResourceService()))
}

func TestContainerConstructionIsLazy(t *testing.T) {
	RegisterTestingT(t)

	c := newTestContainer(t)

	Expect(c.resourceDao).To(BeNil())
	Expect(c.resourceLabelDao).To(BeNil())
	Expect(c.adapterStatusDao).To(BeNil())
	Expect(c.resourceConditionDao).To(BeNil())
	Expect(c.genericDao).To(BeNil())
	Expect(c.resourceService).To(BeNil())
	Expect(c.adapterStatusService).To(BeNil())
	Expect(c.genericService).To(BeNil())
	Expect(c.schemaValidator).To(BeNil())
	Expect(c.apiServer).To(BeNil())
	Expect(c.jwtHandler).To(BeNil())
}
