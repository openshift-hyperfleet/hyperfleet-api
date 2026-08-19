package integration

import (
	"context"
	"net/http"
	"testing"

	. "github.com/onsi/gomega"

	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/api"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/db"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/errors"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/services"
	"github.com/openshift-hyperfleet/hyperfleet-api/pkg/tenant"
	"github.com/openshift-hyperfleet/hyperfleet-api/test"
)

const (
	tenancyClusterKind = "Cluster"
	tenancyClusterName = "prod"
	tenancyTestActor   = "test@example.com"
	tenancyOrgKey      = "org"
)

// tenancyCtx returns a context carrying a resolved tenant with the given dimensions.
func tenancyCtx(dimensions map[string]string) context.Context {
	return tenant.WithTenant(context.Background(), &tenant.ResolvedTenant{Dimensions: dimensions})
}

// newTenancyCluster builds a fresh Cluster resource. Create mutates the resource
// (ID, timestamps, tenancy), so each attempt needs its own instance.
func newTenancyCluster(name string) *api.Resource {
	return &api.Resource{
		Kind:      tenancyClusterKind,
		Name:      name,
		Spec:      []byte(`{"region": "us-central1", "provider": "gcp"}`),
		CreatedBy: tenancyTestActor,
		UpdatedBy: tenancyTestActor,
	}
}

// createInTx runs svc.Create inside a real write transaction, mirroring the transaction
// middleware. This gives the DAO a transaction to roll back on a conflict, so the assertions
// exercise the same write path as an HTTP request. The tenant-resolution middleware is not
// yet wired into the HTTP path, so base carries the tenant identity directly.
func createInTx(
	base context.Context, sf db.SessionFactory, svc services.ResourceService, r *api.Resource,
) (*api.Resource, *errors.ServiceError) {
	txCtx, err := db.NewContext(base, sf)
	Expect(err).NotTo(HaveOccurred())
	defer db.Resolve(txCtx)
	return svc.Create(txCtx, tenancyClusterKind, r, nil)
}

// deleteInTx runs svc.Delete inside a real write transaction.
func deleteInTx(
	base context.Context, sf db.SessionFactory, svc services.ResourceService, id string,
) *errors.ServiceError {
	txCtx, err := db.NewContext(base, sf)
	Expect(err).NotTo(HaveOccurred())
	defer db.Resolve(txCtx)
	_, svcErr := svc.Delete(txCtx, tenancyClusterKind, id)
	return svcErr
}

// TestClusterNameUniquenessIsTenantScoped verifies that root-resource name uniqueness is
// scoped to the caller's tenancy: two tenants may own a cluster with the same name, while a
// same-name create within a single tenancy still conflicts. It drives the service layer
// directly with distinct tenant contexts because the tenant-resolution middleware is not yet
// wired into the HTTP path.
func TestClusterNameUniquenessIsTenantScoped(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()
	sf := h.Container.SessionFactory()

	ctxAcme := tenancyCtx(map[string]string{tenancyOrgKey: "acme"})
	ctxGlobex := tenancyCtx(map[string]string{tenancyOrgKey: "globex"})

	// Tenant "acme" creates a cluster named "prod". The tenancy is stamped from the context.
	acme, svcErr := createInTx(ctxAcme, sf, svc, newTenancyCluster(tenancyClusterName))
	Expect(svcErr).To(BeNil())
	Expect(acme.Tenancy).To(MatchJSON(`{"org":"acme"}`))

	// Tenant "globex" creates a cluster with the SAME name — allowed, different tenancy.
	globex, svcErr := createInTx(ctxGlobex, sf, svc, newTenancyCluster(tenancyClusterName))
	Expect(svcErr).To(BeNil(), "cross-tenant same-name create should succeed")
	Expect(globex.Tenancy).To(MatchJSON(`{"org":"globex"}`))

	// Tenant "acme" creates "prod" again — conflict within its own tenancy.
	_, svcErr = createInTx(ctxAcme, sf, svc, newTenancyCluster(tenancyClusterName))
	Expect(svcErr).NotTo(BeNil(), "same-tenant duplicate name should be rejected")
	Expect(svcErr.IsConflict()).To(BeTrue())
	Expect(svcErr.HTTPCode).To(Equal(http.StatusConflict))
}

// TestClusterRecreateAfterDeleteWithTenancy verifies the soft-delete-and-recreate flow still
// works under the tenant-scoped unique index: a name freed by deletion can be reused within
// the same tenancy.
func TestClusterRecreateAfterDeleteWithTenancy(t *testing.T) {
	h, _ := test.RegisterIntegration(t)
	svc := h.Container.ResourceService()
	sf := h.Container.SessionFactory()

	ctxAcme := tenancyCtx(map[string]string{tenancyOrgKey: "acme"})

	created, svcErr := createInTx(ctxAcme, sf, svc, newTenancyCluster(tenancyClusterName))
	Expect(svcErr).To(BeNil())

	// A second create with the same name in the same tenancy conflicts.
	_, svcErr = createInTx(ctxAcme, sf, svc, newTenancyCluster(tenancyClusterName))
	Expect(svcErr).NotTo(BeNil(), "same-tenant duplicate name should be rejected")
	Expect(svcErr.IsConflict()).To(BeTrue())
	Expect(svcErr.HTTPCode).To(Equal(http.StatusConflict))

	// Delete the original, then recreate with the same name — the deleted row no longer
	// participates in the unique index.
	Expect(deleteInTx(ctxAcme, sf, svc, created.ID)).To(BeNil())

	_, svcErr = createInTx(ctxAcme, sf, svc, newTenancyCluster(tenancyClusterName))
	Expect(svcErr).To(BeNil(), "recreate after delete should succeed")
}
